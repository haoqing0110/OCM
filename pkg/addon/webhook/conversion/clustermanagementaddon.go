package conversion

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
)

// Hub interface implementations for ClusterManagementAddOn

// ClusterManagementAddOnV1Beta1 implements the Hub interface for v1beta1 ClusterManagementAddOn
type ClusterManagementAddOnV1Beta1 struct {
	*addonv1beta1.ClusterManagementAddOn
}

// Hub marks this as a hub type
func (r *ClusterManagementAddOnV1Beta1) Hub() {}

// ClusterManagementAddOnV1Alpha1 implements the Convertible interface
type ClusterManagementAddOnV1Alpha1 struct {
	*addonv1alpha1.ClusterManagementAddOn
}

// ConvertTo converts this ClusterManagementAddOn to the Hub version (v1beta1)
func (src *ClusterManagementAddOnV1Alpha1) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*ClusterManagementAddOnV1Beta1)
	return ConvertClusterManagementAddOnToV1Beta1(src.ClusterManagementAddOn, dst.ClusterManagementAddOn)
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha1)
func (dst *ClusterManagementAddOnV1Alpha1) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*ClusterManagementAddOnV1Beta1)
	return ConvertClusterManagementAddOnFromV1Beta1(src.ClusterManagementAddOn, dst.ClusterManagementAddOn)
}

// Core conversion functions for ClusterManagementAddOn

// ConvertClusterManagementAddOnToV1Beta1 converts v1alpha1 ClusterManagementAddOn to v1beta1
func ConvertClusterManagementAddOnToV1Beta1(src *addonv1alpha1.ClusterManagementAddOn, dst *addonv1beta1.ClusterManagementAddOn) error {
	// Convert TypeMeta
	dst.TypeMeta = src.TypeMeta
	dst.APIVersion = "addon.open-cluster-management.io/v1beta1"
	dst.Kind = "ClusterManagementAddOn"

	// Convert ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Convert Spec
	dst.Spec.AddOnMeta = addonv1beta1.AddOnMeta{
		DisplayName: src.Spec.AddOnMeta.DisplayName,
		Description: src.Spec.AddOnMeta.Description,
	}

	// Convert deprecated SupportedConfigs to DefaultConfigs
	dst.Spec.DefaultConfigs = convertConfigMetaToAddOnConfig(src.Spec.SupportedConfigs)

	// Convert InstallStrategy
	dst.Spec.InstallStrategy = addonv1beta1.InstallStrategy{
		Type:       src.Spec.InstallStrategy.Type,
		Placements: convertPlacementStrategies(src.Spec.InstallStrategy.Placements),
	}

	// Convert Status
	dst.Status.DefaultConfigReferences = convertDefaultConfigReferences(src.Status.DefaultConfigReferences)
	dst.Status.InstallProgressions = convertInstallProgressions(src.Status.InstallProgressions)

	return nil
}

// ConvertClusterManagementAddOnFromV1Beta1 converts v1beta1 ClusterManagementAddOn to v1alpha1
func ConvertClusterManagementAddOnFromV1Beta1(src *addonv1beta1.ClusterManagementAddOn, dst *addonv1alpha1.ClusterManagementAddOn) error {
	// Convert TypeMeta
	dst.TypeMeta = src.TypeMeta
	dst.APIVersion = "addon.open-cluster-management.io/v1alpha1"
	dst.Kind = "ClusterManagementAddOn"

	// Convert ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Convert Spec
	dst.Spec.AddOnMeta = addonv1alpha1.AddOnMeta{
		DisplayName: src.Spec.AddOnMeta.DisplayName,
		Description: src.Spec.AddOnMeta.Description,
	}

	// Convert DefaultConfigs back to SupportedConfigs (deprecated pattern)
	dst.Spec.SupportedConfigs = convertAddOnConfigToConfigMeta(src.Spec.DefaultConfigs)

	// Clear deprecated AddOnConfiguration field
	dst.Spec.AddOnConfiguration = addonv1alpha1.ConfigCoordinates{}

	// Convert InstallStrategy
	dst.Spec.InstallStrategy = addonv1alpha1.InstallStrategy{
		Type:       src.Spec.InstallStrategy.Type,
		Placements: convertPlacementStrategiesFrom(src.Spec.InstallStrategy.Placements),
	}

	// Convert Status
	dst.Status.DefaultConfigReferences = convertDefaultConfigReferencesFrom(src.Status.DefaultConfigReferences)
	dst.Status.InstallProgressions = convertInstallProgressionsFrom(src.Status.InstallProgressions)

	return nil
}
