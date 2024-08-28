package kueuesecretcopy

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"
	informerv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"

	"open-cluster-management.io/sdk-go/pkg/patcher"
	cpv1alpha1 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	cpclientset "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
	cpinformerv1alpha1 "sigs.k8s.io/cluster-inventory-api/client/informers/externalversions/apis/v1alpha1"
	cplisterv1alpha1 "sigs.k8s.io/cluster-inventory-api/client/listers/apis/v1alpha1"
)

// kueueSecretCopyController reconciles instances of secret on the hub.
type kueueSecretCopyController struct {
	kubeClient            kubernetes.Interface
	clusterProfileLister  cplisterv1alpha1.ClusterProfileLister
	clusterProfilePatcher patcher.Patcher[*cpv1alpha1.ClusterProfile, cpv1alpha1.ClusterProfileSpec, cpv1alpha1.ClusterProfileStatus]
	eventRecorder         events.Recorder
}

// NewKueueSecretCopyController copied secret from cluster namespace to kueue
func NewKueueSecretCopyController(
	kubeClient kubernetes.Interface,
	secretInformer informerv1.SecretInformer,
	clusterProfileClient cpclientset.Interface,
	clusterProfileInformer cpinformerv1alpha1.ClusterProfileInformer,
	recorder events.Recorder) factory.Controller {
	c := &kueueSecretCopyController{
		kubeClient:           kubeClient,
		clusterProfileLister: clusterProfileInformer.Lister(),
		clusterProfilePatcher: patcher.NewPatcher[
			*cpv1alpha1.ClusterProfile, cpv1alpha1.ClusterProfileSpec, cpv1alpha1.ClusterProfileStatus](
			clusterProfileClient.ApisV1alpha1().ClusterProfiles("open-cluster-management")),
		eventRecorder: recorder.WithComponentSuffix("cluster-profile-controller"),
	}

	return factory.New().
		WithFilteredEventsInformersQueueKeysFunc(
			func(obj runtime.Object) []string {
				accessor, _ := meta.Accessor(obj)
				return []string{fmt.Sprintf("%s/%s", accessor.GetNamespace(), accessor.GetName())}
			},
			func(obj interface{}) bool {
				accessor, _ := meta.Accessor(obj)
				return len(accessor.GetLabels()) > 0 && len(accessor.GetLabels()["authentication.open-cluster-management.io/is-managed-serviceaccount"]) > 0
			},
			secretInformer.Informer()).
		WithSync(c.sync).
		ToController("KueueSecretCopyController", recorder)
}

func (c *kueueSecretCopyController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	key := syncCtx.QueueKey()
	logger := klog.FromContext(ctx)
	logger.Info("Reconciling Secret", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	secret, err := c.kubeClient.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	//	get cluster url
	clusterProfileNamespace := "open-cluster-management"
	clusterProfileName := namespace
	targetNamespace := "kueue-system"
	clusterProfile, err := c.clusterProfileLister.ClusterProfiles(clusterProfileNamespace).Get(clusterProfileName)
	if err != nil {
		return err
	}

	// generate kubeconf secret
	kubeconfSecret, err := c.generateKueConfigSecret(ctx, secret, clusterProfile, targetNamespace)
	if err != nil {
		return err
	}
	if kubeconfSecret == nil {
		logger.V(4).Info("kubeconf secret is not ready")
		return nil
	}

	err = c.createOrUpdateSecret(ctx, kubeconfSecret)
	if err != nil {
		return err
	}

	// sync status to clusterprofile status
	newClusterProfile := clusterProfile.DeepCopy()
	newClusterProfile.Status.Credentials = []cpv1alpha1.Credential{
		{
			Consumer: "kueue-admin",
			AccessRef: cpv1alpha1.AccessRef{
				Kind:      "Secret",
				Name:      kubeconfSecret.Name,
				Namespace: kubeconfSecret.Namespace,
			},
		},
	}
	_, err = c.clusterProfilePatcher.PatchStatus(ctx, newClusterProfile, newClusterProfile.Status, clusterProfile.Status)
	if err != nil {
		return err
	}

	return err
}

func clusterProfileProperty(properties []cpv1alpha1.Property, name string) string {
	for _, p := range properties {
		if p.Name == name {
			return p.Value
		}
	}
	return ""
}

func (c *kueueSecretCopyController) createOrUpdateSecret(ctx context.Context, secret *v1.Secret) error {
	existSecret, err := c.kubeClient.CoreV1().Secrets(secret.Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err := c.kubeClient.CoreV1().Secrets(secret.Namespace).Create(context.Background(), secret, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	newKubeconfSecret := existSecret.DeepCopy()
	newKubeconfSecret.Data = secret.Data
	_, err = c.kubeClient.CoreV1().Secrets(secret.Namespace).Update(context.Background(), newKubeconfSecret, metav1.UpdateOptions{})
	return err
}

func (c *kueueSecretCopyController) generateKueConfigSecret(ctx context.Context, secret *v1.Secret, clusterProfile *cpv1alpha1.ClusterProfile, targetNamespace string) (*v1.Secret, error) {
	saToken := secret.Data["token"]
	caCert := base64.StdEncoding.EncodeToString(secret.Data["ca.crt"])
	clusterAddr := clusterProfileProperty(clusterProfile.Status.Properties, "url")

	kubeconfigStr := generateKueConfigStr(caCert, clusterAddr, clusterProfile.Name, secret.Name, saToken)

	// Create the Secret contains kubeconf
	kubeconfSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name + "-kubeconfig",
			Namespace: targetNamespace,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(kubeconfigStr),
		},
	}

	return kubeconfSecret, nil
}

func generateKueConfigStr(caCert, clusterAddr, clusterName, userName string, saToken []byte) string {
	kubeConfigString := fmt.Sprintf(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: %s
    server: %s
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
  name: %s
current-context: %s
kind: Config
preferences: {}
users:
- name: %s
  user:
    token: %s`,
		caCert, clusterAddr, clusterName, clusterName, userName, clusterName, clusterName, userName, saToken)

	return kubeConfigString
}
