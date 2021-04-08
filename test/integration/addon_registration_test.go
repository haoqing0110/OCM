package integration_test

import (
	"context"
	"fmt"
	"path"
	"reflect"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/rand"

	addonv1alpha1 "github.com/open-cluster-management/api/addon/v1alpha1"
	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	"github.com/open-cluster-management/registration/pkg/clientcert"
	"github.com/open-cluster-management/registration/pkg/features"
	"github.com/open-cluster-management/registration/pkg/spoke"
	"github.com/open-cluster-management/registration/test/integration/util"
	certificates "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
)

var _ = ginkgo.Describe("Addon Registration", func() {
	var managedClusterName, hubKubeconfigSecret, hubKubeconfigDir, addOnName string
	var err error

	ginkgo.BeforeEach(func() {
		suffix := rand.String(5)
		managedClusterName = fmt.Sprintf("managedcluster-%s", suffix)
		hubKubeconfigSecret = fmt.Sprintf("hub-kubeconfig-secret-%s", suffix)
		hubKubeconfigDir = path.Join(util.TestDir, fmt.Sprintf("addontest-%s", suffix), "hub-kubeconfig")
		addOnName = fmt.Sprintf("addon-%s", suffix)

		// run registration agent
		go func() {
			features.DefaultMutableFeatureGate.Set("AddonManagement=true")
			agentOptions := spoke.SpokeAgentOptions{
				ClusterName:              managedClusterName,
				BootstrapKubeconfig:      bootstrapKubeConfigFile,
				HubKubeconfigSecret:      hubKubeconfigSecret,
				HubKubeconfigDir:         hubKubeconfigDir,
				ClusterHealthCheckPeriod: 1 * time.Minute,
			}
			err := agentOptions.RunSpokeAgent(context.Background(), &controllercmd.ControllerContext{
				KubeConfig:    spokeCfg,
				EventRecorder: util.NewIntegrationTestEventRecorder("addontest"),
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}()
	})

	assertSuccessClusterBootstrap := func() {
		// the spoke cluster and csr should be created after bootstrap
		ginkgo.By("Check existence of ManagedCluster & CSR")
		gomega.Eventually(func() bool {
			if _, err := util.GetManagedCluster(clusterClient, managedClusterName); err != nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

		gomega.Eventually(func() bool {
			if _, err := util.FindUnapprovedSpokeCSR(kubeClient, managedClusterName); err != nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

		// the spoke cluster should has finalizer that is added by hub controller
		gomega.Eventually(func() bool {
			spokeCluster, err := util.GetManagedCluster(clusterClient, managedClusterName)
			if err != nil {
				return false
			}
			if len(spokeCluster.Finalizers) != 1 {
				return false
			}

			if spokeCluster.Finalizers[0] != "cluster.open-cluster-management.io/api-resource-cleanup" {
				return false
			}

			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

		ginkgo.By("Accept and approve the ManagedCluster")
		// simulate hub cluster admin to accept the managedcluster and approve the csr
		err = util.AcceptManagedCluster(clusterClient, managedClusterName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		err = util.ApproveSpokeClusterCSR(kubeClient, managedClusterName, time.Hour*24)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// the managed cluster should have accepted condition after it is accepted
		gomega.Eventually(func() bool {
			spokeCluster, err := util.GetManagedCluster(clusterClient, managedClusterName)
			if err != nil {
				return false
			}
			accpeted := meta.FindStatusCondition(spokeCluster.Status.Conditions, clusterv1.ManagedClusterConditionHubAccepted)
			if accpeted == nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

		// the hub kubeconfig secret should be filled after the csr is approved
		gomega.Eventually(func() bool {
			if _, err := util.GetFilledHubKubeConfigSecret(kubeClient, testNamespace, hubKubeconfigSecret); err != nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

		ginkgo.By("ManagedCluster joins the hub")
		// the spoke cluster should have joined condition finally
		gomega.Eventually(func() bool {
			spokeCluster, err := util.GetManagedCluster(clusterClient, managedClusterName)
			if err != nil {
				return false
			}
			joined := meta.FindStatusCondition(spokeCluster.Status.Conditions, clusterv1.ManagedClusterConditionJoined)
			if joined == nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

		// ensure cluster namespace is in place
		gomega.Eventually(func() bool {
			_, err := kubeClient.CoreV1().Namespaces().Get(context.TODO(), managedClusterName, metav1.GetOptions{})
			if err != nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
	}

	assertSuccessCSRApproval := func() {
		ginkgo.By("Approve bootstrap csr")
		var csr *certificates.CertificateSigningRequest
		gomega.Eventually(func() bool {
			csr, err = util.FindUnapprovedAddOnCSR(kubeClient, managedClusterName, addOnName)
			if err != nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

		now := time.Now()
		err = util.ApproveCSR(kubeClient, csr, now.UTC(), now.Add(30*time.Second).UTC())
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}

	assertValidClientCertificate := func(secretNamespace, secretName, signerName string) {
		ginkgo.By("Check client certificate in secret")
		gomega.Eventually(func() bool {
			secret, err := kubeClient.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
			if err != nil {
				return false
			}
			if _, ok := secret.Data[clientcert.TLSKeyFile]; !ok {
				return false
			}
			if _, ok := secret.Data[clientcert.TLSCertFile]; !ok {
				return false
			}
			_, ok := secret.Data[clientcert.KubeconfigFile]
			if !ok && signerName == "kubernetes.io/kube-apiserver-client" {
				return false
			}
			if ok && signerName != "kubernetes.io/kube-apiserver-client" {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
	}

	assertSuccessAddOnBootstrap := func(signerName string) {
		ginkgo.By("Create ManagedClusterAddOn cr with required annotations")
		// create addon namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: addOnName,
			},
		}
		_, err = kubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// create addon
		registrations := fmt.Sprintf(`[{"signerName":"%s"}]`, signerName)
		addOn := &addonv1alpha1.ManagedClusterAddOn{
			ObjectMeta: metav1.ObjectMeta{
				Name:      addOnName,
				Namespace: managedClusterName,
				Annotations: map[string]string{
					"addon.open-cluster-management.io/installNamespace": addOnName,
					"addon.open-cluster-management.io/registrations":    registrations,
				},
			},
		}
		_, err = addOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Create(context.TODO(), addOn, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		assertSuccessCSRApproval()
		assertValidClientCertificate(addOnName, getSecretName(addOnName, signerName), signerName)
	}

	assertSecretGone := func(secretNamespace, secretName string) {
		gomega.Eventually(func() bool {
			_, err = kubeClient.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return true
			}

			return false
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
	}

	ginkgo.It("should register addon successfully", func() {
		assertSuccessClusterBootstrap()
		signerName := "kubernetes.io/kube-apiserver-client"
		assertSuccessAddOnBootstrap(signerName)

		ginkgo.By("Delete the addon and check if secret is gone")
		err = addOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Delete(context.TODO(), addOnName, metav1.DeleteOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		assertSecretGone(addOnName, getSecretName(addOnName, signerName))
	})

	ginkgo.It("should register addon with custom signer successfully", func() {
		assertSuccessClusterBootstrap()
		signerName := "example.com/signer1"
		assertSuccessAddOnBootstrap(signerName)
	})

	ginkgo.It("should addon registraton config updated successfully", func() {
		assertSuccessClusterBootstrap()
		signerName := "kubernetes.io/kube-apiserver-client"
		assertSuccessAddOnBootstrap(signerName)

		// update registration config and change the signer
		addOn, err := addOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Get(context.TODO(), addOnName, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		newSignerName := "example.com/signer1"
		registrations := fmt.Sprintf(`[{"signerName":"%s"}]`, newSignerName)
		addOn.Annotations["addon.open-cluster-management.io/registrations"] = registrations
		_, err = addOnClient.AddonV1alpha1().ManagedClusterAddOns(managedClusterName).Update(context.TODO(), addOn, metav1.UpdateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		assertSecretGone(addOnName, getSecretName(addOnName, signerName))

		assertSuccessCSRApproval()
		assertValidClientCertificate(addOnName, getSecretName(addOnName, newSignerName), newSignerName)
	})

	ginkgo.It("should rotate addon client cert successfully", func() {
		assertSuccessClusterBootstrap()
		signerName := "kubernetes.io/kube-apiserver-client"
		assertSuccessAddOnBootstrap(signerName)

		secretName := getSecretName(addOnName, signerName)
		secret, err := kubeClient.CoreV1().Secrets(addOnName).Get(context.TODO(), secretName, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		ginkgo.By("Wait for cert rotation")
		assertSuccessCSRApproval()
		gomega.Eventually(func() bool {
			newSecret, err := kubeClient.CoreV1().Secrets(addOnName).Get(context.TODO(), secretName, metav1.GetOptions{})
			if err != nil {
				return false
			}

			return !reflect.DeepEqual(secret.Data, newSecret.Data)
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
	})
})

func getSecretName(addOnName, signerName string) string {
	if signerName == "kubernetes.io/kube-apiserver-client" {
		return fmt.Sprintf("%s-hub-kubeconfig", addOnName)
	}
	return fmt.Sprintf("%s-%s-client-cert", addOnName, strings.ReplaceAll(signerName, "/", "-"))
}
