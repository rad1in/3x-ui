package panel

import (
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
)

const hostedHelperEnv = "XUI_HOSTED_PANEL_TEST_HELPER"

// TestMain lets the test binary stand in for the x-ui executable the master spawns.
func TestMain(m *testing.M) {
	if os.Getenv(hostedHelperEnv) != "" && len(os.Args) > 1 {
		switch os.Args[1] {
		case "hosted-init":
			if err := RunHostedInit(os.Stdin); err != nil {
				os.Stderr.WriteString(err.Error())
				os.Exit(1)
			}
			os.Exit(0)
		case "run":
			env := strings.Join([]string{os.Getenv(hosted.EnvDir), os.Getenv("XUI_LOOPBACK_IP"), os.Getenv("XUI_PORT")}, "\n")
			_ = os.WriteFile("run-env", []byte(env), 0o600)
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt)
			<-stop
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func setupHostedPanels(t *testing.T) *HostedPanelService {
	t.Helper()
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, config.GetDBPath())
	t.Setenv(hostedHelperEnv, "1")
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	prev := hostedExecutable
	hostedExecutable = func() (string, error) { return os.Args[0], nil }
	t.Cleanup(func() {
		StopHostedPanels()
		hostedExecutable = prev
	})
	return &HostedPanelService{}
}

func resellerInput() HostedPanelInput {
	return HostedPanelInput{
		Name: "Reseller A", Enable: true, Username: "reseller", Password: "first-password",
		WebPort: 20530, SubPort: 20531, WebBasePath: "resell",
		PortMin: 30000, PortMax: 30099, MaxInbounds: 3, MaxClients: 50, TrafficBytes: 10 << 30,
	}
}

func openChildDB(t *testing.T, id int) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(hostedPanelDir(id), "db", "x-ui.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func childSetting(t *testing.T, db *gorm.DB, key string) string {
	t.Helper()
	var s model.Setting
	if err := db.Where("key = ?", key).First(&s).Error; err != nil {
		t.Fatalf("child setting %s: %v", key, err)
	}
	return s.Value
}

func childUser(t *testing.T, db *gorm.DB) model.User {
	t.Helper()
	var u model.User
	if err := db.First(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}

func waitHosted(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestHostedPanelLifecycle(t *testing.T) {
	svc := setupHostedPanels(t)

	p, err := svc.Create(resellerInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	dir := hostedPanelDir(p.Id)

	child := openChildDB(t, p.Id)
	if got := childSetting(t, child, "webPort"); got != "20530" {
		t.Errorf("child webPort = %s", got)
	}
	if got := childSetting(t, child, "subPort"); got != "20531" {
		t.Errorf("child subPort = %s", got)
	}
	if got := childSetting(t, child, "webBasePath"); got != "/resell/" {
		t.Errorf("child webBasePath = %s", got)
	}
	if u := childUser(t, child); u.Username != "reseller" || !crypto.CheckPasswordHash(u.Password, "first-password") {
		t.Fatalf("child login = %q, password does not match the one the owner set", u.Username)
	}
	limits, err := hosted.ReadLimits(dir)
	if err != nil || limits.MaxClients != 50 || limits.PortMin != 30000 || limits.TrafficBytes != 10<<30 {
		t.Fatalf("limits.json = %+v, %v", limits, err)
	}

	waitHosted(t, "panel process", func() bool {
		_, err := os.Stat(filepath.Join(dir, "run-env"))
		return err == nil
	})
	env, _ := os.ReadFile(filepath.Join(dir, "run-env"))
	if want := dir + "\n127.77.0.2\n20530"; string(env) != want {
		t.Fatalf("panel process env = %q, want %q", env, want)
	}

	in := resellerInput()
	in.Password = "second-password"
	if _, err := svc.Update(p.Id, in); err != nil {
		t.Fatalf("password reset: %v", err)
	}
	if u := childUser(t, child); !crypto.CheckPasswordHash(u.Password, "second-password") {
		t.Fatal("password reset did not reach the hosted panel's database")
	}

	if err := svc.ResetUsage(p.Id); err != nil {
		t.Fatal(err)
	}
	if limits, _ := hosted.ReadLimits(dir); limits.UsageEpoch != 1 {
		t.Fatalf("usage reset left epoch %d, want 1", limits.UsageEpoch)
	}

	if err := svc.Delete(p.Id); err != nil {
		t.Fatal(err)
	}
	if st := hostedSupervisor.State(p.Id); st.Running {
		t.Fatalf("process still running after delete: %+v", st)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("hosted dir survives delete: %v", err)
	}
}

func TestHostedPanelInputRules(t *testing.T) {
	svc := setupHostedPanels(t)
	tests := []struct {
		name string
		edit func(*HostedPanelInput)
	}{
		{"no name", func(in *HostedPanelInput) { in.Name = "  " }},
		{"username with a space", func(in *HostedPanelInput) { in.Username = "re seller" }},
		{"short password", func(in *HostedPanelInput) { in.Password = "short" }},
		{"path with a query", func(in *HostedPanelInput) { in.WebBasePath = "/a?b/" }},
		{"negative quota", func(in *HostedPanelInput) { in.TrafficBytes = -1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := resellerInput()
			in.Enable = false
			tt.edit(&in)
			if _, err := svc.Create(in); !errors.Is(err, ErrHostedPanelInvalid) {
				t.Fatalf("err = %v, want ErrHostedPanelInvalid", err)
			}
		})
	}

	in := resellerInput()
	in.Enable = false
	p, err := svc.Create(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Username, in.Password = "renamed", ""
	if _, err := svc.Update(p.Id, in); !errors.Is(err, ErrHostedPanelInvalid) {
		t.Fatalf("renaming the login without a new password: err = %v", err)
	}
}

// A hosted panel inherits the master's environment; it must never inherit the
// master's database, port or folders, even when the env file sets them.
func TestHostedPanelEnvReplacesTheMastersOwnSettings(t *testing.T) {
	t.Setenv("XUI_DB_TYPE", "postgres")
	t.Setenv("XUI_DB_DSN", "postgres://master")
	t.Setenv("XUI_PORT", "2053")
	t.Setenv("XRAY_LOCATION_ASSET", "/usr/local/x-ui/bin")
	t.Setenv("XUI_LOG_LEVEL", "debug")
	env := hostedPanelEnv(&model.HostedPanel{Id: 251, WebPort: 20530})

	get := func(key string) []string {
		var vals []string
		for _, kv := range env {
			if k, v, _ := strings.Cut(kv, "="); k == key {
				vals = append(vals, v)
			}
		}
		return vals
	}
	dir := hostedPanelDir(251)
	for key, want := range map[string]string{
		"XUI_DB_TYPE":         "sqlite",
		"XUI_DB_DSN":          "",
		"XUI_PORT":            "20530",
		"XRAY_LOCATION_ASSET": filepath.Join(dir, "bin"),
		"XUI_LOOPBACK_IP":     "127.77.1.2",
		"XUI_LOG_LEVEL":       "debug",
	} {
		if got := get(key); !slices.Equal(got, []string{want}) {
			t.Errorf("%s = %q, want exactly [%q]", key, got, want)
		}
	}
}

func TestSyncHostedBinCopiesFilesButNotTheXrayConfig(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("xray-linux-amd64", "v1")
	write("geoip.dat", "geo")
	write("config.json", "master config")
	if err := syncHostedBin(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "config.json")); !os.IsNotExist(err) {
		t.Fatal("the master's Xray config was copied into a hosted panel")
	}
	if fi, err := os.Lstat(filepath.Join(dst, "xray-linux-amd64")); err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0o755 {
		t.Fatalf("binary copy = %v, %v; want a private executable file", fi, err)
	}

	write("xray-linux-amd64", "v2-updated")
	if err := syncHostedBin(src, dst); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "xray-linux-amd64")); string(got) != "v2-updated" {
		t.Fatalf("after a master update the copy holds %q", got)
	}
}
