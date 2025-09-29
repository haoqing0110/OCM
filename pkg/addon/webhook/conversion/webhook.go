package conversion

import (
	ctrl "sigs.k8s.io/controller-runtime"
)

// ClusterManagementAddOnConversionWebhook implements conversion webhook for ClusterManagementAddOn
type ClusterManagementAddOnConversionWebhook struct{}

func (w *ClusterManagementAddOnConversionWebhook) Init(mgr ctrl.Manager) error {
	return w.SetupWebhookWithManager(mgr)
}

func (w *ClusterManagementAddOnConversionWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&ClusterManagementAddOnV1Alpha1{}).
		Complete()
}

// ManagedClusterAddOnConversionWebhook implements conversion webhook for ManagedClusterAddOn
type ManagedClusterAddOnConversionWebhook struct{}

func (w *ManagedClusterAddOnConversionWebhook) Init(mgr ctrl.Manager) error {
	return w.SetupWebhookWithManager(mgr)
}

func (w *ManagedClusterAddOnConversionWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&ManagedClusterAddOnV1Alpha1{}).
		Complete()
}
