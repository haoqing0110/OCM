package library

import (
	"math"

	"github.com/google/cel-go/common"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
)

// CostEstimator implements CEL's interpretable.ActualCostEstimator for runtime cost estimation
type CostEstimator struct{}

func actualSize(value ref.Val) uint64 {
	if sz, ok := value.(traits.Sizer); ok {
		return uint64(sz.Size().(types.Int))
	}
	return 1
}

// CallCost calculates the runtime cost for CEL function calls
func (l *CostEstimator) CallCost(function, overloadId string, args []ref.Val, result ref.Val) *uint64 {
	switch function {
	case "scores":
		itemsTotalCost := uint64(0)
		if result != nil {
			if lister, ok := result.(traits.Lister); ok {
				size := lister.Size().(types.Int)
				// each item is a map
				itemsTotalCost = uint64(size) * (common.SelectAndIdentCost + common.MapCreateBaseCost)
			}
		}
		// each scores returns a list
		totalCost := common.ListCreateBaseCost + itemsTotalCost
		return &totalCost
	case "parseJSON":
		if len(args) >= 1 {
			// Calculate the traversal cost of input string
			inputSize := actualSize(args[0])
			traversalCost := uint64(math.Ceil(float64(inputSize) * common.StringTraversalCostFactor))

			// Recursively calculate the cost of result structure
			structCost := calculateStructCost(result)

			totalCost := traversalCost + structCost
			return &totalCost
		}
		cost := uint64(common.MapCreateBaseCost)
		return &cost
	}
	return nil
}

// calculateStructCost recursively calculates the cost of data structures
func calculateStructCost(val ref.Val) uint64 {
	if val == nil {
		return 0
	}

	var cost uint64
	switch v := val.(type) {
	case traits.Mapper:
		cost += calculateMapCost(v)
	case traits.Lister:
		cost += calculateListCost(v)
	}
	return cost
}

// calculateMapCost computes cost for map structures
func calculateMapCost(v traits.Mapper) uint64 {
	cost := uint64(common.MapCreateBaseCost)
	it := v.Iterator()
	for it.HasNext() == types.True {
		key := it.Next()
		value := v.Get(key)
		cost += calculateStructCost(value)
	}
	return cost
}

// calculateListCost computes cost for list structures
func calculateListCost(v traits.Lister) uint64 {
	cost := uint64(common.ListCreateBaseCost)
	for i := types.Int(0); i < v.Size().(types.Int); i++ {
		item := v.Get(types.Int(i))
		cost += calculateStructCost(item)
	}
	return cost
}
