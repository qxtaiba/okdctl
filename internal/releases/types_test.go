package releases

import (
	"encoding/json"
	"testing"
)

func TestReleaseTypeUnmarshalJSONUnknown(t *testing.T) {
	var rt ReleaseType
	err := json.Unmarshal([]byte(`"lts-preview"`), &rt)
	if err == nil {
		t.Errorf("expected error for unknown release type, got nil; rt=%v", rt)
	}
}
