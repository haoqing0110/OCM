package hub

import (
	"context"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"

	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	commonoptions "open-cluster-management.io/ocm/pkg/common/options"
	controllers "open-cluster-management.io/ocm/pkg/placement/controllers"
	"open-cluster-management.io/ocm/pkg/placement/debugger"
	"open-cluster-management.io/ocm/pkg/version"
)

func NewPlacementController() *cobra.Command {
	opts := commonoptions.NewOptions()

	// Store shared informers to pass to controller
	var sharedInformers clusterinformers.SharedInformerFactory

	// Create a custom RunControllerManager that uses shared informers
	runControllerManagerWithShared := func(ctx context.Context, controllerContext *controllercmd.ControllerContext) error {
		return controllers.RunControllerManagerWithSharedInformers(ctx, controllerContext, sharedInformers)
	}

	cmdConfig := opts.
		NewControllerCommandConfig("placement", version.Get(), runControllerManagerWithShared, clock.RealClock{})
	cmd := cmdConfig.NewCommandWithContext(context.TODO())
	cmd.Use = "controller"
	cmd.Short = "Start the Placement Scheduling Controller"

	// Add debugger port flag
	var debuggerPort int
	flags := cmd.Flags()
	opts.AddFlags(flags)
	flags.IntVar(&debuggerPort, "debugger-port", 9443, "Port for debugger HTTP service")

	// Wrap the original Run function to start debugger service first
	originalRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		// Start independent debugger service on all pods (before leader election)
		var err error
		sharedInformers, err = debugger.StartDebuggerService(ctx, opts.QPS, opts.Burst, debuggerPort)
		if err != nil {
			klog.Errorf("Failed to start debugger service: %v", err)
			// Don't return error, let controller continue (will create its own informers)
		} else if sharedInformers != nil {
			klog.Info("Debugger informers will be shared with controller manager")
		}

		// Continue with original startup logic (will be blocked by leader election)
		if originalRunE != nil {
			return originalRunE(cmd, args)
		}
		return cmdConfig.StartController(ctx)
	}

	return cmd
}
