package ocserv

import (
	"encoding/json"
	"testing"
)

func pkiJSON(t *testing.T) string {
	t.Helper()
	bs, err := json.Marshal(testPKI(t))
	if err != nil {
		t.Fatal(err)
	}
	return string(bs)
}
