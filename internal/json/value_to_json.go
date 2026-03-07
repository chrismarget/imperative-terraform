package json

import (
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// ValueToJSON converts a tftypes.Value to JSON bytes, handling all Terraform types.
// This reverses what tftypes.ValueFromJSON does.
func ValueToJSON(val tftypes.Value) ([]byte, error) {
	// Convert the tftypes.Value to a generic Go value
	goVal, err := tftypesToGoValue(val)
	if err != nil {
		return nil, fmt.Errorf("converting tftypes.Value to go value: %w", err)
	}

	// Marshal to JSON
	data, err := json.Marshal(goVal)
	if err != nil {
		return nil, fmt.Errorf("marshaling to json: %w", err)
	}

	return data, nil
}

// tftypesToGoValue recursively converts a tftypes.Value to a Go value
// that can be JSON-marshaled.
func tftypesToGoValue(val tftypes.Value) (interface{}, error) {
	// Handle null values
	if val.IsNull() {
		return nil, nil
	}

	// Handle unknown values (not representable in JSON)
	if !val.IsKnown() {
		return nil, fmt.Errorf("cannot convert unknown value to JSON")
	}

	// Get the type to determine how to handle the value
	typ := val.Type()

	// Try to handle as primitives first by attempting conversions
	var s string
	if err := val.As(&s); err == nil {
		return s, nil
	}

	var n *big.Float
	if err := val.As(&n); err == nil {
		if n == nil {
			return nil, nil
		}
		f, _ := n.Float64()
		return f, nil
	}

	var b bool
	if err := val.As(&b); err == nil {
		return b, nil
	}

	// Handle collection types
	switch typ.(type) {
	case tftypes.List:
		return convertList(val)

	case tftypes.Set:
		return convertSet(val)

	case tftypes.Tuple:
		return convertTuple(val)

	case tftypes.Map:
		return convertMap(val)

	case tftypes.Object:
		return convertObject(val)

	default:
		return nil, fmt.Errorf("unsupported type: %T", typ)
	}
}

func convertList(val tftypes.Value) (interface{}, error) {
	var values []tftypes.Value
	if err := val.As(&values); err != nil {
		return nil, fmt.Errorf("converting list: %w", err)
	}

	result := make([]interface{}, len(values))
	for i, v := range values {
		goVal, err := tftypesToGoValue(v)
		if err != nil {
			return nil, fmt.Errorf("converting list element %d: %w", i, err)
		}
		result[i] = goVal
	}
	return result, nil
}

func convertSet(val tftypes.Value) (interface{}, error) {
	var values []tftypes.Value
	if err := val.As(&values); err != nil {
		return nil, fmt.Errorf("converting set: %w", err)
	}

	result := make([]interface{}, len(values))
	for i, v := range values {
		goVal, err := tftypesToGoValue(v)
		if err != nil {
			return nil, fmt.Errorf("converting set element %d: %w", i, err)
		}
		result[i] = goVal
	}
	return result, nil
}

func convertTuple(val tftypes.Value) (interface{}, error) {
	var values []tftypes.Value
	if err := val.As(&values); err != nil {
		return nil, fmt.Errorf("converting tuple: %w", err)
	}

	result := make([]interface{}, len(values))
	for i, v := range values {
		goVal, err := tftypesToGoValue(v)
		if err != nil {
			return nil, fmt.Errorf("converting tuple element %d: %w", i, err)
		}
		result[i] = goVal
	}
	return result, nil
}

func convertMap(val tftypes.Value) (interface{}, error) {
	var values map[string]tftypes.Value
	if err := val.As(&values); err != nil {
		return nil, fmt.Errorf("converting map: %w", err)
	}

	result := make(map[string]interface{}, len(values))
	for k, v := range values {
		goVal, err := tftypesToGoValue(v)
		if err != nil {
			return nil, fmt.Errorf("converting map value for key %q: %w", k, err)
		}
		result[k] = goVal
	}
	return result, nil
}

func convertObject(val tftypes.Value) (interface{}, error) {
	var values map[string]tftypes.Value
	if err := val.As(&values); err != nil {
		return nil, fmt.Errorf("converting object: %w", err)
	}

	result := make(map[string]interface{}, len(values))
	for k, v := range values {
		goVal, err := tftypesToGoValue(v)
		if err != nil {
			return nil, fmt.Errorf("converting object attribute %q: %w", k, err)
		}
		result[k] = goVal
	}
	return result, nil
}
