package hub

import (
	"context"
	"net/http"
	"time"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"k8s.io/apiserver/pkg/server/mux"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/clock"

	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterscheme "open-cluster-management.io/api/client/cluster/clientset/versioned/scheme"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	kueueclient "sigs.k8s.io/kueue/client-go/clientset/versioned"
	kueueinformers "sigs.k8s.io/kueue/client-go/informers/externalversions"

	"open-cluster-management.io/ocm/pkg/common/helpers"
	"open-cluster-management.io/ocm/pkg/placement/controllers/metrics"
	"open-cluster-management.io/ocm/pkg/placement/controllers/scheduling"
	"open-cluster-management.io/ocm/pkg/placement/debugger"

	"open-cluster-management.io/ocm/pkg/placement/index"
	cpclientset "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
	cpinformerv1alpha1 "sigs.k8s.io/cluster-inventory-api/client/informers/externalversions"
)

// RunControllerManager starts the controllers on hub to make placement decisions.
func RunControllerManager(ctx context.Context, controllerContext *controllercmd.ControllerContext) error {
	clusterClient, err := clusterclient.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	kueueClient, err := kueueclient.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	clusterProfileClient, err := cpclientset.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	clusterInformers := clusterinformers.NewSharedInformerFactory(clusterClient, 10*time.Minute)
	kueueInformers := kueueinformers.NewSharedInformerFactory(kueueClient, 10*time.Minute)
	clusterProfileInformers := cpinformerv1alpha1.NewSharedInformerFactory(clusterProfileClient, 30*time.Minute)

	return RunControllerManagerWithInformers(ctx, controllerContext, kubeClient, clusterClient, kueueClient, clusterInformers, clusterProfileInformers, kueueInformers)
}

func RunControllerManagerWithInformers(
	ctx context.Context,
	controllerContext *controllercmd.ControllerContext,
	kubeClient kubernetes.Interface,
	clusterClient clusterclient.Interface,
	kueueClient *kueueclient.Clientset,
	clusterInformers clusterinformers.SharedInformerFactory,
	clusterProfileInformers cpinformerv1alpha1.SharedInformerFactory,
	kueueInformers kueueinformers.SharedInformerFactory,
) error {
	recorder, err := helpers.NewEventRecorder(ctx, clusterscheme.Scheme, kubeClient, "placement-controller")
	if err != nil {
		return err
	}

	// admissionChecksController
	err = kueueInformers.Kueue().V1beta1().AdmissionChecks().Informer().AddIndexers(
		cache.Indexers{
			index.AdmissionCheckByPlacement: index.IndexAdmissionCheckByPlacement,
		})
	if err != nil {
		return err
	}

	metrics := metrics.NewScheduleMetrics(clock.RealClock{})

	scheduler := scheduling.NewPluginScheduler(
		scheduling.NewSchedulerHandler(
			clusterClient,
			clusterInformers.Cluster().V1beta1().PlacementDecisions().Lister(),
			clusterInformers.Cluster().V1alpha1().AddOnPlacementScores().Lister(),
			clusterInformers.Cluster().V1().ManagedClusters().Lister(),
			recorder, metrics),
	)

	if controllerContext.Server != nil {
		debug := debugger.NewDebugger(
			scheduler,
			clusterInformers.Cluster().V1beta1().Placements(),
			clusterInformers.Cluster().V1().ManagedClusters(),
		)

		installDebugger(controllerContext.Server.Handler.NonGoRestfulMux, debug)
	}

	schedulingController := scheduling.NewSchedulingController(
		ctx,
		clusterClient,
		clusterInformers.Cluster().V1().ManagedClusters(),
		clusterInformers.Cluster().V1beta2().ManagedClusterSets(),
		clusterInformers.Cluster().V1beta2().ManagedClusterSetBindings(),
		clusterInformers.Cluster().V1beta1().Placements(),
		clusterInformers.Cluster().V1beta1().PlacementDecisions(),
		clusterInformers.Cluster().V1alpha1().AddOnPlacementScores(),
		scheduler,
		controllerContext.EventRecorder, recorder, metrics,
	)
	// TODO: featuregates
	admissionCheckController := scheduling.NewAdmissionCheckController(
		ctx,
		clusterClient,
		kueueClient,
		clusterProfileInformers.Apis().V1alpha1().ClusterProfiles(),
		clusterInformers.Cluster().V1beta1().Placements(),
		clusterInformers.Cluster().V1beta1().PlacementDecisions(),
		kueueInformers.Kueue().V1beta1().AdmissionChecks(),
		controllerContext.EventRecorder, recorder,
	)

	go clusterInformers.Start(ctx.Done())
	go clusterProfileInformers.Start(ctx.Done())
	go kueueInformers.Start(ctx.Done())

	go schedulingController.Run(ctx, 1)
	// TODO: featuregates
	go admissionCheckController.Run(ctx, 1)

	<-ctx.Done()

	return nil
}

func installDebugger(mux *mux.PathRecorderMux, d *debugger.Debugger) {
	mux.HandlePrefix(debugger.DebugPath, http.HandlerFunc(d.Handler))
}
