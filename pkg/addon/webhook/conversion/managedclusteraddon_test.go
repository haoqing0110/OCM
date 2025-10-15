package conversion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
)

func TestManagedClusterAddOnConversion(t *testing.T) {
	tests := []struct {
		name string
		src  *addonv1alpha1.ManagedClusterAddOn
		want *addonv1beta1.ManagedClusterAddOn
	}{
		{
			name: "basic managed cluster addon conversion",
			src: &addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-managed-addon",
					Namespace: "cluster1",
				},
				Spec: addonv1alpha1.ManagedClusterAddOnSpec{
					InstallNamespace: "addon-ns",
					Configs: []addonv1alpha1.AddOnConfig{
						{
							ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
								Group:    "addon.open-cluster-management.io",
								Resource: "addondeploymentconfigs",
							},
							ConfigReferent: addonv1alpha1.ConfigReferent{
								Namespace: "config-ns",
								Name:      "config-name",
							},
						},
					},
				},
			},
			want: &addonv1beta1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-managed-addon",
					Namespace: "cluster1",
				},
				Spec: addonv1beta1.ManagedClusterAddOnSpec{
					Configs: []addonv1beta1.AddOnConfig{
						{
							ConfigGroupResource: addonv1beta1.ConfigGroupResource{
								Group:    "addon.open-cluster-management.io",
								Resource: "addondeploymentconfigs",
							},
							ConfigReferent: addonv1beta1.ConfigReferent{
								Namespace: "config-ns",
								Name:      "config-name",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := &addonv1beta1.ManagedClusterAddOn{}
			err := ConvertManagedClusterAddOnToV1Beta1(tt.src, dst)
			assert.NoError(t, err)
			assert.Equal(t, tt.want.ObjectMeta.Name, dst.ObjectMeta.Name)
			assert.Equal(t, tt.want.ObjectMeta.Namespace, dst.ObjectMeta.Namespace)
			assert.Equal(t, tt.want.Spec, dst.Spec)
			// Check that the InstallNamespace is preserved as annotation
			if tt.src.Spec.InstallNamespace != "" {
				assert.Equal(t, tt.src.Spec.InstallNamespace, dst.Annotations["addon.open-cluster-management.io/v1alpha1-install-namespace"])
			}
		})
	}
}

func TestManagedClusterAddOnConversionRoundtrip(t *testing.T) {
	original := &addonv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "roundtrip-managed-test",
			Namespace: "cluster1",
		},
		Spec: addonv1alpha1.ManagedClusterAddOnSpec{
			InstallNamespace: "addon-install-ns",
			Configs: []addonv1alpha1.AddOnConfig{
				{
					ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
						Group:    "addon.open-cluster-management.io",
						Resource: "addontemplates",
					},
					ConfigReferent: addonv1alpha1.ConfigReferent{
						Namespace: "template-ns",
						Name:      "template-name",
					},
				},
			},
		},
	}

	// Convert to v1beta1
	v1beta1Obj := &addonv1beta1.ManagedClusterAddOn{}
	err := ConvertManagedClusterAddOnToV1Beta1(original, v1beta1Obj)
	assert.NoError(t, err)

	// Convert back to v1alpha1
	converted := &addonv1alpha1.ManagedClusterAddOn{}
	err = ConvertManagedClusterAddOnFromV1Beta1(v1beta1Obj, converted)
	assert.NoError(t, err)

	// Verify roundtrip preservation
	assert.Equal(t, original.ObjectMeta.Name, converted.ObjectMeta.Name)
	assert.Equal(t, original.ObjectMeta.Namespace, converted.ObjectMeta.Namespace)
	assert.Equal(t, original.Spec, converted.Spec)
}

func TestManagedClusterAddOnHubInterfaceImplementation(t *testing.T) {
	// Test that Hub methods exist (compile-time check)
	v1beta1MCA := &ManagedClusterAddOnV1Beta1{ManagedClusterAddOn: &addonv1beta1.ManagedClusterAddOn{}}

	// This should compile without error
	v1beta1MCA.Hub()

	// Test that conversion methods exist
	v1alpha1MCA := &ManagedClusterAddOnV1Alpha1{ManagedClusterAddOn: &addonv1alpha1.ManagedClusterAddOn{}}

	// These should compile without error
	err := v1alpha1MCA.ConvertTo(v1beta1MCA)
	assert.NoError(t, err)

	err = v1alpha1MCA.ConvertFrom(v1beta1MCA)
	assert.NoError(t, err)
}
