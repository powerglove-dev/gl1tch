package research

import (
	"bytes"
	"encoding/json"
)

// jsonUnmarshalStrict decodes a JSON value with DisallowUnknownFields off
// (we want planners and pipelines to be able to add fields safely) but with
// UseNumber on so that integer fields like schema_version round-trip without
// floating-point silliness.
func jsonUnmarshalStrict(text string, dst any) error {
	dec := json.NewDecoder(bytes.NewReader([]byte(text)))
	dec.UseNumber()
	return dec.Decode(dst)
}
