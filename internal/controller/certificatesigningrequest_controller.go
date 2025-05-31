/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// CertificateSigningRequestReconciler reconciles a CertificateSigningRequest object
// Note: CertificateSigningRequest is a cluster-scoped resource, not namespace-scoped
type CertificateSigningRequestReconciler struct {
	*SharedReconciler
}

// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/approval,verbs=update
// +kubebuilder:rbac:groups=certificates.k8s.io,resources=certificatesigningrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets/finalizers,verbs=update
// Note: CSRs are cluster-scoped resources, while Secrets are namespace-scoped

// cleanupOrphanedSecrets removes secrets that no longer have a corresponding CSR
func (r *CertificateSigningRequestReconciler) cleanupOrphanedSecrets(ctx context.Context) error {
	log := logf.FromContext(ctx)

	// List all secrets with our type
	secretList := &corev1.SecretList{}
	if err := r.Client().List(ctx, secretList, client.MatchingFields{"type": "networkpolicy.webhook.io/approval"}); err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	// Check each secret to see if its CSR still exists
	for i := range secretList.Items {
		secret := &secretList.Items[i]
		csrName, ok := secret.Annotations["networkpolicy.webhook.io/csr-name"]
		if !ok {
			continue
		}

		// Check if CSR exists
		csr := &certificatesv1.CertificateSigningRequest{}
		exists, err := r.GetResource(ctx, types.NamespacedName{Name: csrName}, csr)
		if err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to check if CSR exists", "csr", csrName)
			continue
		}

		if !exists {
			// CSR doesn't exist, but secret still does - remove finalizer and delete
			log.Info("Found orphaned secret without CSR, cleaning up", "secret", secret.Name, "namespace", secret.Namespace)

			// First remove finalizer if it exists
			if controllerutil.ContainsFinalizer(secret, "networkpolicy-approval-protection") {
				toContinue, err := r.RemoveFinalizer(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, secret, "networkpolicy-approval-protection")
				if !toContinue || err != nil {
					log.Error(err, "Failed to remove finalizer from orphaned secret")
					continue
				}
			}

			// Delete the secret
			_, err = r.DeleteResource(ctx, secret)
			if err != nil {
				log.Error(err, "Failed to delete orphaned secret")
			}
		}
	}

	return nil
}

func (r *CertificateSigningRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("csr", req.Name)
	log.Info("Reconciling CSR")

	// Get the CSR object
	csr := &certificatesv1.CertificateSigningRequest{}
	exists, err := r.GetResource(ctx, req.NamespacedName, csr)
	if err != nil || !exists {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "Failed to get CSR")
			return ctrl.Result{}, err
		}
		// CSR not found, likely deleted
		return ctrl.Result{}, nil
	}

	// Check if this is a NetworkPolicy approval CSR by looking for the label
	if _, isNPApproval := csr.Labels["networkpolicy.webhook.io/approval"]; !isNPApproval {
		// Not a NetworkPolicy approval CSR, ignore
		return ctrl.Result{}, nil
	}

	// Check if CSR has been approved
	isApproved := false
	for _, condition := range csr.Status.Conditions {
		if condition.Type == certificatesv1.CertificateApproved {
			isApproved = true
			break
		}
	}

	if !isApproved {
		// CSR not yet approved, nothing to do
		return ctrl.Result{}, nil
	}

	// Get NetworkPolicy details from CSR annotations
	npName, hasNPName := csr.Annotations["networkpolicy.webhook.io/name"]
	npNamespace, hasNPNamespace := csr.Annotations["networkpolicy.webhook.io/namespace"]
	approvalHash, hasHash := csr.Annotations["networkpolicy.webhook.io/approval-hash"]

	if !hasNPName || !hasNPNamespace || !hasHash {
		log.Info("CSR missing required annotations", "name", csr.Name)
		return ctrl.Result{}, nil
	}

	// Certificate data should be in the CSR status
	if len(csr.Status.Certificate) == 0 {
		log.Info("Approved CSR has no certificate data yet", "name", csr.Name)
		return ctrl.Result{Requeue: true}, nil
	}

	// Create or update the secret with the certificate
	secretName := fmt.Sprintf("np-approval-%s-%s", npNamespace, npName)
	secretNamespacedName := types.NamespacedName{
		Name:      secretName,
		Namespace: npNamespace,
	}

	// Check if secret already exists
	secret := &corev1.Secret{}
	exists, err = r.GetResource(ctx, secretNamespacedName, secret)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to check if secret exists")
		return ctrl.Result{}, err
	}

	// Prepare secret data - use only valid keys (alphanumeric, -, _ or .)
	secretData := map[string][]byte{
		"hash":     []byte(approvalHash),
		"tls-crt":  csr.Status.Certificate,
		"csr-name": []byte(csr.Name),
	}

	// Create metadata for annotations - will go in secret's metadata not data
	annotations := map[string]string{
		"networkpolicy.webhook.io/csr-name":      csr.Name,
		"networkpolicy.webhook.io/approval-hash": approvalHash,
		"networkpolicy.webhook.io/np-name":       npName,
		"networkpolicy.webhook.io/np-namespace":  npNamespace,
	}

	if !exists {
		// Create new secret
		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: npNamespace,
				Labels: map[string]string{
					"networkpolicy.webhook.io/approval": "true",
					"networkpolicy.webhook.io/name":     npName,
				},
				Annotations: annotations,
			},
			Type: "networkpolicy.webhook.io/approval",
			Data: secretData,
		}

		// Create the secret
		toContinue, err := r.CreateResource(ctx, newSecret)
		if !toContinue || err != nil {
			log.Error(err, "Failed to create secret")
			return ctrl.Result{}, err
		}

		// Add finalizer to the secret
		toContinue, err = r.AddFinalizer(ctx, secretNamespacedName, newSecret, "networkpolicy.webhook.io/approval-protection")
		if !toContinue || err != nil {
			log.Error(err, "Failed to add finalizer to secret")
			return ctrl.Result{}, err
		}

		log.Info("Created secret for approved NetworkPolicy", "name", secretName, "namespace", npNamespace)
	} else {
		// Update existing secret
		secret.Data = secretData

		// Update the secret
		toContinue, err := r.UpdateResource(ctx, secretNamespacedName, secret)
		if !toContinue || err != nil {
			log.Error(err, "Failed to update secret")
			return ctrl.Result{}, err
		}

		// Ensure finalizer is set
		toContinue, err = r.AddFinalizer(ctx, secretNamespacedName, secret, "networkpolicy.webhook.io/approval-protection")
		if !toContinue || err != nil {
			log.Error(err, "Failed to add finalizer to secret")
			return ctrl.Result{}, err
		}

		log.Info("Updated secret for approved NetworkPolicy", "name", secretName, "namespace", npNamespace)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CertificateSigningRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&certificatesv1.CertificateSigningRequest{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			// Only process CSRs with our label
			csr, ok := obj.(*certificatesv1.CertificateSigningRequest)
			if !ok {
				return false
			}

			// Check for our specific label
			_, hasLabel := csr.Labels["networkpolicy.webhook.io/approval"]
			if !hasLabel {
				return false
			}

			// Additional check: Only process CSRs that have been approved or denied
			for _, condition := range csr.Status.Conditions {
				if condition.Type == certificatesv1.CertificateApproved {
					return true
				}
			}

			// If we got here, CSR has our label but isn't approved yet - return true
			// to stay informed about future status changes
			return true
		})).
		Named("certificatesigningrequest").
		Complete(r)
}
