package service

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Check outcomes, worst last: a skipped check never degrades the report.
const (
	HealthSkip = "skip"
	HealthOK   = "ok"
	HealthWarn = "warn"
	HealthFail = "fail"
)

const (
	diskWarnPercent  = 85
	diskFailPercent  = 95
	certWarnDays     = 14
	healthDBDeadline = 2 * time.Second
)

// HealthCheck is the outcome of one probe.
type HealthCheck struct {
	Name   string `json:"name" example:"database"`
	Status string `json:"status" example:"ok"`
	Detail string `json:"detail,omitempty" example:"3 of 3 enabled nodes online"`
}

// HealthReport is the panel's overall state, the worst of its checks.
type HealthReport struct {
	Status    string        `json:"status" example:"ok"`
	CheckedAt int64         `json:"checkedAt" example:"1760000000000"`
	Checks    []HealthCheck `json:"checks"`
}

// HealthService probes the database, Xray, nodes, disk and TLS certificates.
type HealthService struct {
	xrayRunning func() bool
	diskPercent func() (float64, error)
}

func (s *HealthService) isXrayRunning() bool {
	if s.xrayRunning != nil {
		return s.xrayRunning()
	}
	return (&XrayService{}).IsXrayRunning()
}

func (s *HealthService) rootDiskPercent() (float64, error) {
	if s.diskPercent != nil {
		return s.diskPercent()
	}
	usage, err := disk.Usage("/")
	if err != nil {
		return 0, err
	}
	return usage.UsedPercent, nil
}

// Check runs every probe once. The cert and node probes read the database, so
// they report a failure of their own rather than panic when it is down.
func (s *HealthService) Check(now time.Time) *HealthReport {
	checks := []HealthCheck{
		s.checkDatabase(),
		s.checkXray(),
		s.checkNodes(),
		s.checkDisk(),
		s.checkCerts(now),
	}
	report := &HealthReport{Status: HealthOK, CheckedAt: now.UnixMilli(), Checks: checks}
	for _, c := range checks {
		if healthRank(c.Status) > healthRank(report.Status) {
			report.Status = c.Status
		}
	}
	return report
}

func healthRank(status string) int {
	switch status {
	case HealthFail:
		return 3
	case HealthWarn:
		return 2
	case HealthOK:
		return 1
	}
	return 0
}

func (s *HealthService) checkDatabase() HealthCheck {
	c := HealthCheck{Name: "database", Status: HealthOK}
	db := database.GetDB()
	if db == nil {
		return HealthCheck{Name: "database", Status: HealthFail, Detail: "not initialised"}
	}
	sqlDB, err := db.DB()
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), healthDBDeadline)
		defer cancel()
		err = sqlDB.PingContext(ctx)
	}
	if err != nil {
		c.Status, c.Detail = HealthFail, err.Error()
	}
	return c
}

func (s *HealthService) checkXray() HealthCheck {
	if s.isXrayRunning() {
		return HealthCheck{Name: "xray", Status: HealthOK}
	}
	return HealthCheck{Name: "xray", Status: HealthFail, Detail: "not running"}
}

func (s *HealthService) checkNodes() HealthCheck {
	c := HealthCheck{Name: "nodes", Status: HealthSkip}
	var nodes []model.Node
	if err := database.GetDB().Where("enable = ?", true).Find(&nodes).Error; err != nil {
		return HealthCheck{Name: "nodes", Status: HealthFail, Detail: err.Error()}
	}
	if len(nodes) == 0 {
		return c
	}
	offline := 0
	for _, n := range nodes {
		if n.Status == "offline" {
			offline++
		}
	}
	c.Status = HealthOK
	c.Detail = fmt.Sprintf("%d of %d enabled nodes online", len(nodes)-offline, len(nodes))
	if offline > 0 {
		c.Status = HealthWarn
		c.Detail = fmt.Sprintf("%d of %d enabled nodes offline", offline, len(nodes))
	}
	if offline == len(nodes) {
		c.Status = HealthFail
	}
	return c
}

func (s *HealthService) checkDisk() HealthCheck {
	percent, err := s.rootDiskPercent()
	if err != nil {
		return HealthCheck{Name: "disk", Status: HealthWarn, Detail: err.Error()}
	}
	c := HealthCheck{Name: "disk", Status: HealthOK, Detail: fmt.Sprintf("%.0f%% used", percent)}
	switch {
	case percent >= diskFailPercent:
		c.Status = HealthFail
	case percent >= diskWarnPercent:
		c.Status = HealthWarn
	}
	return c
}

// checkCerts looks at the panel's and the subscription server's certificate
// files and reports the one expiring first.
func (s *HealthService) checkCerts(now time.Time) HealthCheck {
	c := HealthCheck{Name: "cert", Status: HealthSkip}
	settings := &SettingService{}
	panelCert, err1 := settings.GetCertFile()
	subCert, err2 := settings.GetSubCertFile()
	if err1 != nil || err2 != nil {
		return HealthCheck{Name: "cert", Status: HealthFail, Detail: "cannot read the certificate settings"}
	}
	seen := map[string]bool{}
	for _, path := range []string{panelCert, subCert} {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		one := certHealth(path, now)
		if healthRank(one.Status) > healthRank(c.Status) || c.Detail == "" {
			c = one
		}
	}
	return c
}

func certHealth(path string, now time.Time) HealthCheck {
	c := HealthCheck{Name: "cert", Status: HealthFail}
	raw, err := os.ReadFile(path)
	if err != nil {
		c.Detail = fmt.Sprintf("%s: %v", path, err)
		return c
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		c.Detail = path + ": not a PEM certificate"
		return c
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		c.Detail = fmt.Sprintf("%s: %v", path, err)
		return c
	}
	days := int(cert.NotAfter.Sub(now).Hours() / 24)
	switch {
	case !now.Before(cert.NotAfter):
		c.Detail = fmt.Sprintf("%s expired on %s", path, cert.NotAfter.Format("2006-01-02"))
	case days < certWarnDays:
		c.Status, c.Detail = HealthWarn, fmt.Sprintf("%s expires in %d days", path, days)
	default:
		c.Status, c.Detail = HealthOK, fmt.Sprintf("%s expires in %d days", path, days)
	}
	return c
}
