package hub

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
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
	"open-cluster-management.io/ocm/pkg/placement/debugger"
)

// NewDebuggerCommand creates a debugger command that runs a standalone debugger HTTP service
// This is used in sidecar deployment mode where the debugger runs in a separate container
func NewDebuggerCommand() *cobra.Command {
	var bindAddress string
	var kubeconfig string
	var qps float32
	var burst int

	cmd := &cobra.Command{
		Use:   "debugger",
		Short: "Start the Placement Debugger HTTP Service",
		Long: `Start a standalone debugger HTTP service for placement scheduling debug.

This command is designed to run in a sidecar container alongside the placement controller,
providing debug endpoints without requiring leader election.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle signals
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-sigCh
				klog.Info("Received shutdown signal")
				cancel()
			}()

			return runDebuggerService(ctx, bindAddress, kubeconfig, qps, burst)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&bindAddress, "bind-address", "0.0.0.0:9443", "The address to bind the debugger HTTP server")
	flags.StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (default: in-cluster config)")
	flags.Float32Var(&qps, "kube-api-qps", 50.0, "QPS to use while talking with kubernetes apiserver")
	flags.IntVar(&burst, "kube-api-burst", 100, "Burst to use while talking with kubernetes apiserver")

	return cmd
}

func runDebuggerService(ctx context.Context, bindAddress, kubeconfigPath string, qps float32, burst int) error {
	logger := klog.NewKlogr()
	podName := os.Getenv("POD_NAME")
	if podName != "" {
		logger = logger.WithValues("podName", podName)
	}
	logger = logger.WithName("debugger-sidecar")

	klog.Info("Starting Placement Debugger Service")
	klog.Infof("Bind address: %s", bindAddress)

	// Build kubeconfig
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}
	kubeConfig.QPS = qps
	kubeConfig.Burst = burst

	// Create clients
	clusterClient, err := clusterclient.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create cluster client: %w", err)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create kube client: %w", err)
	}

	// Create informers
	clusterInformers := clusterinformers.NewSharedInformerFactory(clusterClient, 10*time.Minute)

	// Create event recorder and metrics
	recorder, err := events.NewEventRecorder(ctx, clusterscheme.Scheme, kubeClient.EventsV1(), "placement-debugger")
	if err != nil {
		return fmt.Errorf("failed to create event recorder: %w", err)
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
	debug := debugger.NewDebugger(
		scheduler,
		clusterInformers.Cluster().V1beta1().Placements(),
		clusterInformers.Cluster().V1().ManagedClusters(),
	)

	// Start informers
	go clusterInformers.Start(ctx.Done())

	// Wait for informers to sync
	klog.Info("Waiting for informer caches to sync")
	cacheSyncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	syncResults := clusterInformers.WaitForCacheSync(cacheSyncCtx.Done())
	for informerType, synced := range syncResults {
		if !synced {
			return fmt.Errorf("failed to sync informer cache for type %v", informerType)
		}
	}
	klog.Info("Informer caches synced successfully")

	// Create HTTP server
	mux := http.NewServeMux()
	mux.Handle(debugger.DebugPath, http.HandlerFunc(debug.Handler))
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
		Addr:    bindAddress,
		Handler: mux,
	}

	// Start server in goroutine
	go func() {
		klog.Infof("Starting debugger HTTP service on %s", bindAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(err, "Debugger HTTP service failed")
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()

	// Graceful shutdown
	klog.Info("Shutting down debugger HTTP service")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("failed to shutdown HTTP server: %w", err)
	}

	klog.Info("Debugger service stopped")
	return nil
}
