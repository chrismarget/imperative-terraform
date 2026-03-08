package json

import "github.com/hashicorp/terraform-plugin-go/tftypes"

func ValueFrom(b []byte, t tftypes.Type) (tftypes.Value, error) {
	return tftypes.ValueFromJSONWithOpts(b, t, tftypes.ValueFromJSONOpts{IgnoreUndefinedAttributes: true})
}
