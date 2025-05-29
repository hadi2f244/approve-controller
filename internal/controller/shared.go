package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"github.com/hadi2f244/approve-controller/internal/pkg/consts"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type SharedReconciler struct {
	client          client.Client
	scheme          *runtime.Scheme
	apiClientReader client.Reader
	logger          logr.Logger
	recorder        record.EventRecorder
}

// blank assignment to verify that SharedReconciler implements reconcile.Reconciler
var _ reconcile.Reconciler = &SharedReconciler{}

func NewSharedReconciler(
	client client.Client, scheme *runtime.Scheme, apiClientReader client.Reader,
	logger logr.Logger, recorder record.EventRecorder) *SharedReconciler {
	return &SharedReconciler{
		client:          client,
		scheme:          scheme,
		apiClientReader: apiClientReader,
		logger:          logger,
		recorder:        recorder,
	}
}

func (r *SharedReconciler) Reconcile(context.Context, ctrl.Request) (ctrl.Result, error) {
	return reconcile.Result{}, nil
}

// Client returns a split client that reads objects from
// the cache and writes to the Kubernetes APIServer
func (r *SharedReconciler) Client() client.Client {
	return r.client
}

// APIClientReader return a client that directly reads objects
// from the Kubernetes APIServer
func (r *SharedReconciler) APIClientReader() client.Reader {
	return r.apiClientReader
}

func (r *SharedReconciler) Scheme() *runtime.Scheme {
	return r.scheme
}

func (r *SharedReconciler) Logger() logr.Logger {
	return r.logger
}

func (r *SharedReconciler) Recorder() record.EventRecorder {
	return r.recorder
}

func (r *SharedReconciler) GetResource(ctx context.Context, objKey types.NamespacedName, obj client.Object) (bool, error) {
	// logger, _ := logr.FromContext(ctx)
	err := r.Client().Get(ctx, objKey, obj)
	if err != nil {
		// Error reading the object - requeue the request.
		// logger.Info("Failed to get ", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", objKey.Name, "Error", err)
		return false, err
	}
	return true, nil
}

func (r *SharedReconciler) CreateResource(ctx context.Context, obj client.Object) (bool, error) {
	logger, _ := logr.FromContext(ctx)
	if err := r.Client().Create(ctx, obj); err != nil {
		logger.Error(err, "create object", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", obj.GetName(), "namespace", obj.GetNamespace())
		return false, err
	}
	r.Recorder().Eventf(obj, core.EventTypeNormal, "Created ", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", obj.GetName(), "namespace", obj.GetNamespace())
	// Let's re-fetch the object after update
	return true, nil
}

func (r *SharedReconciler) UpdateResource(ctx context.Context, objKey types.NamespacedName, obj client.Object) (bool, error) {
	logger, _ := logr.FromContext(ctx)

	if err := r.Client().Update(ctx, obj); err != nil {
		logger.Error(err, "update object", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", obj.GetName(), "namespace", obj.GetNamespace())
		return false, err
	}
	r.Recorder().Eventf(obj, core.EventTypeNormal, "Updated ", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", obj.GetName(), "namespace", obj.GetNamespace())

	// toContinue, err := r.GetResource(ctx, objKey, obj)
	// return toContinue, err
	return true, nil
}

func (r *SharedReconciler) UpdateResourceStatus(ctx context.Context, objKey types.NamespacedName, obj client.Object) (bool, error) {
	logger, _ := logr.FromContext(ctx)
	if err := r.Client().Status().Update(ctx, obj); err != nil {
		logger.Error(err, "failed update object status", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", obj.GetName(), "namespace", obj.GetNamespace())
		return false, err
	}
	r.Recorder().Eventf(obj, core.EventTypeNormal, "Updated status ", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", obj.GetName(), "namespace", obj.GetNamespace())

	// toContinue, err := r.GetResource(ctx, objKey, obj)
	// return toContinue, err
	return true, nil
}

func (r *SharedReconciler) DeleteResource(ctx context.Context, obj client.Object, options ...client.DeleteOption) (bool, error) {
	logger, _ := logr.FromContext(ctx)
	if err := r.Client().Delete(ctx, obj, options...); err != nil {
		logger.Error(err, "delete object", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", obj.GetName(), "namespace", obj.GetNamespace())
		return false, err
	}
	r.Recorder().Eventf(obj, core.EventTypeNormal, "Deleted ", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", obj.GetName(), "namespace", obj.GetNamespace())
	return true, nil
}

func (r *SharedReconciler) AddFinalizer(ctx context.Context, objKey types.NamespacedName, obj client.Object, finalizer string) (bool, error) {
	logger, _ := logr.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(obj, finalizer) {
		logger.Info("add finalizer to ", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), "name", objKey.Name)
		if ok := controllerutil.AddFinalizer(obj, finalizer); !ok {
			logger.Error(errors.New("Add finalizer issue"), "failed to add finalizer to ", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), " name", objKey.Name)
			return false, errors.New("Add finalizer issue")
		}
		if toContinue, err := r.UpdateResource(ctx, objKey, obj); !toContinue {
			return false, err
		} else if !toContinue && err == nil {
			return false, &consts.RequeueError{}
		}
	}
	return true, nil
}

func (r *SharedReconciler) RemoveFinalizer(ctx context.Context, objKey types.NamespacedName, obj client.Object, finalizer string) (bool, error) {
	logger, _ := logr.FromContext(ctx)
	if controllerutil.ContainsFinalizer(obj, finalizer) {
		if ok := controllerutil.RemoveFinalizer(obj, finalizer); !ok {
			logger.Error(errors.New("Add finalizer issue"), "failed to remove finalizer from ", "kind", strings.Replace(fmt.Sprintf("%T", obj), "*", "", 1), " name", objKey.Name)
			return false, errors.New("Add finalizer issue")
		}
		if toContinue, err := r.UpdateResource(ctx, objKey, obj); !toContinue {
			return false, err
		} else if !toContinue && err == nil {
			return false, &consts.RequeueError{}
		}
	}
	return true, nil
}

func (r *SharedReconciler) ListOwnedResources(ctx context.Context, objKey types.NamespacedName, objList client.ObjectList, matchingField client.MatchingFields) (bool, error) {
	logger, _ := logr.FromContext(ctx)
	if err := r.Client().List(ctx, objList, client.InNamespace(objKey.Namespace), matchingField); err != nil {
		logger.Error(err, "Failed to list owned resources of ", "kind", strings.Replace(fmt.Sprintf("%T", objKey.Namespace), "*", "", 1), "name", objKey.Name)
		return false, err
	}
	return true, nil
}
