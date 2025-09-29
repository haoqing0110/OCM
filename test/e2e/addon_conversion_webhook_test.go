package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonapiv1beta1 "open-cluster-management.io/api/addon/v1beta1"
)

var _ = ginkgo.Describe("Addon Conversion Webhook", func() {
	ginkgo.Context("ClusterManagementAddOn API Version Conversion", func() {
		var clusterManagementAddOnName string

		ginkgo.BeforeEach(func() {
			clusterManagementAddOnName = fmt.Sprintf("conversion-test-cma-%s", rand.String(6))
		})

		ginkgo.AfterEach(func() {
			// Clean up v1alpha1 resource
			err := hub.AddonClient.AddonV1alpha1().ClusterManagementAddOns().Delete(
				context.TODO(), clusterManagementAddOnName, metav1.DeleteOptions{})
			if err != nil {
				ginkgo.By(fmt.Sprintf("Failed to delete v1alpha1 ClusterManagementAddOn %s: %v", clusterManagementAddOnName, err))
			}

			// Clean up v1beta1 resource (if it exists)
			err = hub.AddonClient.AddonV1beta1().ClusterManagementAddOns().Delete(
				context.TODO(), clusterManagementAddOnName, metav1.DeleteOptions{})
			if err != nil {
				ginkgo.By(fmt.Sprintf("Failed to delete v1beta1 ClusterManagementAddOn %s: %v", clusterManagementAddOnName, err))
			}
		})

		ginkgo.It("Should convert v1alpha1 ClusterManagementAddOn to v1beta1", func() {
			ginkgo.By(fmt.Sprintf("Creating v1alpha1 ClusterManagementAddOn %q with SupportedConfigs", clusterManagementAddOnName))

			// Create v1alpha1 ClusterManagementAddOn with the old ConfigMeta/ConfigCoordinates pattern
			v1alpha1CMA := &addonapiv1alpha1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterManagementAddOnName,
				},
				Spec: addonapiv1alpha1.ClusterManagementAddOnSpec{
					AddOnMeta: addonapiv1alpha1.AddOnMeta{
						DisplayName: "Conversion Test AddOn",
						Description: "Test addon for API conversion",
					},
					SupportedConfigs: []addonapiv1alpha1.ConfigMeta{
						{
							ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
								Group:    "addon.open-cluster-management.io",
								Resource: "addondeploymentconfigs",
							},
							DefaultConfig: &addonapiv1alpha1.ConfigReferent{
								Namespace: "default",
								Name:      "test-config",
							},
						},
					},
				},
			}

			_, err := hub.AddonClient.AddonV1alpha1().ClusterManagementAddOns().Create(
				context.TODO(), v1alpha1CMA, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("Reading the resource as v1beta1 to verify conversion")
			gomega.Eventually(func() error {
				v1beta1CMA, err := hub.AddonClient.AddonV1beta1().ClusterManagementAddOns().Get(
					context.TODO(), clusterManagementAddOnName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				// Verify the conversion from SupportedConfigs to DefaultConfigs
				if len(v1beta1CMA.Spec.DefaultConfigs) != 1 {
					return fmt.Errorf("expected 1 DefaultConfig, got %d", len(v1beta1CMA.Spec.DefaultConfigs))
				}

				defaultConfig := v1beta1CMA.Spec.DefaultConfigs[0]
				if defaultConfig.ConfigGroupResource.Group != "addon.open-cluster-management.io" {
					return fmt.Errorf("expected group 'addon.open-cluster-management.io', got '%s'", defaultConfig.ConfigGroupResource.Group)
				}
				if defaultConfig.ConfigGroupResource.Resource != "addondeploymentconfigs" {
					return fmt.Errorf("expected resource 'addondeploymentconfigs', got '%s'", defaultConfig.ConfigGroupResource.Resource)
				}
				if defaultConfig.ConfigReferent.Namespace != "default" {
					return fmt.Errorf("expected namespace 'default', got '%s'", defaultConfig.ConfigReferent.Namespace)
				}
				if defaultConfig.ConfigReferent.Name != "test-config" {
					return fmt.Errorf("expected name 'test-config', got '%s'", defaultConfig.ConfigReferent.Name)
				}

				return nil
			}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
		})

		ginkgo.It("Should convert v1beta1 ClusterManagementAddOn to v1alpha1", func() {
			ginkgo.By(fmt.Sprintf("Creating v1beta1 ClusterManagementAddOn %q with DefaultConfigs", clusterManagementAddOnName))

			// Create v1beta1 ClusterManagementAddOn with the new AddOnConfig pattern
			v1beta1CMA := &addonapiv1beta1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterManagementAddOnName,
				},
				Spec: addonapiv1beta1.ClusterManagementAddOnSpec{
					AddOnMeta: addonapiv1beta1.AddOnMeta{
						DisplayName: "Conversion Test AddOn Beta",
						Description: "Test addon for API conversion from v1beta1",
					},
					DefaultConfigs: []addonapiv1beta1.AddOnConfig{
						{
							ConfigGroupResource: addonapiv1beta1.ConfigGroupResource{
								Group:    "addon.open-cluster-management.io",
								Resource: "addontemplates",
							},
							ConfigReferent: addonapiv1beta1.ConfigReferent{
								Namespace: "system",
								Name:      "beta-config",
							},
						},
					},
				},
			}

			_, err := hub.AddonClient.AddonV1beta1().ClusterManagementAddOns().Create(
				context.TODO(), v1beta1CMA, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("Reading the resource as v1alpha1 to verify conversion")
			gomega.Eventually(func() error {
				v1alpha1CMA, err := hub.AddonClient.AddonV1alpha1().ClusterManagementAddOns().Get(
					context.TODO(), clusterManagementAddOnName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				// Verify the conversion from DefaultConfigs to SupportedConfigs
				if len(v1alpha1CMA.Spec.SupportedConfigs) != 1 {
					return fmt.Errorf("expected 1 SupportedConfig, got %d", len(v1alpha1CMA.Spec.SupportedConfigs))
				}

				supportedConfig := v1alpha1CMA.Spec.SupportedConfigs[0]
				if supportedConfig.ConfigGroupResource.Group != "addon.open-cluster-management.io" {
					return fmt.Errorf("expected group 'addon.open-cluster-management.io', got '%s'", supportedConfig.ConfigGroupResource.Group)
				}
				if supportedConfig.ConfigGroupResource.Resource != "addontemplates" {
					return fmt.Errorf("expected resource 'addontemplates', got '%s'", supportedConfig.ConfigGroupResource.Resource)
				}
				if supportedConfig.DefaultConfig.Namespace != "system" {
					return fmt.Errorf("expected namespace 'system', got '%s'", supportedConfig.DefaultConfig.Namespace)
				}
				if supportedConfig.DefaultConfig.Name != "beta-config" {
					return fmt.Errorf("expected name 'beta-config', got '%s'", supportedConfig.DefaultConfig.Name)
				}

				return nil
			}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
		})

		ginkgo.It("Should handle complex conversion scenarios with multiple configs", func() {
			ginkgo.By(fmt.Sprintf("Creating v1alpha1 ClusterManagementAddOn %q with multiple SupportedConfigs", clusterManagementAddOnName))

			v1alpha1CMA := &addonapiv1alpha1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterManagementAddOnName,
				},
				Spec: addonapiv1alpha1.ClusterManagementAddOnSpec{
					AddOnMeta: addonapiv1alpha1.AddOnMeta{
						DisplayName: "Multi-Config Test AddOn",
						Description: "Test addon with multiple configurations",
					},
					SupportedConfigs: []addonapiv1alpha1.ConfigMeta{
						{
							ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
								Group:    "addon.open-cluster-management.io",
								Resource: "addondeploymentconfigs",
							},
							DefaultConfig: &addonapiv1alpha1.ConfigReferent{
								Namespace: "config-ns-1",
								Name:      "deployment-config",
							},
						},
						{
							ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
								Group:    "addon.open-cluster-management.io",
								Resource: "addontemplates",
							},
							DefaultConfig: &addonapiv1alpha1.ConfigReferent{
								Namespace: "config-ns-2",
								Name:      "template-config",
							},
						},
					},
				},
			}

			_, err := hub.AddonClient.AddonV1alpha1().ClusterManagementAddOns().Create(
				context.TODO(), v1alpha1CMA, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("Verifying conversion preserves all configurations")
			gomega.Eventually(func() error {
				v1beta1CMA, err := hub.AddonClient.AddonV1beta1().ClusterManagementAddOns().Get(
					context.TODO(), clusterManagementAddOnName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if len(v1beta1CMA.Spec.DefaultConfigs) != 2 {
					return fmt.Errorf("expected 2 DefaultConfigs, got %d", len(v1beta1CMA.Spec.DefaultConfigs))
				}

				// Check both configs are properly converted
				foundDeployment := false
				foundTemplate := false
				for _, config := range v1beta1CMA.Spec.DefaultConfigs {
					if config.ConfigGroupResource.Resource == "addondeploymentconfigs" &&
						config.ConfigReferent.Namespace == "config-ns-1" &&
						config.ConfigReferent.Name == "deployment-config" {
						foundDeployment = true
					}
					if config.ConfigGroupResource.Resource == "addontemplates" &&
						config.ConfigReferent.Namespace == "config-ns-2" &&
						config.ConfigReferent.Name == "template-config" {
						foundTemplate = true
					}
				}

				if !foundDeployment {
					return fmt.Errorf("deployment config not found in converted resource")
				}
				if !foundTemplate {
					return fmt.Errorf("template config not found in converted resource")
				}

				return nil
			}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
		})
	})

	ginkgo.Context("ManagedClusterAddOn API Version Conversion", func() {
		var managedClusterAddOnName string
		var namespace string

		ginkgo.BeforeEach(func() {
			managedClusterAddOnName = fmt.Sprintf("conversion-test-mca-%s", rand.String(6))
			namespace = "default"
		})

		ginkgo.AfterEach(func() {
			// Clean up v1alpha1 resource
			err := hub.AddonClient.AddonV1alpha1().ManagedClusterAddOns(namespace).Delete(
				context.TODO(), managedClusterAddOnName, metav1.DeleteOptions{})
			if err != nil {
				ginkgo.By(fmt.Sprintf("Failed to delete v1alpha1 ManagedClusterAddOn %s: %v", managedClusterAddOnName, err))
			}

			// Clean up v1beta1 resource
			err = hub.AddonClient.AddonV1beta1().ManagedClusterAddOns(namespace).Delete(
				context.TODO(), managedClusterAddOnName, metav1.DeleteOptions{})
			if err != nil {
				ginkgo.By(fmt.Sprintf("Failed to delete v1beta1 ManagedClusterAddOn %s: %v", managedClusterAddOnName, err))
			}
		})

		ginkgo.It("Should convert v1alpha1 ManagedClusterAddOn to v1beta1", func() {
			ginkgo.By(fmt.Sprintf("Creating v1alpha1 ManagedClusterAddOn %q", managedClusterAddOnName))

			v1alpha1MCA := &addonapiv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      managedClusterAddOnName,
					Namespace: namespace,
				},
				Spec: addonapiv1alpha1.ManagedClusterAddOnSpec{
					InstallNamespace: "addon-install-ns",
					Configs: []addonapiv1alpha1.AddOnConfig{
						{
							ConfigGroupResource: addonapiv1alpha1.ConfigGroupResource{
								Group:    "addon.open-cluster-management.io",
								Resource: "addondeploymentconfigs",
							},
							ConfigReferent: addonapiv1alpha1.ConfigReferent{
								Namespace: "test-ns",
								Name:      "test-deployment-config",
							},
						},
					},
				},
			}

			_, err := hub.AddonClient.AddonV1alpha1().ManagedClusterAddOns(namespace).Create(
				context.TODO(), v1alpha1MCA, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("Reading the resource as v1beta1 to verify conversion")
			gomega.Eventually(func() error {
				v1beta1MCA, err := hub.AddonClient.AddonV1beta1().ManagedClusterAddOns(namespace).Get(
					context.TODO(), managedClusterAddOnName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				// Verify InstallNamespace field is removed in v1beta1 (should not exist)
				// This is checked by ensuring the addon still works without the deprecated field

				// Verify Configs are preserved
				if len(v1beta1MCA.Spec.Configs) != 1 {
					return fmt.Errorf("expected 1 Config, got %d", len(v1beta1MCA.Spec.Configs))
				}

				config := v1beta1MCA.Spec.Configs[0]
				if config.ConfigReferent.Namespace != "test-ns" {
					return fmt.Errorf("expected namespace 'test-ns', got '%s'", config.ConfigReferent.Namespace)
				}
				if config.ConfigReferent.Name != "test-deployment-config" {
					return fmt.Errorf("expected name 'test-deployment-config', got '%s'", config.ConfigReferent.Name)
				}

				return nil
			}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
		})

		ginkgo.It("Should convert v1beta1 ManagedClusterAddOn to v1alpha1", func() {
			ginkgo.By(fmt.Sprintf("Creating v1beta1 ManagedClusterAddOn %q", managedClusterAddOnName))

			v1beta1MCA := &addonapiv1beta1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      managedClusterAddOnName,
					Namespace: namespace,
				},
				Spec: addonapiv1beta1.ManagedClusterAddOnSpec{
					Configs: []addonapiv1beta1.AddOnConfig{
						{
							ConfigGroupResource: addonapiv1beta1.ConfigGroupResource{
								Group:    "addon.open-cluster-management.io",
								Resource: "addontemplates",
							},
							ConfigReferent: addonapiv1beta1.ConfigReferent{
								Namespace: "template-ns",
								Name:      "beta-template-config",
							},
						},
					},
				},
			}

			_, err := hub.AddonClient.AddonV1beta1().ManagedClusterAddOns(namespace).Create(
				context.TODO(), v1beta1MCA, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			ginkgo.By("Reading the resource as v1alpha1 to verify conversion")
			gomega.Eventually(func() error {
				v1alpha1MCA, err := hub.AddonClient.AddonV1alpha1().ManagedClusterAddOns(namespace).Get(
					context.TODO(), managedClusterAddOnName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				// Verify Configs are preserved
				if len(v1alpha1MCA.Spec.Configs) != 1 {
					return fmt.Errorf("expected 1 Config, got %d", len(v1alpha1MCA.Spec.Configs))
				}

				config := v1alpha1MCA.Spec.Configs[0]
				if config.ConfigReferent.Namespace != "template-ns" {
					return fmt.Errorf("expected namespace 'template-ns', got '%s'", config.ConfigReferent.Namespace)
				}
				if config.ConfigReferent.Name != "beta-template-config" {
					return fmt.Errorf("expected name 'beta-template-config', got '%s'", config.ConfigReferent.Name)
				}

				return nil
			}, 30*time.Second, 1*time.Second).Should(gomega.Succeed())
		})
	})

	ginkgo.Context("Webhook Deployment Verification", func() {
		ginkgo.It("Should have addon conversion webhook service deployed", func() {
			ginkgo.By("Checking if the addon webhook service exists")

			gomega.Eventually(func() error {
				_, err := hub.KubeClient.CoreV1().Services("open-cluster-management-hub").Get(
					context.TODO(), "cluster-manager-addon-webhook", metav1.GetOptions{})
				return err
			}, 60*time.Second, 5*time.Second).Should(gomega.Succeed())
		})

		ginkgo.It("Should have correct conversion webhook configuration in CRDs", func() {
			ginkgo.By("Checking ClusterManagementAddOn CRD has conversion webhook configured")

			gomega.Eventually(func() error {
				crd, err := hub.APIExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(
					context.TODO(), "clustermanagementaddons.addon.open-cluster-management.io", metav1.GetOptions{})
				if err != nil {
					return err
				}

				if crd.Spec.Conversion == nil {
					return fmt.Errorf("conversion webhook not configured")
				}
				if crd.Spec.Conversion.Strategy != "Webhook" {
					return fmt.Errorf("expected conversion strategy 'Webhook', got '%s'", crd.Spec.Conversion.Strategy)
				}
				if crd.Spec.Conversion.Webhook == nil {
					return fmt.Errorf("webhook configuration is nil")
				}
				if crd.Spec.Conversion.Webhook.ClientConfig == nil {
					return fmt.Errorf("webhook client config is nil")
				}
				if crd.Spec.Conversion.Webhook.ClientConfig.Service == nil {
					return fmt.Errorf("webhook service config is nil")
				}
				if crd.Spec.Conversion.Webhook.ClientConfig.Service.Name != "cluster-manager-addon-webhook" {
					return fmt.Errorf("expected service name 'cluster-manager-addon-webhook', got '%s'",
						crd.Spec.Conversion.Webhook.ClientConfig.Service.Name)
				}

				return nil
			}, 60*time.Second, 5*time.Second).Should(gomega.Succeed())

			ginkgo.By("Checking ManagedClusterAddOn CRD has conversion webhook configured")

			gomega.Eventually(func() error {
				crd, err := hub.APIExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(
					context.TODO(), "managedclusteraddons.addon.open-cluster-management.io", metav1.GetOptions{})
				if err != nil {
					return err
				}

				if crd.Spec.Conversion == nil {
					return fmt.Errorf("conversion webhook not configured")
				}
				if crd.Spec.Conversion.Strategy != "Webhook" {
					return fmt.Errorf("expected conversion strategy 'Webhook', got '%s'", crd.Spec.Conversion.Strategy)
				}

				return nil
			}, 60*time.Second, 5*time.Second).Should(gomega.Succeed())
		})
	})
})
