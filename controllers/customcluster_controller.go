/*


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
	"cloudengine/pkg/common/event"
	"cloudengine/pkg/common/results"
	"cloudengine/pkg/customcluster"
	"cloudengine/pkg/eventbus"
	"context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hackathonv1 "cloudengine/api/v1"
)

// CustomClusterReconciler reconciles a CustomCluster object
type CustomClusterReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Log      logr.Logger
	Scheme   *runtime.Scheme
}

// +kubebuilder:rbac:groups=hackathon.kaiyuanshe.cn,resources=customclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hackathon.kaiyuanshe.cn,resources=customclusters/status,verbs=get;update;patch

func (r *CustomClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("customcluster", req.NamespacedName)

	ctx := context.Background()
	result := results.NewResults(ctx)
	cluster, err := r.fetchCustomCluster(ctx, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}

	status := customcluster.NewStatus(cluster)
	reconcileResult := r.internalReconcile(ctx, cluster, status)
	err = r.updateStatus(ctx, status)
	return result.WithError(err).WithResult(reconcileResult).Aggregate()
}

func (r *CustomClusterReconciler) fetchCustomCluster(ctx context.Context, name types.NamespacedName) (*hackathonv1.CustomCluster, error) {
	cluster := &hackathonv1.CustomCluster{}
	if err := r.Get(ctx, name, cluster); err != nil {
		r.Log.Error(err, "get custom cluster cr failed")
		return nil, err
	}
	return cluster, nil
}

func (r *CustomClusterReconciler) ReconcileCompatibility(cluster *hackathonv1.CustomCluster) bool {
	return true
}

func (r *CustomClusterReconciler) internalReconcile(ctx context.Context, cluster *hackathonv1.CustomCluster, status *customcluster.Status) *results.Results {
	result := results.NewResults(ctx)

	if r.isMarkedForDeletion(cluster) {
		eventbus.Publish(eventbus.CustomClusterDeletedTopic, cluster)
		return result
	}

	warnings := cluster.CheckForWarning()
	if warnings != nil {
		r.Log.Info("cluster validation has warning",
			"namespace", cluster.Namespace,
			"name", cluster.Name,
			"warning", warnings.Error(),
		)
		status.AddEvent(corev1.EventTypeWarning, event.ReasonValidation, warnings.Error())
	}

	driver := customcluster.Driver{
		Client:   r.Client,
		Cluster:  cluster,
		Recorder: r.Recorder,
		Log:      r.Log.WithName("ClusterDriver"),
	}
	reconcileResult := driver.Reconcile(ctx, status)
	return result.WithResult(reconcileResult)
}

func (r *CustomClusterReconciler) isMarkedForDeletion(cluster *hackathonv1.CustomCluster) bool {
	return !cluster.ObjectMeta.DeletionTimestamp.IsZero()
}

func (r *CustomClusterReconciler) updateStatus(ctx context.Context, status *customcluster.Status) error {
	events, crt := status.Apply()
	if crt == nil {
		return nil
	}

	for _, evt := range events {
		r.Recorder.Event(crt, evt.EventType, evt.Reason, evt.Message)
	}

	r.Log.Info("update custom cluster status",
		"namespace", crt.Namespace,
		"name", crt.Name,
	)
	return r.Client.Status().Update(ctx, crt)
}

func (r *CustomClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hackathonv1.CustomCluster{}).
		Complete(r)
}
