package index

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
	kueueinformerv1beta1 "sigs.k8s.io/kueue/client-go/informers/externalversions/kueue/v1beta1"

	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

const (
	AdmissionCheckByPlacement = "admissionCheckByPlacement"
)

func IndexAdmissionCheckByPlacement(obj interface{}) ([]string, error) {
	ac, ok := obj.(*kueuev1beta1.AdmissionCheck)

	if !ok {
		return []string{}, fmt.Errorf("obj %T is not a valid ocm admission check", obj)
	}

	if ac.Spec.ControllerName != "open-cluster-management.io/placement" {
		return []string{}, nil
	}

	var keys []string
	placementName := ac.Spec.Parameters.Name
	key := fmt.Sprintf("%s/%s", "kueue-system", placementName)
	keys = append(keys, key)

	return keys, nil
}

func AdmissionCheckByPlacementQueueKey(
	aci kueueinformerv1beta1.AdmissionCheckInformer) func(obj runtime.Object) []string {
	return func(obj runtime.Object) []string {
		key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
		if err != nil {
			utilruntime.HandleError(err)
			return []string{}
		}

		objs, err := aci.Informer().GetIndexer().ByIndex(AdmissionCheckByPlacement, key)
		if err != nil {
			utilruntime.HandleError(err)
			return []string{}
		}

		var keys []string
		for _, o := range objs {
			ac := o.(*kueuev1beta1.AdmissionCheck)
			klog.V(4).Infof("enqueue AdmissionCheck %s, because of placement %s", ac.Name, key)
			keys = append(keys, ac.Name)
		}

		return keys
	}
}

func AdmissionCheckByPlacementDecisionQueueKey(
	aci kueueinformerv1beta1.AdmissionCheckInformer) func(obj runtime.Object) []string {
	return func(obj runtime.Object) []string {
		accessor, _ := meta.Accessor(obj)
		placementName, ok := accessor.GetLabels()[clusterv1beta1.PlacementLabel]
		if !ok {
			return []string{}
		}

		objs, err := aci.Informer().GetIndexer().ByIndex(AdmissionCheckByPlacement,
			fmt.Sprintf("%s/%s", accessor.GetNamespace(), placementName))
		if err != nil {
			utilruntime.HandleError(err)
			return []string{}
		}

		var keys []string
		for _, o := range objs {
			ac := o.(*kueuev1beta1.AdmissionCheck)
			klog.V(4).Infof("enqueue AdmissionCheck %s, because of placementDecision %s/%s",
				ac.Name, accessor.GetNamespace(), accessor.GetName())
			keys = append(keys, ac.Name)
		}

		return keys
	}
}
