package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDBPathLegacyFallback(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{name: "empty folder uses the new default", want: "eternal.db"},
		{name: "legacy only keeps the legacy file", files: []string{"x-ui.db"}, want: "x-ui.db"},
		{name: "new file wins over legacy", files: []string{"x-ui.db", "eternal.db"}, want: "eternal.db"},
		{name: "new file alone", files: []string{"eternal.db"}, want: "eternal.db"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XUI_DB_FOLDER", dir)
			for _, f := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want := dir + "/" + tt.want
			if got := GetDBPath(); got != want {
				t.Fatalf("GetDBPath = %q, want %q", got, want)
			}
		})
	}
}

func TestGetPanelVersion(t *testing.T) {
	orig := buildCommit
	t.Cleanup(func() { buildCommit = orig })

	buildCommit = ""
	if got := GetPanelVersion(); got != GetBaseVersion() {
		t.Fatalf("stable build: GetPanelVersion = %q, want %q", got, GetBaseVersion())
	}

	buildCommit = "1d1128cf"
	if got := GetPanelVersion(); got != "dev+1d1128cf" {
		t.Fatalf("dev build: GetPanelVersion = %q, want %q", got, "dev+1d1128cf")
	}

	buildCommit = "1d1128cf945c4615efa05cf41ba7fa766e2ee428"
	if got := GetPanelVersion(); got != "dev+1d1128cf" {
		t.Fatalf("dev build (full sha): GetPanelVersion = %q, want %q", got, "dev+1d1128cf")
	}
}

func TestGetPortOverride(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		set        bool
		wantPort   int
		configured bool
		wantErr    bool
	}{
		{name: "unset"},
		{name: "empty", value: "", set: true},
		{name: "whitespace", value: "   ", set: true},
		{name: "minimum", value: "1", set: true, wantPort: 1, configured: true},
		{name: "default panel port", value: "2053", set: true, wantPort: 2053, configured: true},
		{name: "surrounding whitespace", value: " 8080 ", set: true, wantPort: 8080, configured: true},
		{name: "maximum", value: "65535", set: true, wantPort: 65535, configured: true},
		{name: "zero", value: "0", set: true, configured: true, wantErr: true},
		{name: "above maximum", value: "65536", set: true, configured: true, wantErr: true},
		{name: "negative", value: "-1", set: true, configured: true, wantErr: true},
		{name: "non-numeric", value: "abc", set: true, configured: true, wantErr: true},
		{name: "decimal", value: "8080.0", set: true, configured: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("XUI_PORT", tt.value)
			} else {
				original, existed := os.LookupEnv("XUI_PORT")
				if err := os.Unsetenv("XUI_PORT"); err != nil {
					t.Fatalf("unset XUI_PORT: %v", err)
				}
				t.Cleanup(func() {
					if existed {
						_ = os.Setenv("XUI_PORT", original)
					} else {
						_ = os.Unsetenv("XUI_PORT")
					}
				})
			}

			port, configured, err := GetPortOverride()
			if port != tt.wantPort {
				t.Errorf("port = %d, want %d", port, tt.wantPort)
			}
			if configured != tt.configured {
				t.Errorf("configured = %t, want %t", configured, tt.configured)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestLoopbackIP(t *testing.T) {
	cases := map[string]string{
		"":            "127.0.0.1",
		"127.0.0.7":   "127.0.0.7",
		" 127.3.2.1 ": "127.3.2.1",
		"10.0.0.1":    "127.0.0.1",
		"::1":         "127.0.0.1",
		"not-an-ip":   "127.0.0.1",
		"127.0.0.0/8": "127.0.0.1",
	}
	for env, want := range cases {
		t.Setenv("XUI_LOOPBACK_IP", env)
		if got := LoopbackIP(); got != want {
			t.Errorf("XUI_LOOPBACK_IP=%q: LoopbackIP() = %q, want %q", env, got, want)
		}
	}
}
