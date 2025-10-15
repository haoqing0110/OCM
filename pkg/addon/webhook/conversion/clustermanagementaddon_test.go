package conversion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
)

func TestClusterManagementAddOnConversion(t *testing.T) {
	tests := []struct {
		name string
		src  *addonv1alpha1.ClusterManagementAddOn
		want *addonv1beta1.ClusterManagementAddOn
	}{
		{
			name: "basic conversion with supported configs",
			src: &addonv1alpha1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-addon",
					Namespace: "test-ns",
				},
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					AddOnMeta: addonv1alpha1.AddOnMeta{
						DisplayName: "Test AddOn",
						Description: "Test addon for conversion",
					},
					SupportedConfigs: []addonv1alpha1.ConfigMeta{
						{
							ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
								Group:    "addon.open-cluster-management.io",
								Resource: "addondeploymentconfigs",
							},
							DefaultConfig: &addonv1alpha1.ConfigReferent{
								Namespace: "default",
								Name:      "test-config",
							},
						},
					},
				},
			},
			want: &addonv1beta1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-addon",
					Namespace: "test-ns",
				},
				Spec: addonv1beta1.ClusterManagementAddOnSpec{
					AddOnMeta: addonv1beta1.AddOnMeta{
						DisplayName: "Test AddOn",
						Description: "Test addon for conversion",
					},
					DefaultConfigs: []addonv1beta1.AddOnConfig{
						{
							ConfigGroupResource: addonv1beta1.ConfigGroupResource{
								Group:    "addon.open-cluster-management.io",
								Resource: "addondeploymentconfigs",
							},
							ConfigReferent: addonv1beta1.ConfigReferent{
								Namespace: "default",
								Name:      "test-config",
							},
						},
					},
				},
			},
		},
		{
			name: "conversion without default config",
			src: &addonv1alpha1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-addon-no-config",
				},
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					AddOnMeta: addonv1alpha1.AddOnMeta{
						DisplayName: "Test AddOn No Config",
					},
					SupportedConfigs: []addonv1alpha1.ConfigMeta{
						{
							ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
								Group:    "addon.open-cluster-management.io",
								Resource: "addontemplates",
							},
						},
					},
				},
			},
			want: &addonv1beta1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-addon-no-config",
				},
				Spec: addonv1beta1.ClusterManagementAddOnSpec{
					AddOnMeta: addonv1beta1.AddOnMeta{
						DisplayName: "Test AddOn No Config",
					},
					DefaultConfigs: []addonv1beta1.AddOnConfig{
						{
							ConfigGroupResource: addonv1beta1.ConfigGroupResource{
								Group:    "addon.open-cluster-management.io",
								Resource: "addontemplates",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := &addonv1beta1.ClusterManagementAddOn{}
			err := ConvertClusterManagementAddOnToV1Beta1(tt.src, dst)
			assert.NoError(t, err)
			assert.Equal(t, tt.want.ObjectMeta, dst.ObjectMeta)
			assert.Equal(t, tt.want.Spec.AddOnMeta, dst.Spec.AddOnMeta)
			assert.Equal(t, tt.want.Spec.DefaultConfigs, dst.Spec.DefaultConfigs)
		})
	}
}

func TestClusterManagementAddOnConversionRoundtrip(t *testing.T) {
	original := &addonv1alpha1.ClusterManagementAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name: "roundtrip-test",
		},
		Spec: addonv1alpha1.ClusterManagementAddOnSpec{
			AddOnMeta: addonv1alpha1.AddOnMeta{
				DisplayName: "Roundtrip Test AddOn",
				Description: "Test roundtrip conversion",
			},
			SupportedConfigs: []addonv1alpha1.ConfigMeta{
				{
					ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
						Group:    "addon.open-cluster-management.io",
						Resource: "addondeploymentconfigs",
					},
					DefaultConfig: &addonv1alpha1.ConfigReferent{
						Namespace: "test",
						Name:      "config",
					},
				},
			},
		},
	}

	// Convert to v1beta1
	v1beta1Obj := &addonv1beta1.ClusterManagementAddOn{}
	err := ConvertClusterManagementAddOnToV1Beta1(original, v1beta1Obj)
	assert.NoError(t, err)

	// Convert back to v1alpha1
	converted := &addonv1alpha1.ClusterManagementAddOn{}
	err = ConvertClusterManagementAddOnFromV1Beta1(v1beta1Obj, converted)
	assert.NoError(t, err)

	// Verify roundtrip preservation
	assert.Equal(t, original.ObjectMeta, converted.ObjectMeta)
	assert.Equal(t, original.Spec.AddOnMeta, converted.Spec.AddOnMeta)
	assert.Equal(t, original.Spec.SupportedConfigs, converted.Spec.SupportedConfigs)
}

func TestClusterManagementAddOnHubInterfaceImplementation(t *testing.T) {
	// Test that Hub methods exist (compile-time check)
	v1beta1CMA := &ClusterManagementAddOnV1Beta1{ClusterManagementAddOn: &addonv1beta1.ClusterManagementAddOn{}}

	// This should compile without error
	v1beta1CMA.Hub()

	// Test that conversion methods exist
	v1alpha1CMA := &ClusterManagementAddOnV1Alpha1{ClusterManagementAddOn: &addonv1alpha1.ClusterManagementAddOn{}}

	// These should compile without error
	err := v1alpha1CMA.ConvertTo(v1beta1CMA)
	assert.NoError(t, err)

	err = v1alpha1CMA.ConvertFrom(v1beta1CMA)
	assert.NoError(t, err)
}
