package debugger

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"

	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterscheme "open-cluster-management.io/api/client/cluster/clientset/versioned/scheme"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	"open-cluster-management.io/sdk-go/pkg/basecontroller/events"

	"open-cluster-management.io/ocm/pkg/placement/controllers/metrics"
	"open-cluster-management.io/ocm/pkg/placement/controllers/scheduling"
)

// StartDebuggerService starts an independent debugger HTTP service on all pods
// This function is called before leader election, so it runs on all pods
// Returns the created clusterInformers so caller can share them with controller
func StartDebuggerService(ctx context.Context, qps float32, burst int, port int) (clusterinformers.SharedInformerFactory, error) {
	logger := klog.NewKlogr()
	podName := os.Getenv("POD_NAME")
	if podName != "" {
		logger = logger.WithValues("podName", podName)
	}
	logger = logger.WithName("debugger-service")

	// Build kubeconfig (try in-cluster first, then fall back to kubeconfig file)
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	// Apply QPS and Burst settings
	kubeConfig.QPS = qps
	kubeConfig.Burst = burst

	// Create clients
	clusterClient, err := clusterclient.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster client: %w", err)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube client: %w", err)
	}

	// Create informers
	clusterInformers := clusterinformers.NewSharedInformerFactory(clusterClient, 10*time.Minute)

	// Create event recorder and metrics
	recorder, err := events.NewEventRecorder(ctx, clusterscheme.Scheme, kubeClient.EventsV1(), "placement-debugger")
	if err != nil {
		return nil, fmt.Errorf("failed to create event recorder: %w", err)
	}
	metricsRecorder := metrics.NewScheduleMetrics(clock.RealClock{})

	// Create scheduler (for debugger only, not for actual scheduling)
	scheduler := scheduling.NewPluginScheduler(
		scheduling.NewSchedulerHandler(
			clusterClient,
			clusterInformers.Cluster().V1beta1().PlacementDecisions().Lister(),
			clusterInformers.Cluster().V1alpha1().AddOnPlacementScores().Lister(),
			clusterInformers.Cluster().V1().ManagedClusters().Lister(),
			recorder, metricsRecorder),
	)

	// Create debugger
	debug := NewDebugger(
		scheduler,
		clusterInformers.Cluster().V1beta1().Placements(),
		clusterInformers.Cluster().V1().ManagedClusters(),
	)

	// Start informers
	go clusterInformers.Start(ctx.Done())

	// Wait for informers to sync
	logger.Info("Waiting for informer caches to sync")
	cacheSyncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// WaitForCacheSync returns a map, we need to check if all caches synced
	syncResults := clusterInformers.WaitForCacheSync(cacheSyncCtx.Done())
	for informerType, synced := range syncResults {
		if !synced {
			return nil, fmt.Errorf("failed to sync informer cache for type %v", informerType)
		}
	}
	logger.Info("Informer caches synced successfully")

	// Create HTTP server
	mux := http.NewServeMux()
	mux.Handle(DebugPath, http.HandlerFunc(debug.Handler))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// Check if informers are synced
		if clusterInformers.Cluster().V1().ManagedClusters().Informer().HasSynced() &&
			clusterInformers.Cluster().V1beta1().Placements().Informer().HasSynced() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		logger.Info("Starting debugger HTTP service", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(err, "Debugger HTTP service failed")
		}
	}()

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		logger.Info("Shutting down debugger HTTP service")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error(err, "Error shutting down debugger HTTP service")
		}
	}()

	logger.Info("Debugger service started successfully on all pods", "port", port)
	return clusterInformers, nil
}
