package authtokenrequestbk

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	informerv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	permissionv1alpha1 "open-cluster-management.io/cluster-permission/apis/v1alpha1"
	permissionclientset "open-cluster-management.io/cluster-permission/client/clientset/versioned"
	permissioninformerv1alpha1 "open-cluster-management.io/cluster-permission/client/informers/externalversions/apis/v1alpha1"
	msav1beta1 "open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	msaclientset "open-cluster-management.io/managed-serviceaccount/pkg/generated/clientset/versioned"
	"open-cluster-management.io/ocm/pkg/common/queue"
	"open-cluster-management.io/sdk-go/pkg/patcher"
	cpv1alpha1 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	cpclientset "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
	cpinformerv1alpha1 "sigs.k8s.io/cluster-inventory-api/client/informers/externalversions/apis/v1alpha1"
	cplisterv1alpha1 "sigs.k8s.io/cluster-inventory-api/client/listers/apis/v1alpha1"
)

const (
	authTokenRequestFinalizer      = "cluster.open-cluster-management.io/auth-token-request-cleanup"
	labelAuthTokenRequestNamespace = "multicluster.x-k8s.io/auth-token-request-namespace"
	labelAuthTokenRequestName      = "multicluster.x-k8s.io/auth-token-request-name"
)

// authTokenRequestController reconciles instances of AuthTokenRequest on the hub.
type authTokenRequestController struct {
	kubeClient              kubernetes.Interface
	msaClient               msaclientset.Interface
	clusterProfileLister    cplisterv1alpha1.ClusterProfileLister
	authTokenRequestLister  cplisterv1alpha1.AuthTokenRequestLister
	clusterpermissionClient permissionclientset.Interface
	//permissionLister        permissionlisterv1alpha1.ClusterPermissionLister
	clusterProfilePatcher   patcher.Patcher[*cpv1alpha1.ClusterProfile, cpv1alpha1.ClusterProfileSpec, cpv1alpha1.ClusterProfileStatus]
	authTokenRequestPatcher patcher.Patcher[*cpv1alpha1.AuthTokenRequest, cpv1alpha1.AuthTokenRequestSpec, cpv1alpha1.AuthTokenRequestStatus]
	eventRecorder           events.Recorder
}

// NewAuthTokenRequestController creates a new managed cluster controller
func NewAuthTokenRequestControllerbk(
	kubeClient kubernetes.Interface,
	secretInformer informerv1.SecretInformer,
	clusterProfileClient cpclientset.Interface,
	clusterProfileInformer cpinformerv1alpha1.ClusterProfileInformer,
	authTokenRequestInformer cpinformerv1alpha1.AuthTokenRequestInformer,
	clusterpermissionClient permissionclientset.Interface,
	clusterpermissionInformer permissioninformerv1alpha1.ClusterPermissionInformer,
	msaClient msaclientset.Interface,
	recorder events.Recorder) factory.Controller {
	c := &authTokenRequestController{
		kubeClient:              kubeClient,
		msaClient:               msaClient,
		clusterProfileLister:    clusterProfileInformer.Lister(),
		authTokenRequestLister:  authTokenRequestInformer.Lister(),
		clusterpermissionClient: clusterpermissionClient,
		//permissionLister:       permissionInformer.Lister(),
		clusterProfilePatcher: patcher.NewPatcher[
			*cpv1alpha1.ClusterProfile, cpv1alpha1.ClusterProfileSpec, cpv1alpha1.ClusterProfileStatus](
			clusterProfileClient.ApisV1alpha1().ClusterProfiles("open-cluster-management")),
		authTokenRequestPatcher: patcher.NewPatcher[
			*cpv1alpha1.AuthTokenRequest, cpv1alpha1.AuthTokenRequestSpec, cpv1alpha1.AuthTokenRequestStatus](
			clusterProfileClient.ApisV1alpha1().AuthTokenRequests("kueue-system")),
		eventRecorder: recorder.WithComponentSuffix("cluster-profile-controller"),
	}

	return factory.New().
		WithInformersQueueKeysFunc(queue.QueueKeyByMetaNamespaceName, authTokenRequestInformer.Informer()).
		WithInformersQueueKeysFunc(func(obj runtime.Object) []string {
			accessor, _ := meta.Accessor(obj)
			msa, _ := c.msaClient.AuthenticationV1alpha1().ManagedServiceAccounts(accessor.GetNamespace()).Get(context.Background(), accessor.GetName(), metav1.GetOptions{})
			namespace := msa.GetLabels()[labelAuthTokenRequestNamespace]
			name := msa.GetLabels()[labelAuthTokenRequestName]
			return []string{fmt.Sprintf("%s/%s", namespace, name)}
		}, secretInformer.Informer()).
		WithSync(c.sync).
		ToController("AuthTokenRequestController", recorder)
}

func (c *authTokenRequestController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	key := syncCtx.QueueKey()
	logger := klog.FromContext(ctx)
	logger.V(4).Info("Reconciling AuthTokenRequester", key)

	requestNamespace, requestName, err := cache.SplitMetaNamespaceKey(key)
	authTokenRequest, err := c.authTokenRequestLister.AuthTokenRequests(requestNamespace).Get(requestName)
	if errors.IsNotFound(err) {
		// Spoke cluster not found, could have been deleted, do nothing.
		return nil
	}
	if err != nil {
		return err
	}

	clusterProfileName := authTokenRequest.Spec.TargetClusterProfile.Name
	clusterProfileNamespace := authTokenRequest.Spec.TargetClusterProfile.Namespace
	generateNamespace := clusterProfileName
	generateSAName := authTokenRequest.Spec.ServiceAccountName

	// if clusterProfileNamespace != "open-cluster-management"

	if !authTokenRequest.DeletionTimestamp.IsZero() {
		// delete clusterpermission and msa
		for _, generateClusterRole := range authTokenRequest.Spec.ClusterRoles {
			err = c.clusterpermissionClient.ApisV1alpha1().ClusterPermissions(generateNamespace).Delete(ctx, generateClusterRole.Name, metav1.DeleteOptions{})
			if err != nil {
				return err
			}
		}
		// delete msa
		err = c.msaClient.AuthenticationV1beta1().ManagedServiceAccounts(generateNamespace).Delete(ctx, generateSAName, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
		// delete kubeconf secret
		err := c.kubeClient.CoreV1().Secrets(requestNamespace).Delete(context.Background(), authTokenRequest.Status.TokenResponse.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
		// remove finalizer
		err = c.authTokenRequestPatcher.RemoveFinalizer(ctx, authTokenRequest, authTokenRequestFinalizer)
		return err
	}

	_, err = c.authTokenRequestPatcher.AddFinalizer(ctx, authTokenRequest, authTokenRequestFinalizer)
	if err != nil {
		return err
	}

	// create or update ClusterPermission
	for _, generateClusterRole := range authTokenRequest.Spec.ClusterRoles {
		cluserPermission := &permissionv1alpha1.ClusterPermission{
			ObjectMeta: metav1.ObjectMeta{
				Name:      generateClusterRole.Name,
				Namespace: generateNamespace,
				Labels: map[string]string{
					labelAuthTokenRequestNamespace: requestNamespace,
					labelAuthTokenRequestName:      requestName,
				},
			},
			Spec: permissionv1alpha1.ClusterPermissionSpec{
				ClusterRole: &permissionv1alpha1.ClusterRole{
					Rules: generateClusterRole.Rules,
				},
				ClusterRoleBinding: &permissionv1alpha1.ClusterRoleBinding{
					Subject: rbacv1.Subject{
						Kind:      "ServiceAccount",
						Name:      generateSAName,
						Namespace: "open-cluster-management-agent-addon",
					},
				},
				//TODO: roles
			},
		}
		err = c.createOrUpdateClusterPermission(ctx, cluserPermission)
		if err != nil {
			return err
		}
	}
	// TODO: authTokenRequest.Spec.Roles

	// create and get ManagedServiceAccount
	managedServiceAccount := &msav1beta1.ManagedServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateSAName,
			Namespace: generateNamespace,
			Labels: map[string]string{
				labelAuthTokenRequestNamespace: requestNamespace,
				labelAuthTokenRequestName:      requestName,
			},
		},
		Spec: msav1beta1.ManagedServiceAccountSpec{
			Rotation: msav1beta1.ManagedServiceAccountRotation{
				Enabled:  true,
				Validity: metav1.Duration{8640 * time.Hour},
			},
		},
	}
	managedServiceAccount, err = c.createManagedServiceAccount(ctx, managedServiceAccount)
	if err != nil {
		return err
	}

	//	get cluster url
	clusterProfile, err := c.clusterProfileLister.ClusterProfiles(clusterProfileNamespace).Get(clusterProfileName)
	if err != nil {
		return err
	}

	// generate kubeconf secret
	kubeconfSecret, err := c.generateKueConfigSecret(ctx, managedServiceAccount, clusterProfile, requestNamespace)
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

	// sync status to authtokenrequest
	newAuthTokenRequest := authTokenRequest.DeepCopy()
	newAuthTokenRequest.Status.TokenResponse = cpv1alpha1.ConfigMapRef{
		APIGroup: "core",
		Kind:     "Secret",
		Name:     kubeconfSecret.Name,
	}
	//TODO: set conditions
	_, err = c.authTokenRequestPatcher.PatchStatus(ctx, newAuthTokenRequest, newAuthTokenRequest.Status, authTokenRequest.Status)
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

func (c *authTokenRequestController) createOrUpdateClusterPermission(ctx context.Context, cp *permissionv1alpha1.ClusterPermission) error {
	existcp, err := c.clusterpermissionClient.ApisV1alpha1().ClusterPermissions(cp.Namespace).Get(ctx, cp.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = c.clusterpermissionClient.ApisV1alpha1().ClusterPermissions(cp.Namespace).Create(ctx, cp, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	clusterPermissionPatcher := patcher.NewPatcher[
		*permissionv1alpha1.ClusterPermission, permissionv1alpha1.ClusterPermissionSpec, permissionv1alpha1.ClusterPermissionStatus](
		c.clusterpermissionClient.ApisV1alpha1().ClusterPermissions(cp.Namespace))
	_, err = clusterPermissionPatcher.PatchSpec(ctx, cp, cp.Spec, existcp.Spec)
	return err
}

func (c *authTokenRequestController) createManagedServiceAccount(ctx context.Context, msa *msav1beta1.ManagedServiceAccount) (*msav1beta1.ManagedServiceAccount, error) {
	existmsa, err := c.msaClient.AuthenticationV1beta1().ManagedServiceAccounts(msa.Namespace).Get(ctx, msa.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		existmsa, err := c.msaClient.AuthenticationV1beta1().ManagedServiceAccounts(msa.Namespace).Create(ctx, msa, metav1.CreateOptions{})
		return existmsa, err
	}
	return existmsa, err
}

func (c *authTokenRequestController) createOrUpdateSecret(ctx context.Context, secret *v1.Secret) error {
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

func (c *authTokenRequestController) generateKueConfigSecret(ctx context.Context, managedServiceAccount *msav1beta1.ManagedServiceAccount, clusterProfile *cpv1alpha1.ClusterProfile, namespace string) (*v1.Secret, error) {
	// check msa
	if !meta.IsStatusConditionTrue(managedServiceAccount.Status.Conditions, msav1beta1.ConditionTypeSecretCreated) ||
		!meta.IsStatusConditionTrue(managedServiceAccount.Status.Conditions, msav1beta1.ConditionTypeTokenReported) ||
		managedServiceAccount.Status.TokenSecretRef == nil ||
		(managedServiceAccount.Status.ExpirationTimestamp != nil && time.Now().After(managedServiceAccount.Status.ExpirationTimestamp.Time)) {
		return nil, nil
	}

	// get mas generated secret
	msatokenSecret, err := c.kubeClient.CoreV1().Secrets(managedServiceAccount.Namespace).Get(context.TODO(), managedServiceAccount.Status.TokenSecretRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	saToken := msatokenSecret.Data["token"]
	caCert := base64.StdEncoding.EncodeToString(msatokenSecret.Data["ca.crt"])
	clusterAddr := clusterProfileProperty(clusterProfile.Status.Properties, "url")

	kubeconfigStr := generateKueConfigStr(caCert, clusterAddr, clusterProfile.Name, managedServiceAccount.Name, saToken)

	// Create the Secret contains kubeconf
	kubeconfSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedServiceAccount.Name + "-kubeconfig",
			Namespace: namespace,
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
