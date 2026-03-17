package hub

import (
	"context"
	"os"
	"time"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"

	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterscheme "open-cluster-management.io/api/client/cluster/clientset/versioned/scheme"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	"open-cluster-management.io/sdk-go/pkg/basecontroller/events"

	"open-cluster-management.io/ocm/pkg/placement/controllers/metrics"
	"open-cluster-management.io/ocm/pkg/placement/controllers/scheduling"
)

// RunControllerManager starts the controllers on hub to make placement decisions.
func RunControllerManager(ctx context.Context, controllerContext *controllercmd.ControllerContext) error {
	return RunControllerManagerWithSharedInformers(ctx, controllerContext, nil)
}

// RunControllerManagerWithSharedInformers starts the controllers with optional shared informers
// If sharedInformers is nil, it will create new informers
func RunControllerManagerWithSharedInformers(
	ctx context.Context,
	controllerContext *controllercmd.ControllerContext,
	sharedInformers clusterinformers.SharedInformerFactory,
) error {
	// setting up contextual logger
	logger := klog.NewKlogr()
	podName := os.Getenv("POD_NAME")
	if podName != "" {
		logger = logger.WithValues("podName", podName)
	}
	ctx = klog.NewContext(ctx, logger)

	clusterClient, err := clusterclient.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(controllerContext.KubeConfig)
	if err != nil {
		return err
	}

	// Use shared informers if provided, otherwise create new ones
	clusterInformers := sharedInformers
	if clusterInformers == nil {
		// Fallback: create new informers if not shared (e.g., debugger disabled)
		klog.Info("Shared informers not available, creating new informers")
		clusterInformers = clusterinformers.NewSharedInformerFactory(clusterClient, 10*time.Minute)
	} else {
		klog.Info("Reusing shared informers from debugger service")
	}

	return RunControllerManagerWithInformers(ctx, controllerContext, kubeClient, clusterClient, clusterInformers)
}

func RunControllerManagerWithInformers(
	ctx context.Context,
	controllerContext *controllercmd.ControllerContext,
	kubeClient kubernetes.Interface,
	clusterClient clusterclient.Interface,
	clusterInformers clusterinformers.SharedInformerFactory,
) error {
	recorder, err := events.NewEventRecorder(ctx, clusterscheme.Scheme, kubeClient.EventsV1(), "placement-controller")
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
		recorder, metrics,
	)

	go clusterInformers.Start(ctx.Done())

	go schedulingController.Run(ctx, 1)

	<-ctx.Done()

	return nil
}
