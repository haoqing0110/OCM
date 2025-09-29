package webhook

import (
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"

	"open-cluster-management.io/ocm/pkg/addon/webhook/conversion"
	commonoptions "open-cluster-management.io/ocm/pkg/common/options"
)

func SetupWebhookServer(opts *commonoptions.WebhookOptions) error {
	if err := opts.InstallScheme(
		clientgoscheme.AddToScheme,
		addonv1alpha1.Install,
		addonv1beta1.Install,
	); err != nil {
		return err
	}

	// Install conversion webhooks
	opts.InstallWebhook(&conversion.ClusterManagementAddOnConversionWebhook{})
	opts.InstallWebhook(&conversion.ManagedClusterAddOnConversionWebhook{})

	return nil
}
