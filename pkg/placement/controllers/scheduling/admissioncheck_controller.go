package scheduling

import (
	"context"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kevents "k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	sdkv1beta1 "open-cluster-management.io/sdk-go/pkg/apis/cluster/v1beta1"
	"open-cluster-management.io/sdk-go/pkg/patcher"

	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterinformerv1beta1 "open-cluster-management.io/api/client/cluster/informers/externalversions/cluster/v1beta1"
	clusterlisterv1beta1 "open-cluster-management.io/api/client/cluster/listers/cluster/v1beta1"
	clusterapiv1beta1 "open-cluster-management.io/api/cluster/v1beta1"

	"open-cluster-management.io/ocm/pkg/common/helpers"
	"open-cluster-management.io/ocm/pkg/common/queue"
	cpinformerv1alpha1 "sigs.k8s.io/cluster-inventory-api/client/informers/externalversions/apis/v1alpha1"
	cplisterv1alpha1 "sigs.k8s.io/cluster-inventory-api/client/listers/apis/v1alpha1"
	kueuev1alpha1 "sigs.k8s.io/kueue/apis/kueue/v1alpha1"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	kueueclient "sigs.k8s.io/kueue/client-go/clientset/versioned"
	kueueinformerv1beta1 "sigs.k8s.io/kueue/client-go/informers/externalversions/kueue/v1beta1"
	kueuelisterv1beta1 "sigs.k8s.io/kueue/client-go/listers/kueue/v1beta1"
)

const (
	admissioncheckControllerName = "AdmissionCheckController"
	clusterProfileNamespace      = "open-cluster-management"
)

// admissioncheckController schedules cluster decisions for Placements
type admissioncheckController struct {
	clusterClient           clusterclient.Interface
	kueueClient             *kueueclient.Clientset
	clusterProfileLister    cplisterv1alpha1.ClusterProfileLister
	placementLister         clusterlisterv1beta1.PlacementLister
	placementDecisionGetter helpers.PlacementDecisionGetter
	admissioncheckLister    kueuelisterv1beta1.AdmissionCheckLister
	eventsRecorder          kevents.EventRecorder
}

// NewSchedulingController return an instance of schedulingController
func NewAdmissionCheckController(
	ctx context.Context,
	clusterClient clusterclient.Interface,
	kueueClient *kueueclient.Clientset,
	clusterProfileInformer cpinformerv1alpha1.ClusterProfileInformer,
	placementInformer clusterinformerv1beta1.PlacementInformer,
	placementDecisionInformer clusterinformerv1beta1.PlacementDecisionInformer,
	admissionCheckInformer kueueinformerv1beta1.AdmissionCheckInformer,
	recorder events.Recorder, krecorder kevents.EventRecorder,
) factory.Controller {
	syncCtx := factory.NewSyncContext(admissioncheckControllerName, recorder)

	// build controller
	c := &admissioncheckController{
		clusterClient:           clusterClient,
		kueueClient:             kueueClient,
		clusterProfileLister:    clusterProfileInformer.Lister(),
		placementLister:         placementInformer.Lister(),
		placementDecisionGetter: helpers.PlacementDecisionGetter{Client: placementDecisionInformer.Lister()},
		admissioncheckLister:    admissionCheckInformer.Lister(),
		eventsRecorder:          krecorder,
	}

	return factory.New().
		WithSyncContext(syncCtx).
		WithFilteredEventsInformersQueueKeysFunc(
			queue.QueueKeyByMetaName,
			func(obj interface{}) bool {
				accessor, _ := meta.Accessor(obj)
				admissionCheck, _ := accessor.(*kueuev1beta1.AdmissionCheck)
				return admissionCheck.Spec.ControllerName == "open-cluster-management.io/placement"
			},
			admissionCheckInformer.Informer()).
		WithSync(c.sync).
		ToController(admissioncheckControllerName, recorder)
}

func (c *admissioncheckController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	key := syncCtx.QueueKey()
	logger := klog.FromContext(ctx)
	logger.V(4).Info("Reconciling AdmissionCheck", key)

	admissionCheck, err := c.admissioncheckLister.Get(key)
	if errors.IsNotFound(err) {
		// Spoke cluster not found, could have been deleted, do nothing.
		return nil
	}
	if err != nil {
		return err
	}

	placementName := admissionCheck.Spec.Parameters.Name

	// init placement tracker
	placement := &clusterapiv1beta1.Placement{
		ObjectMeta: metav1.ObjectMeta{Name: placementName, Namespace: "kueue-system"},
		Spec:       clusterapiv1beta1.PlacementSpec{},
	}
	// new decision tracker
	pdTracker := sdkv1beta1.NewPlacementDecisionClustersTracker(placement, c.placementDecisionGetter, nil)

	// refresh and get existing decision clusters
	err = pdTracker.Refresh()
	if err != nil {
		return err
	}
	clusters := pdTracker.ExistingClusterGroupsBesides().GetClusters()

	mkconfig := &kueuev1alpha1.MultiKueueConfig{
		ObjectMeta: metav1.ObjectMeta{Name: admissionCheck.Name},
		Spec: kueuev1alpha1.MultiKueueConfigSpec{
			Clusters: []string{},
		},
	}

	for cn := range clusters {
		klog.Warningf("cluster %s", c)
		cp, err := c.clusterProfileLister.ClusterProfiles(clusterProfileNamespace).Get(cn)
		if err != nil {
			return err
		}
		mkc := &kueuev1alpha1.MultiKueueCluster{
			ObjectMeta: metav1.ObjectMeta{Name: placementName + "-" + cn},
			Spec: kueuev1alpha1.MultiKueueClusterSpec{
				kueuev1alpha1.KubeConfig{
					LocationType: kueuev1alpha1.SecretLocationType,
					Location:     cp.Status.Credentials[0].AccessRef.Name,
				},
			},
		}
		err = c.createOrUpdateMultiKueueCluster(ctx, mkc)
		if err != nil {
			return err
		}
		mkconfig.Spec.Clusters = append(mkconfig.Spec.Clusters, mkc.Name)
	}

	err = c.createOrUpdateMultiKueueConfig(ctx, mkconfig)
	if err != nil {
		return err
	}

	newadmissioncheck := admissionCheck.DeepCopy()
	meta.SetStatusCondition(&newadmissioncheck.Status.Conditions, metav1.Condition{
		Type:    kueuev1alpha1.MultiKueueClusterActive,
		Status:  metav1.ConditionTrue,
		Reason:  "Active",
		Message: "MultiKueueConfig and MultiKueueCluster generated",
	})

	admissioncheckPatcher := patcher.NewPatcher[
		*kueuev1beta1.AdmissionCheck, kueuev1beta1.AdmissionCheckSpec, kueuev1beta1.AdmissionCheckStatus](
		c.kueueClient.KueueV1beta1().AdmissionChecks())

	_, err = admissioncheckPatcher.PatchStatus(ctx, newadmissioncheck, newadmissioncheck.Status, admissionCheck.Status)

	return err
}

func (c *admissioncheckController) createOrUpdateMultiKueueConfig(ctx context.Context, mkconfig *kueuev1alpha1.MultiKueueConfig) error {
	oldmkconfig, err := c.kueueClient.KueueV1alpha1().MultiKueueConfigs().Get(ctx, mkconfig.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = c.kueueClient.KueueV1alpha1().MultiKueueConfigs().Create(ctx, mkconfig, metav1.CreateOptions{})
		return err
	}
	if err == nil {
		newmkconfig := oldmkconfig.DeepCopy()
		newmkconfig.Spec.Clusters = mkconfig.Spec.Clusters
		_, err = c.kueueClient.KueueV1alpha1().MultiKueueConfigs().Update(ctx, newmkconfig, metav1.UpdateOptions{})
		return err
	}
	return err
}

func (c *admissioncheckController) createOrUpdateMultiKueueCluster(ctx context.Context, mkc *kueuev1alpha1.MultiKueueCluster) error {
	oldmkc, err := c.kueueClient.KueueV1alpha1().MultiKueueClusters().Get(ctx, mkc.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = c.kueueClient.KueueV1alpha1().MultiKueueClusters().Create(ctx, mkc, metav1.CreateOptions{})
		return err
	}
	if err == nil {
		newmkc := oldmkc.DeepCopy()
		newmkc.Spec.KubeConfig = *mkc.Spec.KubeConfig.DeepCopy()
		_, err = c.kueueClient.KueueV1alpha1().MultiKueueClusters().Update(ctx, newmkc, metav1.UpdateOptions{})
		return err
	}
	return err
}
