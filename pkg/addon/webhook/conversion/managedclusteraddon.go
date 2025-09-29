package conversion

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
)

// Hub interface implementations for ManagedClusterAddOn

// ManagedClusterAddOnV1Beta1 implements the Hub interface for v1beta1 ManagedClusterAddOn
type ManagedClusterAddOnV1Beta1 struct {
	*addonv1beta1.ManagedClusterAddOn
}

// Hub marks this as a hub type
func (r *ManagedClusterAddOnV1Beta1) Hub() {}

// ManagedClusterAddOnV1Alpha1 implements the Convertible interface
type ManagedClusterAddOnV1Alpha1 struct {
	*addonv1alpha1.ManagedClusterAddOn
}

// ConvertTo converts this ManagedClusterAddOn to the Hub version (v1beta1)
func (src *ManagedClusterAddOnV1Alpha1) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*ManagedClusterAddOnV1Beta1)
	return ConvertManagedClusterAddOnToV1Beta1(src.ManagedClusterAddOn, dst.ManagedClusterAddOn)
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha1)
func (dst *ManagedClusterAddOnV1Alpha1) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*ManagedClusterAddOnV1Beta1)
	return ConvertManagedClusterAddOnFromV1Beta1(src.ManagedClusterAddOn, dst.ManagedClusterAddOn)
}

// Core conversion functions for ManagedClusterAddOn

// ConvertManagedClusterAddOnToV1Beta1 converts v1alpha1 ManagedClusterAddOn to v1beta1
func ConvertManagedClusterAddOnToV1Beta1(src *addonv1alpha1.ManagedClusterAddOn, dst *addonv1beta1.ManagedClusterAddOn) error {
	// Convert ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Preserve InstallNamespace as an annotation for backwards compatibility
	if dst.Annotations == nil {
		dst.Annotations = make(map[string]string)
	}
	if src.Spec.InstallNamespace != "" {
		dst.Annotations["addon.open-cluster-management.io/v1alpha1-install-namespace"] = src.Spec.InstallNamespace
	}

	// Convert Spec - InstallNamespace is deprecated in v1beta1, so we skip it
	// Convert configs from old format to new format
	for _, config := range src.Spec.Configs {
		dst.Spec.Configs = append(dst.Spec.Configs, addonv1beta1.AddOnConfig{
			ConfigGroupResource: addonv1beta1.ConfigGroupResource{
				Group:    config.Group,
				Resource: config.Resource,
			},
			ConfigReferent: addonv1beta1.ConfigReferent{
				Namespace: config.Namespace,
				Name:      config.Name,
			},
		})
	}

	// Convert Status
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.Namespace = src.Status.Namespace
	dst.Status.SupportedConfigs = convertSupportedConfigs(src.Status.SupportedConfigs)
	dst.Status.ConfigReferences = convertManagedClusterAddOnConfigReferences(src.Status.ConfigReferences)
	// Note: RegistrationApplied field doesn't exist in v1beta1, so we skip it

	// Convert health check mode
	dst.Status.HealthCheck.Mode = addonv1beta1.HealthCheckMode(src.Status.HealthCheck.Mode)

	// Convert other status fields
	dst.Status.AddOnMeta = addonv1beta1.AddOnMeta{
		DisplayName: src.Status.AddOnMeta.DisplayName,
		Description: src.Status.AddOnMeta.Description,
	}
	dst.Status.RelatedObjects = convertRelatedObjects(src.Status.RelatedObjects)
	dst.Status.Registrations = convertRegistrations(src.Status.Registrations)

	return nil
}

// ConvertManagedClusterAddOnFromV1Beta1 converts v1beta1 ManagedClusterAddOn to v1alpha1
func ConvertManagedClusterAddOnFromV1Beta1(src *addonv1beta1.ManagedClusterAddOn, dst *addonv1alpha1.ManagedClusterAddOn) error {
	// Convert ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	// Convert Spec - restore the deprecated InstallNamespace field from annotation
	if src.Annotations != nil {
		if installNS, exists := src.Annotations["addon.open-cluster-management.io/v1alpha1-install-namespace"]; exists {
			dst.Spec.InstallNamespace = installNS
			// Clean up the annotation to avoid duplication
			if dst.Annotations == nil {
				dst.Annotations = make(map[string]string)
			}
			for k, v := range src.Annotations {
				if k != "addon.open-cluster-management.io/v1alpha1-install-namespace" {
					dst.Annotations[k] = v
				}
			}
			// Remove the annotation map if it's empty
			if len(dst.Annotations) == 0 {
				dst.Annotations = nil
			}
		}
	}
	// If no annotation found, use default value
	if dst.Spec.InstallNamespace == "" {
		dst.Spec.InstallNamespace = "open-cluster-management-agent-addon"
	}

	// Convert configs from new format to old format
	for _, config := range src.Spec.Configs {
		dst.Spec.Configs = append(dst.Spec.Configs, addonv1alpha1.AddOnConfig{
			ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
				Group:    config.Group,
				Resource: config.Resource,
			},
			ConfigReferent: addonv1alpha1.ConfigReferent{
				Namespace: config.Namespace,
				Name:      config.Name,
			},
		})
	}

	// Convert Status
	dst.Status.Conditions = src.Status.Conditions
	dst.Status.Namespace = src.Status.Namespace
	dst.Status.SupportedConfigs = convertSupportedConfigsFrom(src.Status.SupportedConfigs)
	dst.Status.ConfigReferences = convertManagedClusterAddOnConfigReferencesFrom(src.Status.ConfigReferences)
	// Note: RegistrationApplied field doesn't exist in v1beta1, so we can't restore it

	// Convert health check mode
	dst.Status.HealthCheck.Mode = addonv1alpha1.HealthCheckMode(src.Status.HealthCheck.Mode)

	// Convert other status fields
	dst.Status.AddOnMeta = addonv1alpha1.AddOnMeta{
		DisplayName: src.Status.AddOnMeta.DisplayName,
		Description: src.Status.AddOnMeta.Description,
	}
	dst.Status.RelatedObjects = convertRelatedObjectsFrom(src.Status.RelatedObjects)
	dst.Status.Registrations = convertRegistrationsFrom(src.Status.Registrations)

	return nil
}
