// Package hosted is the master/child contract for panels hosted on one server:
// the master writes limits.json, the child writes status.json, in one directory.
package hosted

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnvDir is set only by the master when it spawns a child; the child admin
// cannot reach the directory it names through the web UI.
const EnvDir = "XUI_HOSTED_DIR"

const (
	limitsFile = "limits.json"
	statusFile = "status.json"
)

// Blocked reasons a hosted panel stops serving.
const (
	BlockedExpired  = "expired"
	BlockedTraffic  = "traffic"
	BlockedNoLimits = "no-limits"
)

// Limits are the owner-set caps for one hosted panel. Zero means unlimited.
type Limits struct {
	MaxInbounds  int   `json:"maxInbounds"`
	MaxClients   int   `json:"maxClients"`
	TrafficBytes int64 `json:"trafficBytes"`
	ExpiresAt    int64 `json:"expiresAt"`
	PortMin      int   `json:"portMin"`
	PortMax      int   `json:"portMax"`
	// UsageEpoch bumps when the owner resets usage; the child zeroes its counter.
	UsageEpoch int `json:"usageEpoch"`
}

func (l Limits) PortAllowed(port int) bool {
	if l.PortMin <= 0 && l.PortMax <= 0 {
		return true
	}
	return port >= l.PortMin && port <= l.PortMax
}

func (l Limits) Expired(now time.Time) bool {
	return l.ExpiresAt > 0 && now.UnixMilli() >= l.ExpiresAt
}

// Status is what the child reports back for the master's Panels page.
type Status struct {
	Inbounds    int    `json:"inbounds"`
	Clients     int    `json:"clients"`
	UsedBytes   int64  `json:"usedBytes"`
	Blocked     string `json:"blocked"`
	XrayRunning bool   `json:"xrayRunning"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// Dir is this process's hosted directory, or "" for an ordinary panel.
func Dir() string {
	return strings.TrimSpace(os.Getenv(EnvDir))
}

func Enabled() bool {
	return Dir() != ""
}

func ReadLimits(dir string) (Limits, error) {
	var l Limits
	return l, readJSON(filepath.Join(dir, limitsFile), &l)
}

func WriteLimits(dir string, l Limits) error {
	return writeJSON(dir, limitsFile, l)
}

func ReadStatus(dir string) (Status, error) {
	var s Status
	return s, readJSON(filepath.Join(dir, statusFile), &s)
}

func WriteStatus(dir string, s Status) error {
	return writeJSON(dir, statusFile, s)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// writeJSON replaces the file atomically so a reader never sees half a write.
func writeJSON(dir, name string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, name+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}
