package common

import (
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/ext"
	"github.com/google/cel-go/interpreter"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/cel/library"
)

// BaseEnvOpts defines the base CEL environment options
var BaseEnvOpts = []cel.EnvOption{
	cel.OptionalTypes(),
	ext.Strings(),
	library.Lists(),
	library.Regex(),
	library.URLs(),
	library.Quantity(),
	library.IP(),
	library.CIDR(),
	library.Format(),
}

// BaseEnvCostEstimator implements CEL's interpretable.ActualCostEstimator
type BaseEnvCostEstimator struct {
	// Wraps a CEL cost estimator with additional functionality
	CostEstimator interpreter.ActualCostEstimator
}

// CallCost implements runtime cost estimation for CEL function calls
func (b *BaseEnvCostEstimator) CallCost(function, overloadId string, args []ref.Val, result ref.Val) *uint64 {
	k8sEstimator := &library.CostEstimator{}
	if cost := k8sEstimator.CallCost(function, overloadId, args, result); cost != nil {
		return cost
	}
	return b.CostEstimator.CallCost(function, overloadId, args, result)
}

// ConvertObjectToUnstructured converts any object to an unstructured.Unstructured object
func ConvertObjectToUnstructured(obj interface{}) (*unstructured.Unstructured, error) {
	if obj == nil || reflect.ValueOf(obj).IsNil() {
		return &unstructured.Unstructured{Object: nil}, nil
	}
	ret, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: ret}, nil
}
