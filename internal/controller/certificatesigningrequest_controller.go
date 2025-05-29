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
	"github.com/go-logr/logr"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// CertificateSigningRequestReconciler reconciles a CertificateSigningRequest object
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

func (r *CertificateSigningRequestReconciler) createSecret(ctx context.Context, req ctrl.Request, namespace *v1.Namespace, calicoNPFound *calicov3.NetworkPolicy, calicoNP *calicov3.NetworkPolicy) (bool, error) {
	log := r.Logger().WithName("CertificateSigningRequest_Controller")

	toContinue, err := r.GetResource(ctx, types.NamespacedName{Name: namespace.Name, Namespace: namespace.Name}, calicoNPFound)
	if !toContinue && err != nil && !apierrors.IsNotFound(err) {
		log.Info("Failed to get Calico NetworkPolicy")
		return false, err
	} else if !toContinue && err != nil && apierrors.IsNotFound(err) { // Not found, Create new calicoNP
		log.Info("Creating a new Calico NetworkPolicy NS",
			"CalicoNetworkPolicy.Namespace", calicoNP.Namespace, "CalicoNetworkPolicy.Name", calicoNP.Name)
		if toContinue, err := r.CreateResource(ctx, calicoNP); !toContinue {
			log.Error(err, "Failed to create new Calico NetworkPolicy",
				"CalicoNetworkPolicy.Namespace", calicoNP.Namespace, "CalicoNetworkPolicy.Name", calicoNP.Name)
		}
	} else { // Check existing Calico NetworkPolicy
		if equal, _ := calico_utils.IsCalicoNPEqual(calicoNPFound, calicoNP); !equal {
			// Delete old calico networkpolicy if not equal to current calico networkpolicy
			if err := r.Client().Delete(ctx, calicoNPFound); err != nil {
				log.Error(err, "failed to delete Calico NetworkPolicy resource")
				return false, err
			}
			r.Recorder().Eventf(namespace, core.EventTypeNormal, "Deleted", "Deleted old CalicoNetworkPolicy %q", calicoNPFound.Name)
			log.Info(fmt.Sprintf("finished cleaning up old CalicoNetworkPolicy resources %s", calicoNPFound.Name))
		}
	}
	return true, nil
}

func (r *CertificateSigningRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CertificateSigningRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For().
		Named("certificatesigningrequest").
		Complete(r)
}
