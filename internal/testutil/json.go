package testutil

import (
	"encoding/json"
	"testing"
)

// DecodeJSONMap decodes a JSON object into a generic map for assertions.
func DecodeJSONMap(t *testing.T, data []byte) map[string]any {
	t.Helper()

	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("decode json: %v\nbody=%s", err, string(data))
	}
	return v
}
