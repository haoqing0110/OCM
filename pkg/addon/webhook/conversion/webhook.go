package conversion

import (
	"encoding/json"
	"fmt"
	"net/http"

	apiv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addonv1beta1 "open-cluster-management.io/api/addon/v1beta1"
)

var (
	scheme = runtime.NewScheme()
	codecs = serializer.NewCodecFactory(scheme)
)

func init() {
	addonv1alpha1.Install(scheme)
	addonv1beta1.Install(scheme)
	apiv1.AddToScheme(scheme)
}

// ConversionWebhookHandler handles conversion webhook requests
type ConversionWebhookHandler struct{}

func (h *ConversionWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var review apiv1.ConversionReview

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&review); err != nil {
		klog.Errorf("failed to decode conversion review: %v", err)
		http.Error(w, fmt.Sprintf("failed to decode body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Convert the objects
	convertedObjects := make([]runtime.RawExtension, len(review.Request.Objects))
	for i, obj := range review.Request.Objects {
		converted, err := h.convert(obj, review.Request.DesiredAPIVersion)
		if err != nil {
			klog.Errorf("failed to convert object: %v", err)
			review.Response = &apiv1.ConversionResponse{
				UID:    review.Request.UID,
				Result: metav1.Status{
					Status:  metav1.StatusFailure,
					Message: err.Error(),
				},
			}
			writeResponse(w, &review)
			return
		}
		convertedObjects[i] = runtime.RawExtension{Object: converted}
	}

	review.Response = &apiv1.ConversionResponse{
		UID:              review.Request.UID,
		ConvertedObjects: convertedObjects,
		Result: metav1.Status{
			Status: metav1.StatusSuccess,
		},
	}

	writeResponse(w, &review)
}

func (h *ConversionWebhookHandler) convert(obj runtime.RawExtension, targetVersion string) (runtime.Object, error) {
	// Decode the object
	u := &unstructured.Unstructured{}
	if err := json.Unmarshal(obj.Raw, u); err != nil {
		return nil, fmt.Errorf("failed to unmarshal object: %w", err)
	}

	gvk := u.GroupVersionKind()

	// Handle ClusterManagementAddOn conversion
	if gvk.Group == "addon.open-cluster-management.io" && gvk.Kind == "ClusterManagementAddOn" {
		return h.convertClusterManagementAddOn(u, targetVersion)
	}

	// Handle ManagedClusterAddOn conversion
	if gvk.Group == "addon.open-cluster-management.io" && gvk.Kind == "ManagedClusterAddOn" {
		return h.convertManagedClusterAddOn(u, targetVersion)
	}

	return nil, fmt.Errorf("unsupported GVK: %v", gvk)
}

func (h *ConversionWebhookHandler) convertClusterManagementAddOn(u *unstructured.Unstructured, targetVersion string) (runtime.Object, error) {
	sourceVersion := u.GetAPIVersion()

	// v1alpha1 -> v1beta1
	if sourceVersion == "addon.open-cluster-management.io/v1alpha1" && targetVersion == "addon.open-cluster-management.io/v1beta1" {
		src := &addonv1alpha1.ClusterManagementAddOn{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, src); err != nil {
			return nil, err
		}
		dst := &addonv1beta1.ClusterManagementAddOn{}
		if err := ConvertClusterManagementAddOnToV1Beta1(src, dst); err != nil {
			return nil, err
		}
		return dst, nil
	}

	// v1beta1 -> v1alpha1
	if sourceVersion == "addon.open-cluster-management.io/v1beta1" && targetVersion == "addon.open-cluster-management.io/v1alpha1" {
		src := &addonv1beta1.ClusterManagementAddOn{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, src); err != nil {
			return nil, err
		}
		dst := &addonv1alpha1.ClusterManagementAddOn{}
		if err := ConvertClusterManagementAddOnFromV1Beta1(src, dst); err != nil {
			return nil, err
		}
		return dst, nil
	}

	return nil, fmt.Errorf("unsupported conversion from %s to %s", sourceVersion, targetVersion)
}

func (h *ConversionWebhookHandler) convertManagedClusterAddOn(u *unstructured.Unstructured, targetVersion string) (runtime.Object, error) {
	sourceVersion := u.GetAPIVersion()

	// v1alpha1 -> v1beta1
	if sourceVersion == "addon.open-cluster-management.io/v1alpha1" && targetVersion == "addon.open-cluster-management.io/v1beta1" {
		src := &addonv1alpha1.ManagedClusterAddOn{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, src); err != nil {
			return nil, err
		}
		dst := &addonv1beta1.ManagedClusterAddOn{}
		if err := ConvertManagedClusterAddOnToV1Beta1(src, dst); err != nil {
			return nil, err
		}
		return dst, nil
	}

	// v1beta1 -> v1alpha1
	if sourceVersion == "addon.open-cluster-management.io/v1beta1" && targetVersion == "addon.open-cluster-management.io/v1alpha1" {
		src := &addonv1beta1.ManagedClusterAddOn{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, src); err != nil {
			return nil, err
		}
		dst := &addonv1alpha1.ManagedClusterAddOn{}
		if err := ConvertManagedClusterAddOnFromV1Beta1(src, dst); err != nil {
			return nil, err
		}
		return dst, nil
	}

	return nil, fmt.Errorf("unsupported conversion from %s to %s", sourceVersion, targetVersion)
}

func writeResponse(w http.ResponseWriter, review *apiv1.ConversionReview) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(review)
}

// RegisterConversionWebhook registers the conversion webhook with the manager
func RegisterConversionWebhook(mgr ctrl.Manager) error {
	handler := &ConversionWebhookHandler{}
	mgr.GetWebhookServer().Register("/convert", handler)
	klog.Info("Registered conversion webhook at /convert")
	return nil
}
