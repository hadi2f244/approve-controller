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

package v1

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
// log is for logging in this package.
var networkpolicylog = logf.Log.WithName("networkpolicy-resource")

const (
	// AnnotationApprovalHash contains the hash of the approved NetworkPolicy
	AnnotationApprovalHash = "networkpolicy.webhook.io/approval-hash"
	// AnnotationCSRName contains the CSR name for pending approval
	AnnotationCSRName = "networkpolicy.webhook.io/csr-name"
	// LabelNetworkPolicyApproval labels CSRs for NetworkPolicy approval
	LabelNetworkPolicyApproval = "networkpolicy.webhook.io/approval"
	// SecretTypeNetworkPolicyApproval is the type for approved NetworkPolicy secrets
	SecretTypeNetworkPolicyApproval = "networkpolicy.webhook.io/approval"
)

// SetupNetworkPolicyWebhookWithManager registers the webhook for NetworkPolicy in the manager.
func SetupNetworkPolicyWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&networkingv1.NetworkPolicy{}).
		WithValidator(&NetworkPolicyCustomValidator{Client: mgr.GetClient()}).
		WithDefaulter(&NetworkPolicyCustomDefaulter{}).
		Complete()
}

// NetworkPolicyData represents the data used for generating hash
type NetworkPolicyData struct {
	Name      string                         `json:"name"`
	Namespace string                         `json:"namespace"`
	Spec      networkingv1.NetworkPolicySpec `json:"spec"`
}

// generateNetworkPolicyHash creates a unique hash for the NetworkPolicy
func generateNetworkPolicyHash(np *networkingv1.NetworkPolicy) (string, error) {
	data := NetworkPolicyData{
		Name:      np.Name,
		Namespace: np.Namespace,
		Spec:      np.Spec,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal NetworkPolicy data: %w", err)
	}

	hash := sha256.Sum256(jsonData)
	return fmt.Sprintf("%x", hash), nil
}

// +kubebuilder:webhook:path=/mutate-networking-k8s-io-v1-networkpolicy,mutating=true,failurePolicy=fail,sideEffects=None,groups=networking.k8s.io,resources=networkpolicies,verbs=create;update,versions=v1,name=mnetworkpolicy-v1.kb.io,admissionReviewVersions=v1

// NetworkPolicyCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind NetworkPolicy when those are created or updated.
type NetworkPolicyCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &NetworkPolicyCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind NetworkPolicy.
func (d *NetworkPolicyCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	networkpolicy, ok := obj.(*networkingv1.NetworkPolicy)

	if !ok {
		return fmt.Errorf("expected an NetworkPolicy object but got %T", obj)
	}
	networkpolicylog.Info("Defaulting for NetworkPolicy", "name", networkpolicy.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// +kubebuilder:webhook:path=/validate-networking-k8s-io-v1-networkpolicy,mutating=false,failurePolicy=fail,sideEffects=None,groups=networking.k8s.io,resources=networkpolicies,verbs=create;update,versions=v1,name=vnetworkpolicy-v1.kb.io,admissionReviewVersions=v1

// NetworkPolicyCustomValidator struct is responsible for validating the NetworkPolicy resource
// when it is created, updated, or deleted.
type NetworkPolicyCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &NetworkPolicyCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NetworkPolicy.
func (v *NetworkPolicyCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	networkpolicy, ok := obj.(*networkingv1.NetworkPolicy)
	if !ok {
		return nil, fmt.Errorf("expected a NetworkPolicy object but got %T", obj)
	}
	networkpolicylog.Info("Validation for NetworkPolicy upon creation", "name", networkpolicy.GetName())

	return v.validateNetworkPolicyApproval(ctx, networkpolicy)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NetworkPolicy.
func (v *NetworkPolicyCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	networkpolicy, ok := newObj.(*networkingv1.NetworkPolicy)
	if !ok {
		return nil, fmt.Errorf("expected a NetworkPolicy object for the newObj but got %T", newObj)
	}
	networkpolicylog.Info("Validation for NetworkPolicy upon update", "name", networkpolicy.GetName())

	return v.validateNetworkPolicyApproval(ctx, networkpolicy)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NetworkPolicy.
func (v *NetworkPolicyCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	networkpolicy, ok := obj.(*networkingv1.NetworkPolicy)
	if !ok {
		return nil, fmt.Errorf("expected a NetworkPolicy object but got %T", obj)
	}
	networkpolicylog.Info("Validation for NetworkPolicy upon deletion", "name", networkpolicy.GetName())

	// Allow deletion without approval check
	return nil, nil
}

// validateNetworkPolicyApproval validates if the NetworkPolicy is approved
func (v *NetworkPolicyCustomValidator) validateNetworkPolicyApproval(ctx context.Context, np *networkingv1.NetworkPolicy) (admission.Warnings, error) {
	hash, err := generateNetworkPolicyHash(np)
	if err != nil {
		return nil, fmt.Errorf("failed to generate NetworkPolicy hash: %w", err)
	}

	// Check if there's an approved certificate (secret) for this NetworkPolicy
	approved, err := v.checkForApprovedCertificate(ctx, np, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to check for approved certificate: %w", err)
	}

	if approved {
		networkpolicylog.Info("NetworkPolicy is approved", "name", np.Name, "namespace", np.Namespace, "hash", hash)
		return nil, nil
	}

	// Check if CSR already exists
	csrName := fmt.Sprintf("np-approval-%s-%s", np.Namespace, np.Name)
	existingCSR := &certificatesv1.CertificateSigningRequest{}
	err = v.Client.Get(ctx, types.NamespacedName{Name: csrName}, existingCSR)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check existing CSR: %w", err)
	}

	if errors.IsNotFound(err) {
		// Create CSR for approval
		err = v.createApprovalCSR(ctx, np, hash, csrName)
		if err != nil {
			return nil, fmt.Errorf("failed to create approval CSR: %w", err)
		}
	}

	return nil, fmt.Errorf("NetworkPolicy has not been approved yet. CSR created: %s. Please ask an administrator to approve the CSR", csrName)
}

// checkForApprovedCertificate checks if there's a valid approved certificate for the NetworkPolicy
func (v *NetworkPolicyCustomValidator) checkForApprovedCertificate(ctx context.Context, np *networkingv1.NetworkPolicy, hash string) (bool, error) {
	secretName := fmt.Sprintf("np-approval-%s-%s", np.Namespace, np.Name)
	secret := &corev1.Secret{}

	err := v.Client.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: np.Namespace,
	}, secret)

	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// Verify the secret type
	if secret.Type != SecretTypeNetworkPolicyApproval {
		return false, nil
	}

	// Verify the hash matches
	storedHash, exists := secret.Data["hash"]
	if !exists {
		return false, nil
	}

	if string(storedHash) != hash {
		networkpolicylog.Info("Hash mismatch", "stored", string(storedHash), "calculated", hash)
		return false, nil
	}

	// Verify certificate data exists
	cert, exists := secret.Data["tls.crt"]
	if !exists || len(cert) == 0 {
		return false, nil
	}

	return true, nil
}

// createApprovalCSR creates a CSR for NetworkPolicy approval
func (v *NetworkPolicyCustomValidator) createApprovalCSR(ctx context.Context, np *networkingv1.NetworkPolicy, hash, csrName string) error {
	// Create CSR with NetworkPolicy metadata
	//npData, err := json.Marshal(NetworkPolicyData{
	//	Name:      np.Name,
	//	Namespace: np.Namespace,
	//	Spec:      np.Spec,
	//})
	//if err != nil {
	//	return fmt.Errorf("failed to marshal NetworkPolicy data: %w", err)
	//}

	// Generate private key for CSR
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate request template
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("np-approval-%s-%s", np.Namespace, np.Name),
			Organization: []string{"networkpolicy-approval"},
		},
		DNSNames: []string{
			fmt.Sprintf("np-approval-%s-%s", np.Namespace, np.Name),
		},
	}

	// Create CSR
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, template, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate request: %w", err)
	}

	// Encode CSR to PEM format
	csrRequest := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	csr := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrName,
			Labels: map[string]string{
				LabelNetworkPolicyApproval: "true",
			},
			Annotations: map[string]string{
				AnnotationApprovalHash:               hash,
				"networkpolicy.webhook.io/name":      np.Name,
				"networkpolicy.webhook.io/namespace": np.Namespace,
			},
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request: csrRequest,
			Usages: []certificatesv1.KeyUsage{
				certificatesv1.UsageDigitalSignature,
				certificatesv1.UsageKeyEncipherment,
				certificatesv1.UsageClientAuth,
			},
			SignerName: "kubernetes.io/kube-apiserver-client",
		},
	}

	err = v.Client.Create(ctx, csr)
	if err != nil {
		return fmt.Errorf("failed to create CSR: %w", err)
	}

	networkpolicylog.Info("Created CSR for NetworkPolicy approval", "csr", csrName, "networkpolicy", np.Name, "namespace", np.Namespace)
	return nil
}
