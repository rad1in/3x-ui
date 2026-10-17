package service

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
)

// A saved decoy must be one the servers can serve; a bad one is refused at
// save time rather than silently turning into the plain 404 on restart.
func TestDecoySettingsAreValidatedOnSave(t *testing.T) {
	cases := []struct {
		mode, target, wantErr string
	}{
		{"", "", ""},
		{"nginx", "", ""},
		{"redirect", "https://www.example.org/", ""},
		{"redirect", "www.example.org", "decoy redirect target"},
		{"redirect", "javascript:alert(1)", "decoy redirect target"},
		{"teapot", "", "unknown decoy mode"},
	}
	for _, c := range cases {
		err := validateSettingsURLs(&entity.AllSetting{DecoyMode: c.mode, DecoyTarget: c.target})
		if (c.wantErr == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("mode %q target %q: err = %v, want %q", c.mode, c.target, err, c.wantErr)
		}
	}
}
