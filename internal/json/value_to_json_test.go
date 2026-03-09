package json_test

import (
	"testing"

	"github.com/chrismarget/imperative-terraform/internal/json"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/require"
)

func TestValueTo(t *testing.T) {
	type testCase struct {
		data      tftypes.Value
		expected  string
		expectErr string
	}

	testCases := map[string]testCase{
		"simple_string": {
			data:     tftypes.NewValue(tftypes.String, "chris"),
			expected: `"chris"`,
		},
		"null_string": {
			data:     tftypes.NewValue(tftypes.String, nil),
			expected: `null`,
		},
		"simple_bool_true": {
			data:     tftypes.NewValue(tftypes.Bool, true),
			expected: `true`,
		},
		"simple_bool_false": {
			data:     tftypes.NewValue(tftypes.Bool, false),
			expected: `false`,
		},
		"null_bool": {
			data:     tftypes.NewValue(tftypes.Bool, nil),
			expected: `null`,
		},
		"simple_number_int": {
			data:     tftypes.NewValue(tftypes.Number, 42),
			expected: `42`,
		},
		"simple_number_float": {
			data:     tftypes.NewValue(tftypes.Number, 3.14),
			expected: `3.14`,
		},
		"simple_number_zero": {
			data:     tftypes.NewValue(tftypes.Number, 0),
			expected: `0`,
		},
		"simple_number_negative": {
			data:     tftypes.NewValue(tftypes.Number, -123),
			expected: `-123`,
		},
		"null_number": {
			data:     tftypes.NewValue(tftypes.Number, nil),
			expected: `null`,
		},
		"object_with_string": {
			data: tftypes.NewValue(
				tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String}},
				map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, "chris")},
			),
			expected: `{"name":"chris"}`,
		},
		"object_with_null_string": {
			data: tftypes.NewValue(
				tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String}},
				map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, nil)},
			),
			expected: `{"name":null}`,
		},
		"object_with_mixed_types": {
			data: tftypes.NewValue(
				tftypes.Object{AttributeTypes: map[string]tftypes.Type{
					"name":    tftypes.String,
					"age":     tftypes.Number,
					"enabled": tftypes.Bool,
				}},
				map[string]tftypes.Value{
					"name":    tftypes.NewValue(tftypes.String, "chris"),
					"age":     tftypes.NewValue(tftypes.Number, 30),
					"enabled": tftypes.NewValue(tftypes.Bool, true),
				},
			),
			expected: `{"name":"chris","age":30,"enabled":true}`,
		},
		"list_of_strings": {
			data: tftypes.NewValue(
				tftypes.List{ElementType: tftypes.String},
				[]tftypes.Value{
					tftypes.NewValue(tftypes.String, "foo"),
					tftypes.NewValue(tftypes.String, "bar"),
					tftypes.NewValue(tftypes.String, nil),
					tftypes.NewValue(tftypes.String, "baz"),
				},
			),
			expected: `["foo","bar",null,"baz"]`,
		},
		"list_of_numbers": {
			data: tftypes.NewValue(
				tftypes.List{ElementType: tftypes.Number},
				[]tftypes.Value{
					tftypes.NewValue(tftypes.Number, 1),
					tftypes.NewValue(tftypes.Number, 2.5),
					tftypes.NewValue(tftypes.Number, nil),
					tftypes.NewValue(tftypes.Number, -10),
				},
			),
			expected: `[1,2.5,null,-10]`,
		},
		"set_of_strings": {
			data: tftypes.NewValue(
				tftypes.Set{ElementType: tftypes.String},
				[]tftypes.Value{
					tftypes.NewValue(tftypes.String, "alpha"),
					tftypes.NewValue(tftypes.String, "beta"),
					tftypes.NewValue(tftypes.String, nil),
					tftypes.NewValue(tftypes.String, "gamma"),
				},
			),
			expected: `["alpha","beta",null,"gamma"]`,
		},
		"set_of_numbers": {
			data: tftypes.NewValue(
				tftypes.Set{ElementType: tftypes.Number},
				[]tftypes.Value{
					tftypes.NewValue(tftypes.Number, 10),
					tftypes.NewValue(tftypes.Number, 20),
					tftypes.NewValue(tftypes.Number, nil),
					tftypes.NewValue(tftypes.Number, 30),
				},
			),
			expected: `[10,20,null,30]`,
		},
		"nested_object": {
			data: tftypes.NewValue(
				tftypes.Object{AttributeTypes: map[string]tftypes.Type{
					"name": tftypes.String,
					"address": tftypes.Object{AttributeTypes: map[string]tftypes.Type{
						"street": tftypes.String,
						"city":   tftypes.String,
						"zip":    tftypes.Number,
					}},
					"contacts": tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{
						"type":  tftypes.String,
						"value": tftypes.String,
					}}},
				}},
				map[string]tftypes.Value{
					"name": tftypes.NewValue(tftypes.String, "John Doe"),
					"address": tftypes.NewValue(
						tftypes.Object{AttributeTypes: map[string]tftypes.Type{
							"street": tftypes.String,
							"city":   tftypes.String,
							"zip":    tftypes.Number,
						}},
						map[string]tftypes.Value{
							"street": tftypes.NewValue(tftypes.String, "123 Main St"),
							"city":   tftypes.NewValue(tftypes.String, "Springfield"),
							"zip":    tftypes.NewValue(tftypes.Number, 12345),
						},
					),
					"contacts": tftypes.NewValue(
						tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{
							"type":  tftypes.String,
							"value": tftypes.String,
						}}},
						[]tftypes.Value{
							tftypes.NewValue(
								tftypes.Object{AttributeTypes: map[string]tftypes.Type{
									"type":  tftypes.String,
									"value": tftypes.String,
								}},
								map[string]tftypes.Value{
									"type":  tftypes.NewValue(tftypes.String, "email"),
									"value": tftypes.NewValue(tftypes.String, "john@example.com"),
								},
							),
							tftypes.NewValue(
								tftypes.Object{AttributeTypes: map[string]tftypes.Type{
									"type":  tftypes.String,
									"value": tftypes.String,
								}},
								map[string]tftypes.Value{
									"type":  tftypes.NewValue(tftypes.String, "phone"),
									"value": tftypes.NewValue(tftypes.String, "555-1234"),
								},
							),
						},
					),
				},
			),
			expected: `{
				"name": "John Doe",
				"address": {
					"street": "123 Main St",
					"city": "Springfield",
					"zip": 12345
				},
				"contacts": [
					{"type": "email", "value": "john@example.com"},
					{"type": "phone", "value": "555-1234"}
				]
			}`,
		},
	}

	for tName, tCase := range testCases {
		t.Run(tName, func(t *testing.T) {
			t.Parallel()
			result, err := json.ValueTo(tCase.data)
			if tCase.expectErr != "" {
				require.Error(t, err, tCase.expectErr)
				require.ErrorContains(t, err, tCase.expectErr)
				return
			}

			require.NoError(t, err)
			require.JSONEq(t, tCase.expected, string(result))
		})
	}
}
