package panel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

const (
	hostedNameMaxRunes   = 64
	hostedUserMaxRunes   = 64
	hostedPasswordMinLen = 8
	hostedMaxPanels      = 62500
)

var (
	ErrHostedPanelInvalid  = errors.New("invalid hosted panel")
	ErrHostedPanelNotFound = errors.New("hosted panel not found")

	hostedBasePathPattern = regexp.MustCompile(`^/([A-Za-z0-9._~-]+/)*$`)

	hostedExecutable = os.Executable
	hostedSupervisor = hosted.NewSupervisor(hostedPanelCommand)
	// hostedPanelsMu serialises edits so a panel's row, files and process agree.
	hostedPanelsMu sync.Mutex
)

// HostedPanelInput is what the owner submits; an empty password on edit keeps the login.
type HostedPanelInput struct {
	Name         string `json:"name" example:"Reseller A"`
	Enable       bool   `json:"enable" example:"true"`
	Username     string `json:"username" example:"reseller"`
	Password     string `json:"password" example:"change-me-please"`
	WebPort      int    `json:"webPort" example:"20530"`
	SubPort      int    `json:"subPort" example:"20531"`
	WebBasePath  string `json:"webBasePath" example:"/reseller/"`
	PortMin      int    `json:"portMin" example:"30000"`
	PortMax      int    `json:"portMax" example:"30099"`
	MaxInbounds  int    `json:"maxInbounds" example:"5"`
	MaxClients   int    `json:"maxClients" example:"100"`
	TrafficBytes int64  `json:"trafficBytes" example:"107374182400"`
	ExpiresAt    int64  `json:"expiresAt" example:"0"`
}

// HostedPanelView is one row of the Panels page.
type HostedPanelView struct {
	model.HostedPanel
	Status  hosted.PanelStatus `json:"status"`
	Process hosted.ProcState   `json:"process"`
}

// HostedPanelService runs the panels this master hosts for other people.
type HostedPanelService struct{}

func hostedPanelDir(id int) string {
	return filepath.Join(config.GetDBFolderPath(), "hosted", strconv.Itoa(id))
}

// hostedLoopbackIP gives each panel its own loopback address, so the fixed
// internal ports (Xray API, relays) of siblings never collide.
func hostedLoopbackIP(id int) string {
	return fmt.Sprintf("127.77.%d.%d", id/250, id%250+1)
}

// hostedEnvOverrides are set for every hosted panel; godotenv never overrides a
// variable already present, so the service env file cannot undo them.
func hostedEnvOverrides(p *model.HostedPanel) map[string]string {
	dir := hostedPanelDir(p.Id)
	return map[string]string{
		hosted.EnvDir:               dir,
		"XUI_DB_TYPE":               "sqlite",
		"XUI_DB_DSN":                "",
		"XUI_DB_FOLDER":             filepath.Join(dir, "db"),
		"XUI_BIN_FOLDER":            filepath.Join(dir, "bin"),
		"XRAY_LOCATION_ASSET":       filepath.Join(dir, "bin"),
		"XUI_LOG_FOLDER":            filepath.Join(dir, "log"),
		"XUI_LOOPBACK_IP":           hostedLoopbackIP(p.Id),
		"XUI_PORT":                  strconv.Itoa(p.WebPort),
		"XUI_PPROF":                 "false",
		"XUI_TUNNEL_HEALTH_MONITOR": "false",
	}
}

func hostedPanelEnv(p *model.HostedPanel) []string {
	overrides := hostedEnvOverrides(p)
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if _, replaced := overrides[key]; replaced || key == "xray.location.asset" {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	return env
}

func hostedPanelCommand(p hosted.Panel) *exec.Cmd {
	exe, err := hostedExecutable()
	if err != nil {
		exe = ""
	}
	cmd := exec.Command(exe, "run")
	cmd.Env, cmd.Dir = p.Env, p.Dir
	return cmd
}

func hostedLimitsOf(p *model.HostedPanel) hosted.Limits {
	return hosted.Limits{
		MaxInbounds:  p.MaxInbounds,
		MaxClients:   p.MaxClients,
		TrafficBytes: p.TrafficBytes,
		ExpiresAt:    p.ExpiresAt,
		PortMin:      p.PortMin,
		PortMax:      p.PortMax,
		UsageEpoch:   p.UsageEpoch,
	}
}

func normalizeHostedBasePath(path string) string {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrHostedPanelInvalid}, args...)...)
}

// apply copies the editable fields onto p, validating them.
func (in *HostedPanelInput) apply(p *model.HostedPanel, creating bool) error {
	in.Name, in.Username = strings.TrimSpace(in.Name), strings.TrimSpace(in.Username)
	switch {
	case in.Name == "" || utf8.RuneCountInString(in.Name) > hostedNameMaxRunes:
		return invalid("name is required (up to %d characters)", hostedNameMaxRunes)
	case in.Username == "" || utf8.RuneCountInString(in.Username) > hostedUserMaxRunes || strings.ContainsAny(in.Username, " \t\r\n"):
		return invalid("username is required and cannot contain spaces")
	case (creating || in.Username != p.Username) && in.Password == "":
		return invalid("a password is required to set the login")
	case in.Password != "" && len(in.Password) < hostedPasswordMinLen:
		return invalid("password must be at least %d characters", hostedPasswordMinLen)
	case in.MaxInbounds < 0 || in.MaxClients < 0 || in.TrafficBytes < 0 || in.ExpiresAt < 0:
		return invalid("limits cannot be negative")
	}
	path := normalizeHostedBasePath(in.WebBasePath)
	if !hostedBasePathPattern.MatchString(path) {
		return invalid("path may only contain letters, digits and . _ ~ -")
	}
	p.Name, p.Enable, p.Username, p.WebBasePath = in.Name, in.Enable, in.Username, path
	p.WebPort, p.SubPort, p.PortMin, p.PortMax = in.WebPort, in.SubPort, in.PortMin, in.PortMax
	p.MaxInbounds, p.MaxClients, p.TrafficBytes, p.ExpiresAt = in.MaxInbounds, in.MaxClients, in.TrafficBytes, in.ExpiresAt
	return nil
}

func refuseInsideHostedPanel() error {
	if hosted.Enabled() {
		return service.ErrHostedForbidden
	}
	return nil
}

func (s *HostedPanelService) List() ([]HostedPanelView, error) {
	var panels []model.HostedPanel
	if err := database.GetDB().Order("id ASC").Find(&panels).Error; err != nil {
		return nil, err
	}
	views := make([]HostedPanelView, 0, len(panels))
	for _, p := range panels {
		status, _ := hosted.ReadStatus(hostedPanelDir(p.Id))
		views = append(views, HostedPanelView{HostedPanel: p, Status: status, Process: hostedSupervisor.State(p.Id)})
	}
	return views, nil
}

func (s *HostedPanelService) Create(in HostedPanelInput) (*model.HostedPanel, error) {
	if err := refuseInsideHostedPanel(); err != nil {
		return nil, err
	}
	hostedPanelsMu.Lock()
	defer hostedPanelsMu.Unlock()

	p := &model.HostedPanel{CreatedAt: time.Now().UnixMilli()}
	if err := in.apply(p, true); err != nil {
		return nil, err
	}
	db := database.GetDB()
	if err := service.CheckHostedPanelPorts(db, p); err != nil {
		return nil, err
	}
	if err := db.Create(p).Error; err != nil {
		return nil, err
	}
	if p.Id > hostedMaxPanels {
		_ = db.Delete(p).Error
		return nil, invalid("too many hosted panels")
	}
	if err := provisionHostedPanel(p, in.Password); err != nil {
		_ = db.Delete(p).Error
		_ = os.RemoveAll(hostedPanelDir(p.Id))
		return nil, err
	}
	if p.Enable {
		startHostedPanel(p)
	}
	return p, nil
}

func (s *HostedPanelService) Update(id int, in HostedPanelInput) (*model.HostedPanel, error) {
	if err := refuseInsideHostedPanel(); err != nil {
		return nil, err
	}
	hostedPanelsMu.Lock()
	defer hostedPanelsMu.Unlock()

	p, err := loadHostedPanel(id)
	if err != nil {
		return nil, err
	}
	before := *p
	if err := in.apply(p, false); err != nil {
		return nil, err
	}
	db := database.GetDB()
	if err := service.CheckHostedPanelPorts(db, p); err != nil {
		return nil, err
	}
	if err := db.Save(p).Error; err != nil {
		return nil, err
	}
	if err := hosted.WriteLimits(hostedPanelDir(p.Id), hostedLimitsOf(p)); err != nil {
		return nil, err
	}

	moved := before.WebPort != p.WebPort || before.SubPort != p.SubPort || before.WebBasePath != p.WebBasePath
	if moved || !p.Enable {
		hostedSupervisor.Stop(p.Id)
	}
	if moved || in.Password != "" {
		if err := initHostedPanel(p, in.Password); err != nil {
			return nil, err
		}
	}
	if p.Enable {
		startHostedPanel(p)
	}
	return p, nil
}

func (s *HostedPanelService) Delete(id int) error {
	if err := refuseInsideHostedPanel(); err != nil {
		return err
	}
	hostedPanelsMu.Lock()
	defer hostedPanelsMu.Unlock()

	p, err := loadHostedPanel(id)
	if err != nil {
		return err
	}
	hostedSupervisor.Stop(p.Id)
	if err := database.GetDB().Delete(p).Error; err != nil {
		return err
	}
	return os.RemoveAll(hostedPanelDir(p.Id))
}

// ResetUsage starts a new billing period: the panel zeroes its meter on the next poll.
func (s *HostedPanelService) ResetUsage(id int) error {
	if err := refuseInsideHostedPanel(); err != nil {
		return err
	}
	hostedPanelsMu.Lock()
	defer hostedPanelsMu.Unlock()

	p, err := loadHostedPanel(id)
	if err != nil {
		return err
	}
	p.UsageEpoch++
	if err := database.GetDB().Model(p).Update("usage_epoch", p.UsageEpoch).Error; err != nil {
		return err
	}
	return hosted.WriteLimits(hostedPanelDir(p.Id), hostedLimitsOf(p))
}

// StartHostedPanels runs every enabled hosted panel; the master calls it once at boot.
func StartHostedPanels() {
	if hosted.Enabled() {
		return
	}
	var panels []model.HostedPanel
	if err := database.GetDB().Where("enable = ?", true).Find(&panels).Error; err != nil {
		logger.Warning("hosted panels: load:", err)
		return
	}
	for i := range panels {
		p := &panels[i]
		if err := hosted.WriteLimits(hostedPanelDir(p.Id), hostedLimitsOf(p)); err != nil {
			logger.Warning("hosted panel", p.Id, "limits:", err)
			continue
		}
		startHostedPanel(p)
	}
}

func StopHostedPanels() {
	hostedSupervisor.StopAll()
}

func loadHostedPanel(id int) (*model.HostedPanel, error) {
	p := &model.HostedPanel{}
	err := database.GetDB().First(p, id).Error
	if database.IsNotFound(err) {
		return nil, ErrHostedPanelNotFound
	}
	return p, err
}

func provisionHostedPanel(p *model.HostedPanel, password string) error {
	dir := hostedPanelDir(p.Id)
	for _, sub := range []string{"db", "bin", "log"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return err
		}
	}
	if err := hosted.WriteLimits(dir, hostedLimitsOf(p)); err != nil {
		return err
	}
	return initHostedPanel(p, password)
}

// initHostedPanel runs `x-ui hosted-init` in the panel's own environment, so
// the panel's database is written by the same code and migrations it runs on.
func initHostedPanel(p *model.HostedPanel, password string) error {
	exe, err := hostedExecutable()
	if err != nil {
		return err
	}
	account, err := json.Marshal(HostedAccount{
		Username: p.Username, Password: password,
		WebPort: p.WebPort, SubPort: p.SubPort, WebBasePath: p.WebBasePath,
	})
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "hosted-init")
	cmd.Env, cmd.Dir = hostedPanelEnv(p), hostedPanelDir(p.Id)
	cmd.Stdin = bytes.NewReader(account)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("prepare hosted panel: %w: %s", err, lastLine(out))
	}
	return nil
}

func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return lines[len(lines)-1]
}

func startHostedPanel(p *model.HostedPanel) {
	dir := hostedPanelDir(p.Id)
	PublishHostedSponsors()
	if err := syncHostedBin(config.GetBinFolderPath(), filepath.Join(dir, "bin")); err != nil {
		logger.Warning("hosted panel", p.Id, "binaries:", err)
	}
	hostedSupervisor.Start(hosted.Panel{
		ID:      p.Id,
		Dir:     dir,
		Env:     hostedPanelEnv(p),
		LogPath: filepath.Join(dir, "log", "panel.log"),
	})
}

// syncHostedBin gives the panel private copies of the master's binaries and geo
// files: a shared file or symlink would let one panel overwrite another's.
func syncHostedBin(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() || e.Name() == "config.json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(dst, e.Name())
		if have, err := os.Stat(target); err == nil && have.Size() == info.Size() && have.ModTime().Equal(info.ModTime()) {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), target, info); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(from, to string, info os.FileInfo) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(to), "."+filepath.Base(to)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chtimes(tmp.Name(), info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), to)
}
