package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func writeCert(t *testing.T, notAfter time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "panel"}, NotBefore: notAfter.Add(-90 * 24 * time.Hour), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cert.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func healthByName(t *testing.T, r *HealthReport, name string) HealthCheck {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %q check in %+v", name, r.Checks)
	return HealthCheck{}
}

func newHealthService(running bool, diskPercent float64) *HealthService {
	return &HealthService{
		xrayRunning: func() bool { return running },
		diskPercent: func() (float64, error) { return diskPercent, nil },
	}
}

func TestHealthIsOkWhenEverythingWorksAndSkipsWhatIsNotConfigured(t *testing.T) {
	setupBulkDB(t)
	r := newHealthService(true, 40).Check(time.Now())
	if r.Status != HealthOK {
		t.Fatalf("report = %+v, want ok", r)
	}
	if c := healthByName(t, r, "nodes"); c.Status != HealthSkip {
		t.Fatalf("nodes = %+v, want skipped without nodes", c)
	}
	if c := healthByName(t, r, "cert"); c.Status != HealthSkip {
		t.Fatalf("cert = %+v, want skipped without a certificate", c)
	}
}

func TestHealthDegradesToTheWorstCheck(t *testing.T) {
	setupBulkDB(t)
	r := newHealthService(false, 90).Check(time.Now())
	if r.Status != HealthFail {
		t.Fatalf("status = %q, want fail with xray down", r.Status)
	}
	if c := healthByName(t, r, "xray"); c.Status != HealthFail {
		t.Fatalf("xray = %+v", c)
	}
	if c := healthByName(t, r, "disk"); c.Status != HealthWarn {
		t.Fatalf("disk at 90%% = %+v, want warn", c)
	}
	if c := healthByName(t, newHealthService(true, 97).Check(time.Now()), "disk"); c.Status != HealthFail {
		t.Fatalf("disk at 97%% = %+v, want fail", c)
	}
}

func TestHealthCountsOnlyEnabledOfflineNodes(t *testing.T) {
	setupBulkDB(t)
	db := database.GetDB()
	nodes := []model.Node{
		{Name: "up", Address: "10.0.0.1", Port: 2053, Scheme: "https", Enable: true, Status: "online"},
		{Name: "down", Address: "10.0.0.2", Port: 2053, Scheme: "https", Enable: true, Status: "offline"},
		{Name: "paused", Address: "10.0.0.3", Port: 2053, Scheme: "https", Status: "offline"},
	}
	for i := range nodes {
		if err := db.Create(&nodes[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&model.Node{}).Where("name = ?", "paused").Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	c := healthByName(t, newHealthService(true, 10).Check(time.Now()), "nodes")
	if c.Status != HealthWarn || c.Detail != "1 of 2 enabled nodes offline" {
		t.Fatalf("nodes = %+v, want a warning naming 1 of 2", c)
	}
}

func TestHealthWarnsBeforeACertificateExpiresAndFailsAfter(t *testing.T) {
	setupBulkDB(t)
	now := time.Now()
	settings := &SettingService{}
	for _, tc := range []struct {
		notAfter time.Time
		want     string
	}{
		{now.Add(60 * 24 * time.Hour), HealthOK},
		{now.Add(5 * 24 * time.Hour), HealthWarn},
		{now.Add(-time.Hour), HealthFail},
	} {
		if err := settings.SetCertFile(writeCert(t, tc.notAfter)); err != nil {
			t.Fatal(err)
		}
		if c := healthByName(t, newHealthService(true, 10).Check(now), "cert"); c.Status != tc.want {
			t.Fatalf("cert expiring %s = %+v, want %s", tc.notAfter.Format(time.RFC3339), c, tc.want)
		}
	}
}

func TestHealthFailsWhenTheDatabaseIsClosed(t *testing.T) {
	setupBulkDB(t)
	if err := database.CloseDB(); err != nil {
		t.Fatal(err)
	}
	r := newHealthService(true, 10).Check(time.Now())
	if c := healthByName(t, r, "database"); c.Status != HealthFail || r.Status != HealthFail {
		t.Fatalf("report = %+v, want a failed database check", r)
	}
}
