package authtokenrequest

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/kubernetes"
	informerv1 "open-cluster-management.io/api/client/cluster/informers/externalversions/cluster/v1"
	listerv1 "open-cluster-management.io/api/client/cluster/listers/cluster/v1"
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

// authTokenRequestController reconciles instances of AuthTokenRequest on the hub.
type authTokenRequestController struct {
	kubeClient    kubernetes.Interface
	clusterLister listerv1.ManagedClusterLister
	//clusterProfileClient   cpclientset.Interface
	clusterProfileLister cplisterv1alpha1.ClusterProfileLister
	//authTokenRequestClient cpclientset.Interface
	authTokenRequestLister cplisterv1alpha1.AuthTokenRequestLister
	permissionClient       permissionclientset.Interface
	//permissionLister        permissionlisterv1alpha1.ClusterPermissionLister
	msaClient               msaclientset.Interface
	clusterProfilePatcher   patcher.Patcher[*cpv1alpha1.ClusterProfile, cpv1alpha1.ClusterProfileSpec, cpv1alpha1.ClusterProfileStatus]
	authTokenRequestPatcher patcher.Patcher[*cpv1alpha1.AuthTokenRequest, cpv1alpha1.AuthTokenRequestSpec, cpv1alpha1.AuthTokenRequestStatus]
	eventRecorder           events.Recorder
}

// NewAuthTokenRequestController creates a new managed cluster controller
func NewAuthTokenRequestController(
	kubeClient kubernetes.Interface,
	clusterInformer informerv1.ManagedClusterInformer,
	clusterProfileClient cpclientset.Interface,
	clusterProfileInformer cpinformerv1alpha1.ClusterProfileInformer,
	authTokenRequestInformer cpinformerv1alpha1.AuthTokenRequestInformer,
	permissionClient permissionclientset.Interface,
	permissionInformer permissioninformerv1alpha1.ClusterPermissionInformer,
	msaClient msaclientset.Interface,
	recorder events.Recorder) factory.Controller {
	c := &authTokenRequestController{
		kubeClient:    kubeClient,
		clusterLister: clusterInformer.Lister(),
		//clusterProfileClient:   clusterProfileClient,
		clusterProfileLister: clusterProfileInformer.Lister(),
		//authTokenRequestClient: authTokenRequestClient,
		authTokenRequestLister: authTokenRequestInformer.Lister(),
		permissionClient:       permissionClient,
		//permissionLister:       permissionInformer.Lister(),
		msaClient: msaClient,
		clusterProfilePatcher: patcher.NewPatcher[
			*cpv1alpha1.ClusterProfile, cpv1alpha1.ClusterProfileSpec, cpv1alpha1.ClusterProfileStatus](
			clusterProfileClient.ApisV1alpha1().ClusterProfiles("open-cluster-management")),
		authTokenRequestPatcher: patcher.NewPatcher[
			*cpv1alpha1.AuthTokenRequest, cpv1alpha1.AuthTokenRequestSpec, cpv1alpha1.AuthTokenRequestStatus](
			clusterProfileClient.ApisV1alpha1().AuthTokenRequests("open-cluster-management")),
		eventRecorder: recorder.WithComponentSuffix("cluster-profile-controller"),
	}

	return factory.New().
		WithInformersQueueKeysFunc(queue.QueueKeyByMetaNamespaceName, authTokenRequestInformer.Informer()).
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

	if !authTokenRequest.DeletionTimestamp.IsZero() {
		// the cleanup job is moved to gc controller
		return nil
	}

	targetCluserName := authTokenRequest.Spec.TargetClusterProfile.Name
	targetNamespace := targetCluserName
	targetServiceAccountName := authTokenRequest.Spec.ServiceAccountName
	targetClusterRoles := authTokenRequest.Spec.ClusterRoles
	//targetRoles := authTokenRequest.Spec.Roles
	targetCluserNamespace := authTokenRequest.Spec.TargetClusterProfile.Namespace

	// create ClusterPermission if not found
	_, err = c.permissionClient.ApisV1alpha1().ClusterPermissions(targetNamespace).Get(ctx, targetServiceAccountName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		cluserPermission := &permissionv1alpha1.ClusterPermission{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetServiceAccountName,
				Namespace: targetNamespace,
			},
			Spec: permissionv1alpha1.ClusterPermissionSpec{
				ClusterRole: &permissionv1alpha1.ClusterRole{
					Rules: targetClusterRoles[0].Rules,
				},
				ClusterRoleBinding: &permissionv1alpha1.ClusterRoleBinding{
					rbacv1.Subject{
						Kind:      "ServiceAccount",
						Name:      targetServiceAccountName,
						Namespace: targetNamespace,
					},
				},
				//TODO: roles
			},
		}
		_, err = c.permissionClient.ApisV1alpha1().ClusterPermissions(targetNamespace).Create(ctx, cluserPermission, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	// create ManagedServiceAccount if not found
	managedServiceAccount, err := c.msaClient.AuthenticationV1beta1().ManagedServiceAccounts(targetNamespace).Get(ctx, targetServiceAccountName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		managedServiceAccount := &msav1beta1.ManagedServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetServiceAccountName,
				Namespace: targetNamespace,
			},
			Spec: msav1beta1.ManagedServiceAccountSpec{},
		}
		_, err := c.msaClient.AuthenticationV1beta1().ManagedServiceAccounts(targetNamespace).Create(ctx, managedServiceAccount, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	// TODO: check condition
	msatokenSecretName := managedServiceAccount.Status.TokenSecretRef.Name
	// get mas generated secret
	msatokenSecret, err := c.kubeClient.CoreV1().Secrets(targetNamespace).Get(context.TODO(), msatokenSecretName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// get mcl
	managedCluster, err := c.clusterLister.Get(targetNamespace)
	if errors.IsNotFound(err) {
		// Spoke cluster not found, could have been deleted, do nothing.
		return nil
	}
	if err != nil {
		return err
	}

	// Generate the kubeconfig content
	saToken := msatokenSecret.Data["token"]
	caCert := msatokenSecret.Data["ca.crt"]
	targetClusterAddr := managedCluster.Spec.ManagedClusterClientConfigs[0].URL

	kubeconfigContent := fmt.Sprintf(`apiVersion: v1
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
		caCert, targetClusterAddr, targetCluserName, targetCluserName, requestName, targetCluserName, targetCluserName, requestName, saToken)

	// Encode the kubeconfig content to Base64
	encodedKubeconfig := base64.StdEncoding.EncodeToString([]byte(kubeconfigContent))

	// Create the Secret contains kubeconf
	kubeconfSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetServiceAccountName + "-kubeconfig",
			Namespace: requestNamespace, // Change if needed
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(encodedKubeconfig),
		},
	}

	// TODO: replace with patcher
	_, err = c.kubeClient.CoreV1().Secrets(requestNamespace).Update(context.Background(), kubeconfSecret, metav1.UpdateOptions{})
	if err != nil {
		log.Fatalf("Error creating Secret: %s", err.Error())
	}

	// sync status to authtokenrequest and clusterprofile status
	newAuthTokenRequest := authTokenRequest.DeepCopy()
	newAuthTokenRequest.Status.TokenResponse.Name = kubeconfSecret.Name
	c.authTokenRequestPatcher.PatchStatus(ctx, newAuthTokenRequest, newAuthTokenRequest.Status, authTokenRequest.Status)
	//TODO: set conditions

	clusterProfile, err := c.clusterProfileLister.ClusterProfiles(targetCluserNamespace).Get(targetCluserName)
	newClusterProfile := clusterProfile.DeepCopy()
	newClusterProfile.Status.TokenRequests = append(newClusterProfile.Status.TokenRequests, cpv1alpha1.TokenRequest{
		RequestRef: cpv1alpha1.TokenRequestRef{
			Name:      kubeconfSecret.Name,
			Namespace: kubeconfSecret.Namespace,
		},
		//TODO: sync conditions
	})
	c.clusterProfilePatcher.PatchStatus(ctx, newClusterProfile, newClusterProfile.Status, clusterProfile.Status)

	return err
}
