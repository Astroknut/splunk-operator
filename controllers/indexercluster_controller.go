/*
Copyright 2021.

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

package controllers

import (
	"context"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	enterprisev4 "github.com/splunk/splunk-operator/api/v4"
	enterprise "github.com/splunk/splunk-operator/pkg/splunk/enterprise"
)

// IndexerClusterReconciler reconciles a IndexerCluster object
type IndexerClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=enterprise.splunk.com,resources=indexerclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=enterprise.splunk.com,resources=indexerclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=enterprise.splunk.com,resources=indexerclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the IndexerCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *IndexerClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// your logic here
	reconcileCounters.With(getPrometheusLabels(req)).Inc()

	reqLogger := log.FromContext(ctx)
	reqLogger = reqLogger.WithValues("baremetalhost", req.NamespacedName)
	reqLogger.Info("start")

	// Fetch the BareMetalHost
	instance := &enterprisev4.IndexerCluster{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after
			// reconcile request.  Owned objects are automatically
			// garbage collected. For additional cleanup logic use
			// finalizers.  Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, errors.Wrap(err, "could not load standalone data")
	}
	return enterprise.ApplyIndexerCluster(r.Client, instance)
}

// SetupWithManager sets up the controller with the Manager.
func (r *IndexerClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&enterprisev4.IndexerCluster{}).
		WithEventFilter(predicate.Or(
			predicate.GenerationChangedPredicate{},
		)).
		Watches(&source.Kind{Type: &appsv1.StatefulSet{}},
			&handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &enterprisev4.SearchHeadCluster{},
			}).
		Watches(&source.Kind{Type: &corev1.Secret{}},
			&handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &enterprisev4.SearchHeadCluster{},
			}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: enterprisev4.TotalWroker,
		}).
		Complete(r)
}