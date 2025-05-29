package utils

import (
	"context"
	"fmt"
	"reflect"

	"github.com/hadi2f244/approve-controller/internal/pkg/consts"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// copy of controllerutil.referSameObject
func referSameObject(a, b metav1.OwnerReference) bool {
	aGV, err := schema.ParseGroupVersion(a.APIVersion)
	if err != nil {
		return false
	}

	bGV, err := schema.ParseGroupVersion(b.APIVersion)
	if err != nil {
		return false
	}

	return aGV.Group == bGV.Group && a.Kind == b.Kind && a.Name == b.Name
}

// copy of controllerutil.indexOwnerRef
func indexOwnerRef(ownerReferences []metav1.OwnerReference, ref metav1.OwnerReference) int {
	for index, r := range ownerReferences {
		if referSameObject(r, ref) {
			return index
		}
	}
	return -1
}

// copy of controllerutil.validateOwner
func validateOwner(owner, object metav1.Object) error {
	ownerNs := owner.GetNamespace()
	if ownerNs != "" {
		objNs := object.GetNamespace()
		if objNs == "" {
			return fmt.Errorf("cluster-scoped resource must not have a namespace-scoped owner, owner's namespace %s", ownerNs)
		}
		if ownerNs != objNs {
			return fmt.Errorf("cross-namespace owner references are disallowed, owner's namespace %s, obj's namespace %s", owner.GetNamespace(), object.GetNamespace())
		}
	}
	return nil
}

// Delete owner references
func DeleteOwnerReference(owner, object metav1.Object, scheme *runtime.Scheme) error {
	// Validate the owner.
	ro, ok := owner.(runtime.Object)
	if !ok {
		return fmt.Errorf("%T is not a runtime.Object, cannot call SetOwnerReference", owner)
	}
	if err := validateOwner(owner, object); err != nil {
		return err
	}

	// Create a new owner ref.
	gvk, err := apiutil.GVKForObject(ro, scheme)
	if err != nil {
		return err
	}
	ref := metav1.OwnerReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		UID:        owner.GetUID(),
		Name:       owner.GetName(),
	}

	// Update owner references and return.
	delOwnerRef(ref, object)
	return nil
}

// Delete an item from owner references
func delOwnerRef(ref metav1.OwnerReference, object metav1.Object) {
	owners := object.GetOwnerReferences()
	if idx := indexOwnerRef(owners, ref); idx != -1 {
		owners = append(owners[:idx], owners[idx+1:]...)
	}
	object.SetOwnerReferences(owners)
}

func CheckNamespaceExists(r client.Client, ctx context.Context, namespaceName string) (bool, error) {
	log := log.FromContext(ctx)
	// Define a namespace object
	namespace := &v1.Namespace{}
	// Attempt to get the namespace with the provided name
	err := r.Get(ctx, client.ObjectKey{Name: namespaceName}, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, err
		} else {
			log.Error(err, fmt.Sprintf("Issue on getting namespaces %s", namespaceName))
			return false, err
		}
	} else {
		return true, nil
	}
}

// Function that convertes inner and helper functions output to proper Reconcile Result
func HandleReconcileReturnValue(toContinue bool, err error) (interface{}, error) {
	config, _ := consts.NewConfiguration()

	if !toContinue {
		if err != nil {
			if apierrors.IsNotFound(err) {
				return ctrl.Result{}, nil
			} else if apierrors.IsNotFound(err) {
				return ctrl.Result{Requeue: true}, nil
			} else if consts.IsRequeueError(err) {
				return ctrl.Result{Requeue: true}, nil
			} else if consts.IsLookupIPError(err) {
				return ctrl.Result{RequeueAfter: config.GetLookupRequeueAfterTimeSecond()}, nil
			} else if consts.IsRequeueAfterError(err) {
				if requeueAfterErrorInstance, ok := err.(*consts.RequeueAfterError); ok {
					return ctrl.Result{RequeueAfter: requeueAfterErrorInstance.RequeueAfter}, nil
				} else {
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{}, err
		} else {
			return ctrl.Result{}, nil
		}
	}
	return nil, nil
}

// ContainsAll checks that all elements of listB are contained in listA
func ContainsAllList[T comparable](listA, listB []T) bool {
	// Create a map to store elements of listA
	set := make(map[T]struct{})

	// Populate the set with all elements of listA
	for _, item := range listA {
		set[item] = struct{}{}
	}

	// Check if all elements of listB exist in the set
	for _, item := range listB {
		if _, found := set[item]; !found {
			return false
		}
	}

	return true
}

// ContainsAll checks if mapA contains all the key-value pairs of mapB
func ContainsAllMap[K comparable, V any](mapA, mapB map[K]V) bool {
	// Iterate through all items in mapB
	for key, valueB := range mapB {
		// Check if the key exists in mapA
		valueA, exists := mapA[key]
		if !exists {
			return false
		}
		// Use reflect.DeepEqual for comparing values
		if !reflect.DeepEqual(valueA, valueB) {
			return false
		}
	}
	return true
}
