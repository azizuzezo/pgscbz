package sociabuzz

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// parseJSON unmarshals a JSON string into the target value.
func parseJSON(data string, v any) error {
	return json.Unmarshal([]byte(data), v)
}

// parseJSONMap unmarshals a JSON string into map[string]any.
func parseJSONMap(data string) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// toFloat64 safely extracts a float64 from any JSON-compatible number type.
func toFloat64(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case string:
		return strconv.ParseFloat(val, 64)
	case json.Number:
		return val.Float64()
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// formatAmount formats a float for form POST fields (int if whole, else minimal decimals).
func formatAmount(v float64) string {
	if v == math.Trunc(v) && !math.IsInf(v, 0) {
		return strconv.Itoa(int(v))
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}