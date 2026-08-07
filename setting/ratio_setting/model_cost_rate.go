package ratio_setting

import (
	"github.com/QuantumNous/new-api/types"
)

// defaultModelCostRate holds built-in per-model cost rates.
// An absent entry means the model uses the default cost rate of 1.0.
var defaultModelCostRate = map[string]float64{}

var modelCostRateMap = types.NewRWMap[string, float64]()

// GetModelCostRate returns the per-request cost rate for a model.
// When a model has no explicit cost rate, it defaults to 1.0.
func GetModelCostRate(name string) float64 {
	name = FormatMatchingModelName(name)

	if rate, ok := modelCostRateMap.Get(name); ok {
		return rate
	}

	return 1.0
}

// ModelCostRate2JSONString converts the cost rate map to a JSON string.
func ModelCostRate2JSONString() string {
	return modelCostRateMap.MarshalJSONString()
}

// UpdateModelCostRateByJSONString updates the cost rate map from a JSON string.
func UpdateModelCostRateByJSONString(jsonStr string) error {
	return types.LoadFromJsonStringWithCallback(modelCostRateMap, jsonStr, InvalidateExposedDataCache)
}

// GetModelCostRateCopy returns a copy of the cost rate map.
func GetModelCostRateCopy() map[string]float64 {
	return modelCostRateMap.ReadAll()
}
