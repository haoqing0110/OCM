package conversion

import (
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
)

// =============================================================================
// ConfigSpecHash Conversion Helpers
// =============================================================================

func convertConfigSpecHashToV1Beta1(src *addonv1alpha1.ConfigSpecHash) *addonv1beta1.ConfigSpecHash {
	if src == nil {
		return nil
	}
	return &addonv1beta1.ConfigSpecHash{
		ConfigReferent: addonv1beta1.ConfigReferent{
			Namespace: src.Namespace,
			Name:      src.Name,
		},
		SpecHash: src.SpecHash,
	}
}

func convertConfigSpecHashFromV1Beta1(src *addonv1beta1.ConfigSpecHash) *addonv1alpha1.ConfigSpecHash {
	if src == nil {
		return nil
	}
	return &addonv1alpha1.ConfigSpecHash{
		ConfigReferent: addonv1alpha1.ConfigReferent{
			Namespace: src.Namespace,
			Name:      src.Name,
		},
		SpecHash: src.SpecHash,
	}
}

// =============================================================================
// RelatedObjects Conversion Helpers
// =============================================================================

func convertRelatedObjects(objects []addonv1alpha1.ObjectReference) []addonv1beta1.ObjectReference {
	var converted []addonv1beta1.ObjectReference

	for _, obj := range objects {
		converted = append(converted, addonv1beta1.ObjectReference{
			Group:     obj.Group,
			Resource:  obj.Resource,
			Namespace: obj.Namespace,
			Name:      obj.Name,
		})
	}

	return converted
}

func convertRelatedObjectsFrom(objects []addonv1beta1.ObjectReference) []addonv1alpha1.ObjectReference {
	var converted []addonv1alpha1.ObjectReference

	for _, obj := range objects {
		converted = append(converted, addonv1alpha1.ObjectReference{
			Group:     obj.Group,
			Resource:  obj.Resource,
			Namespace: obj.Namespace,
			Name:      obj.Name,
		})
	}

	return converted
}

// =============================================================================
// Registrations Conversion Helpers
// =============================================================================

func convertRegistrations(registrations []addonv1alpha1.RegistrationConfig) []addonv1beta1.RegistrationConfig {
	var converted []addonv1beta1.RegistrationConfig

	for _, reg := range registrations {
		converted = append(converted, addonv1beta1.RegistrationConfig{
			SignerName: reg.SignerName,
			Subject: addonv1beta1.Subject{
				User:              reg.Subject.User,
				Groups:            reg.Subject.Groups,
				OrganizationUnits: reg.Subject.OrganizationUnits,
			},
		})
	}

	return converted
}

func convertRegistrationsFrom(registrations []addonv1beta1.RegistrationConfig) []addonv1alpha1.RegistrationConfig {
	var converted []addonv1alpha1.RegistrationConfig

	for _, reg := range registrations {
		converted = append(converted, addonv1alpha1.RegistrationConfig{
			SignerName: reg.SignerName,
			Subject: addonv1alpha1.Subject{
				User:              reg.Subject.User,
				Groups:            reg.Subject.Groups,
				OrganizationUnits: reg.Subject.OrganizationUnits,
			},
		})
	}

	return converted
}

// =============================================================================
// ClusterManagementAddOn Specific Helpers
// =============================================================================

func convertConfigMetaToAddOnConfig(configMetas []addonv1alpha1.ConfigMeta) []addonv1beta1.AddOnConfig {
	var addOnConfigs []addonv1beta1.AddOnConfig

	for _, configMeta := range configMetas {
		addOnConfig := addonv1beta1.AddOnConfig{
			ConfigGroupResource: addonv1beta1.ConfigGroupResource{
				Group:    configMeta.Group,
				Resource: configMeta.Resource,
			},
		}

		// If there's a default config, use it
		if configMeta.DefaultConfig != nil {
			addOnConfig.ConfigReferent = addonv1beta1.ConfigReferent{
				Namespace: configMeta.DefaultConfig.Namespace,
				Name:      configMeta.DefaultConfig.Name,
			}
		}

		addOnConfigs = append(addOnConfigs, addOnConfig)
	}

	return addOnConfigs
}

func convertAddOnConfigToConfigMeta(addOnConfigs []addonv1beta1.AddOnConfig) []addonv1alpha1.ConfigMeta {
	var configMetas []addonv1alpha1.ConfigMeta

	for _, addOnConfig := range addOnConfigs {
		configMeta := addonv1alpha1.ConfigMeta{
			ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
				Group:    addOnConfig.Group,
				Resource: addOnConfig.Resource,
			},
		}

		// If there's a config referent, convert it to default config
		if addOnConfig.Name != "" {
			configMeta.DefaultConfig = &addonv1alpha1.ConfigReferent{
				Namespace: addOnConfig.Namespace,
				Name:      addOnConfig.Name,
			}
		}

		configMetas = append(configMetas, configMeta)
	}

	return configMetas
}

func convertPlacementStrategies(placements []addonv1alpha1.PlacementStrategy) []addonv1beta1.PlacementStrategy {
	var converted []addonv1beta1.PlacementStrategy

	for _, placement := range placements {
		convertedPlacement := addonv1beta1.PlacementStrategy{
			PlacementRef: addonv1beta1.PlacementRef{
				Namespace: placement.Namespace,
				Name:      placement.Name,
			},
			RolloutStrategy: placement.RolloutStrategy,
		}

		// Convert configs
		for _, config := range placement.Configs {
			convertedPlacement.Configs = append(convertedPlacement.Configs, addonv1beta1.AddOnConfig{
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

		converted = append(converted, convertedPlacement)
	}

	return converted
}

func convertPlacementStrategiesFrom(placements []addonv1beta1.PlacementStrategy) []addonv1alpha1.PlacementStrategy {
	var converted []addonv1alpha1.PlacementStrategy

	for _, placement := range placements {
		convertedPlacement := addonv1alpha1.PlacementStrategy{
			PlacementRef: addonv1alpha1.PlacementRef{
				Namespace: placement.Namespace,
				Name:      placement.Name,
			},
			RolloutStrategy: placement.RolloutStrategy,
		}

		// Convert configs
		for _, config := range placement.Configs {
			convertedPlacement.Configs = append(convertedPlacement.Configs, addonv1alpha1.AddOnConfig{
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

		converted = append(converted, convertedPlacement)
	}

	return converted
}

func convertDefaultConfigReferences(refs []addonv1alpha1.DefaultConfigReference) []addonv1beta1.DefaultConfigReference {
	var converted []addonv1beta1.DefaultConfigReference

	for _, ref := range refs {
		converted = append(converted, addonv1beta1.DefaultConfigReference{
			ConfigGroupResource: addonv1beta1.ConfigGroupResource{
				Group:    ref.Group,
				Resource: ref.Resource,
			},
			DesiredConfig: convertConfigSpecHashToV1Beta1(ref.DesiredConfig),
		})
	}

	return converted
}

func convertDefaultConfigReferencesFrom(refs []addonv1beta1.DefaultConfigReference) []addonv1alpha1.DefaultConfigReference {
	var converted []addonv1alpha1.DefaultConfigReference

	for _, ref := range refs {
		converted = append(converted, addonv1alpha1.DefaultConfigReference{
			ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
				Group:    ref.Group,
				Resource: ref.Resource,
			},
			DesiredConfig: convertConfigSpecHashFromV1Beta1(ref.DesiredConfig),
		})
	}

	return converted
}

func convertInstallProgressions(progressions []addonv1alpha1.InstallProgression) []addonv1beta1.InstallProgression {
	var converted []addonv1beta1.InstallProgression

	for _, progression := range progressions {
		convertedProgression := addonv1beta1.InstallProgression{
			PlacementRef: addonv1beta1.PlacementRef{
				Namespace: progression.Namespace,
				Name:      progression.Name,
			},
			Conditions: progression.Conditions,
		}

		// Convert config references
		for _, configRef := range progression.ConfigReferences {
			convertedProgression.ConfigReferences = append(convertedProgression.ConfigReferences, addonv1beta1.InstallConfigReference{
				ConfigGroupResource: addonv1beta1.ConfigGroupResource{
					Group:    configRef.Group,
					Resource: configRef.Resource,
				},
				DesiredConfig:       convertConfigSpecHashToV1Beta1(configRef.DesiredConfig),
				LastKnownGoodConfig: convertConfigSpecHashToV1Beta1(configRef.LastKnownGoodConfig),
				LastAppliedConfig:   convertConfigSpecHashToV1Beta1(configRef.LastAppliedConfig),
			})
		}

		converted = append(converted, convertedProgression)
	}

	return converted
}

func convertInstallProgressionsFrom(progressions []addonv1beta1.InstallProgression) []addonv1alpha1.InstallProgression {
	var converted []addonv1alpha1.InstallProgression

	for _, progression := range progressions {
		convertedProgression := addonv1alpha1.InstallProgression{
			PlacementRef: addonv1alpha1.PlacementRef{
				Namespace: progression.Namespace,
				Name:      progression.Name,
			},
			Conditions: progression.Conditions,
		}

		// Convert config references
		for _, configRef := range progression.ConfigReferences {
			convertedProgression.ConfigReferences = append(convertedProgression.ConfigReferences, addonv1alpha1.InstallConfigReference{
				ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
					Group:    configRef.Group,
					Resource: configRef.Resource,
				},
				DesiredConfig:       convertConfigSpecHashFromV1Beta1(configRef.DesiredConfig),
				LastKnownGoodConfig: convertConfigSpecHashFromV1Beta1(configRef.LastKnownGoodConfig),
				LastAppliedConfig:   convertConfigSpecHashFromV1Beta1(configRef.LastAppliedConfig),
			})
		}

		converted = append(converted, convertedProgression)
	}

	return converted
}

// =============================================================================
// ManagedClusterAddOn Specific Helpers
// =============================================================================

func convertSupportedConfigs(configs []addonv1alpha1.ConfigGroupResource) []addonv1beta1.ConfigGroupResource {
	var converted []addonv1beta1.ConfigGroupResource

	for _, config := range configs {
		converted = append(converted, addonv1beta1.ConfigGroupResource{
			Group:    config.Group,
			Resource: config.Resource,
		})
	}

	return converted
}

func convertSupportedConfigsFrom(configs []addonv1beta1.ConfigGroupResource) []addonv1alpha1.ConfigGroupResource {
	var converted []addonv1alpha1.ConfigGroupResource

	for _, config := range configs {
		converted = append(converted, addonv1alpha1.ConfigGroupResource{
			Group:    config.Group,
			Resource: config.Resource,
		})
	}

	return converted
}

func convertManagedClusterAddOnConfigReferences(refs []addonv1alpha1.ConfigReference) []addonv1beta1.ConfigReference {
	var converted []addonv1beta1.ConfigReference

	for _, ref := range refs {
		converted = append(converted, addonv1beta1.ConfigReference{
			ConfigGroupResource: addonv1beta1.ConfigGroupResource{
				Group:    ref.Group,
				Resource: ref.Resource,
			},
			DesiredConfig:     convertConfigSpecHashToV1Beta1(ref.DesiredConfig),
			LastAppliedConfig: convertConfigSpecHashToV1Beta1(ref.LastAppliedConfig),
			// Note: LastKnownGoodConfig doesn't exist in v1beta1 ConfigReference
		})
	}

	return converted
}

func convertManagedClusterAddOnConfigReferencesFrom(refs []addonv1beta1.ConfigReference) []addonv1alpha1.ConfigReference {
	var converted []addonv1alpha1.ConfigReference

	for _, ref := range refs {
		converted = append(converted, addonv1alpha1.ConfigReference{
			ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
				Group:    ref.Group,
				Resource: ref.Resource,
			},
			DesiredConfig:     convertConfigSpecHashFromV1Beta1(ref.DesiredConfig),
			LastAppliedConfig: convertConfigSpecHashFromV1Beta1(ref.LastAppliedConfig),
		})
	}

	return converted
}
