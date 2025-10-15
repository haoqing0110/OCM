package webhook

import (
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"

	addonwebhook "open-cluster-management.io/ocm/pkg/addon/webhook"
	commonoptions "open-cluster-management.io/ocm/pkg/common/options"
)

func NewAddonWebhook() *cobra.Command {
	webhookOptions := commonoptions.NewWebhookOptions()
	cmd := &cobra.Command{
		Use:   "conversion-webhook",
		Short: "Start the addon API conversion webhook server",
		RunE: func(c *cobra.Command, args []string) error {
			if err := addonwebhook.SetupWebhookServer(webhookOptions); err != nil {
				return err
			}
			return webhookOptions.RunWebhookServer(ctrl.SetupSignalHandler())
		},
	}

	flags := cmd.Flags()
	webhookOptions.AddFlags(flags)

	return cmd
}
