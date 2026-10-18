package service

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// Foreign panel sources whose user exports can be imported.
const (
	ForeignMarzban    = "marzban"
	ForeignMarzneshin = "marzneshin"
	ForeignHiddify    = "hiddify"
)

// ForeignImportWarning names a user whose data could not be carried over as is.
type ForeignImportWarning struct {
	Email   string `json:"email"`
	Message string `json:"message"`
}

// ForeignImportResult is BulkCreateResult plus what was approximated.
type ForeignImportResult struct {
	BulkCreateResult
	Warnings []ForeignImportWarning `json:"warnings,omitempty"`
}

const bytesPerGiB = float64(1 << 30)

func millisPerSecond(s float64) int64 { return int64(math.Round(s * 1000)) }

// parseForeignTime reads the timestamp shapes these panels emit (ISO with or
// without zone or fraction, plain dates); a missing zone means UTC.
func parseForeignTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// foreignResetCycle maps a foreign reset strategy to a 3x-ui cycle; year has no
// equivalent and is reported by the caller.
func foreignResetCycle(strategy string) (cycle string, supported bool) {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "", "no_reset", "never":
		return "never", true
	case "day", "daily":
		return "daily", true
	case "week", "weekly":
		return "weekly", true
	case "month", "monthly":
		return "monthly", true
	default:
		return "never", false
	}
}

func newForeignClient(email string) model.Client {
	return model.Client{
		Email:           email,
		Enable:          true,
		SubID:           uuid.NewString(),
		TrafficReset:    "never",
		TrafficResetDay: 1,
	}
}

func portable(usage float64) *ClientPortableTraffic {
	if usage <= 0 {
		return nil
	}
	return &ClientPortableTraffic{Down: int64(usage)}
}

// ConvertForeignUsers turns a user export of another panel into payloads that
// ImportClients accepts, attaching every user to inboundIds.
func ConvertForeignUsers(source string, data []byte, inboundIds []int) ([]ClientCreatePayload, []ForeignImportWarning, error) {
	if len(inboundIds) == 0 {
		return nil, nil, common.NewError("at least one inbound is required")
	}
	var (
		items    []ClientCreatePayload
		warnings []ForeignImportWarning
		err      error
	)
	switch source {
	case ForeignMarzban:
		items, warnings, err = convertMarzban(data)
	case ForeignMarzneshin:
		items, warnings, err = convertMarzneshin(data)
	case ForeignHiddify:
		items, warnings, err = convertHiddify(data)
	default:
		return nil, nil, common.NewError("unknown import source:", source)
	}
	if err != nil {
		return nil, nil, err
	}
	for i := range items {
		items[i].InboundIds = append([]int(nil), inboundIds...)
	}
	return items, warnings, nil
}

// ImportForeignClients converts a foreign export and creates the clients;
// existing emails are skipped, like any import.
func (s *ClientService) ImportForeignClients(inboundSvc *InboundService, source string, data []byte, inboundIds []int) (ForeignImportResult, bool, error) {
	items, warnings, err := ConvertForeignUsers(source, data, inboundIds)
	if err != nil {
		return ForeignImportResult{}, false, err
	}
	res, needRestart, err := s.ImportClients(inboundSvc, items)
	return ForeignImportResult{BulkCreateResult: res, Warnings: warnings}, needRestart, err
}

// ---- Marzban: GET /api/users ----

type marzbanUser struct {
	Username             string                  `json:"username"`
	Status               string                  `json:"status"`
	Expire               float64                 `json:"expire"`
	DataLimit            float64                 `json:"data_limit"`
	DataLimitResetStrat  string                  `json:"data_limit_reset_strategy"`
	UsedTraffic          float64                 `json:"used_traffic"`
	Note                 string                  `json:"note"`
	OnHoldExpireDuration float64                 `json:"on_hold_expire_duration"`
	Proxies              map[string]marzbanProxy `json:"proxies"`
}

type marzbanProxy struct {
	ID       string `json:"id"`
	Password string `json:"password"`
	Flow     string `json:"flow"`
}

func convertMarzban(data []byte) ([]ClientCreatePayload, []ForeignImportWarning, error) {
	var doc struct {
		Users []marzbanUser `json:"users"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		var bare []marzbanUser
		if err2 := json.Unmarshal(data, &bare); err2 != nil {
			return nil, nil, common.NewError("not a Marzban users export:", err.Error())
		}
		doc.Users = bare
	}
	if len(doc.Users) == 0 {
		return nil, nil, common.NewError("the Marzban export contains no users")
	}
	var (
		items    []ClientCreatePayload
		warnings []ForeignImportWarning
	)
	for _, u := range doc.Users {
		email := strings.TrimSpace(u.Username)
		if email == "" {
			continue
		}
		c := newForeignClient(email)
		warn := func(msg string) { warnings = append(warnings, ForeignImportWarning{Email: email, Message: msg}) }

		vless, vmess := u.Proxies["vless"], u.Proxies["vmess"]
		switch {
		case vless.ID != "":
			c.ID, c.Flow = vless.ID, vless.Flow
			if vmess.ID != "" && vmess.ID != vless.ID {
				warn("vmess id differs from the vless id; 3x-ui keeps one UUID, so the vmess config changes")
			}
		case vmess.ID != "":
			c.ID = vmess.ID
		}
		trojan, ss := u.Proxies["trojan"], u.Proxies["shadowsocks"]
		switch {
		case trojan.Password != "":
			c.Password = trojan.Password
			if ss.Password != "" && ss.Password != trojan.Password {
				warn("shadowsocks password differs from the trojan password; 3x-ui keeps one, so the shadowsocks config changes")
			}
		case ss.Password != "":
			c.Password = ss.Password
		}

		c.Comment = u.Note
		if u.DataLimit > 0 {
			c.TotalGB = int64(u.DataLimit)
		}
		switch {
		case u.Status == "on_hold" && u.OnHoldExpireDuration > 0:
			c.ExpiryTime = -millisPerSecond(u.OnHoldExpireDuration)
		case u.Expire > 0:
			c.ExpiryTime = millisPerSecond(u.Expire)
		}
		c.Enable = u.Status == "active" || u.Status == "on_hold"
		cycle, ok := foreignResetCycle(u.DataLimitResetStrat)
		c.TrafficReset = cycle
		if !ok {
			warn("reset strategy " + u.DataLimitResetStrat + " has no 3x-ui equivalent; the quota is not reset automatically")
		}
		items = append(items, ClientCreatePayload{Client: c, Traffic: portable(u.UsedTraffic)})
	}
	return items, warnings, nil
}

// ---- Marzneshin: GET /api/users (paginated "items") ----

type marzneshinUser struct {
	Username       string  `json:"username"`
	Key            string  `json:"key"`
	ExpireStrategy string  `json:"expire_strategy"`
	ExpireDate     string  `json:"expire_date"`
	UsageDuration  float64 `json:"usage_duration"`
	DataLimit      float64 `json:"data_limit"`
	ResetStrategy  string  `json:"data_limit_reset_strategy"`
	UsedTraffic    float64 `json:"used_traffic"`
	Enabled        *bool   `json:"enabled"`
	Note           string  `json:"note"`
}

// marzneshinUUID formats the 32 hex character user key the way Marzneshin
// builds the VLESS/VMess id from it.
func marzneshinUUID(key string) (string, bool) {
	id, err := uuid.Parse(key)
	if err != nil {
		return "", false
	}
	return id.String(), true
}

func convertMarzneshin(data []byte) ([]ClientCreatePayload, []ForeignImportWarning, error) {
	var doc struct {
		Items []marzneshinUser `json:"items"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		var bare []marzneshinUser
		if err2 := json.Unmarshal(data, &bare); err2 != nil {
			return nil, nil, common.NewError("not a Marzneshin users export:", err.Error())
		}
		doc.Items = bare
	}
	if len(doc.Items) == 0 {
		return nil, nil, common.NewError("the Marzneshin export contains no users")
	}
	var (
		items    []ClientCreatePayload
		warnings []ForeignImportWarning
	)
	for _, u := range doc.Items {
		email := strings.TrimSpace(u.Username)
		if email == "" {
			continue
		}
		c := newForeignClient(email)
		warn := func(msg string) { warnings = append(warnings, ForeignImportWarning{Email: email, Message: msg}) }

		if id, ok := marzneshinUUID(u.Key); ok {
			c.ID = id
			c.Password = strings.ToLower(strings.ReplaceAll(u.Key, "-", ""))
		} else {
			warn("user key is not a 32 character hex string; credentials were generated")
		}
		c.Comment = u.Note
		if u.DataLimit > 0 {
			c.TotalGB = int64(u.DataLimit)
		}
		switch u.ExpireStrategy {
		case "fixed_date":
			if t, ok := parseForeignTime(u.ExpireDate); ok {
				c.ExpiryTime = t.UnixMilli()
			}
		case "start_on_first_use":
			if u.UsageDuration > 0 {
				c.ExpiryTime = -millisPerSecond(u.UsageDuration)
			}
		}
		if u.Enabled != nil {
			c.Enable = *u.Enabled
		}
		cycle, ok := foreignResetCycle(u.ResetStrategy)
		c.TrafficReset = cycle
		if !ok {
			warn("reset strategy " + u.ResetStrategy + " has no 3x-ui equivalent; the quota is not reset automatically")
		}
		items = append(items, ClientCreatePayload{Client: c, Traffic: portable(u.UsedTraffic)})
	}
	return items, warnings, nil
}

// ---- Hiddify-Manager: admin users list ----

type hiddifyUser struct {
	UUID         string   `json:"uuid"`
	Name         string   `json:"name"`
	UsageLimitGB float64  `json:"usage_limit_GB"`
	PackageDays  float64  `json:"package_days"`
	Mode         string   `json:"mode"`
	StartDate    string   `json:"start_date"`
	CurrentGB    float64  `json:"current_usage_GB"`
	Comment      string   `json:"comment"`
	TelegramID   *float64 `json:"telegram_id"`
	MaxIPs       float64  `json:"max_ips"`
	Enable       *bool    `json:"enable"`
}

// hiddifyUnlimitedIPs is Hiddify's placeholder cap, not a real device limit.
const hiddifyUnlimitedIPs = 100

func convertHiddify(data []byte) ([]ClientCreatePayload, []ForeignImportWarning, error) {
	var users []hiddifyUser
	if err := json.Unmarshal(data, &users); err != nil {
		var doc struct {
			Users []hiddifyUser `json:"users"`
		}
		if err2 := json.Unmarshal(data, &doc); err2 != nil || len(doc.Users) == 0 {
			return nil, nil, common.NewError("not a Hiddify users export:", err.Error())
		}
		users = doc.Users
	}
	if len(users) == 0 {
		return nil, nil, common.NewError("the Hiddify export contains no users")
	}
	var (
		items    []ClientCreatePayload
		warnings []ForeignImportWarning
	)
	for _, u := range users {
		email := strings.TrimSpace(u.Name)
		if email == "" {
			email = strings.TrimSpace(u.UUID)
		}
		id, err := uuid.Parse(strings.TrimSpace(u.UUID))
		if email == "" || err != nil {
			warnings = append(warnings, ForeignImportWarning{Email: email, Message: "user has no valid uuid and was skipped"})
			continue
		}
		c := newForeignClient(email)
		c.ID, c.Password, c.Auth = id.String(), id.String(), id.String()
		c.Comment = u.Comment
		if u.UsageLimitGB > 0 {
			c.TotalGB = int64(math.Round(u.UsageLimitGB * bytesPerGiB))
		}
		if u.TelegramID != nil {
			c.TgID = int64(*u.TelegramID)
		}
		if u.MaxIPs >= 1 && u.MaxIPs < hiddifyUnlimitedIPs {
			c.LimitIP = int(u.MaxIPs)
		}
		if u.PackageDays > 0 {
			span := int64(u.PackageDays) * millisPerDay
			if start, ok := parseForeignTime(u.StartDate); ok {
				c.ExpiryTime = start.UnixMilli() + span
			} else {
				c.ExpiryTime = -span
			}
		}
		if u.Enable != nil {
			c.Enable = *u.Enable
		}
		cycle, ok := foreignResetCycle(u.Mode)
		c.TrafficReset = cycle
		if !ok {
			warnings = append(warnings, ForeignImportWarning{Email: email, Message: fmt.Sprintf("mode %s has no 3x-ui equivalent; the quota is not reset automatically", u.Mode)})
		}
		items = append(items, ClientCreatePayload{Client: c, Traffic: portable(u.CurrentGB * bytesPerGiB)})
	}
	return items, warnings, nil
}
