package service

import (
	"encoding/json"
	"strings"
)

// SecurityConfig groups the login and data protections that do not depend on
// two-factor codes. It is one JSON setting that only owners may change.
type SecurityConfig struct {
	CaptchaEnable    bool `json:"captchaEnable"`
	NewLocationAlert bool `json:"newLocationAlert"`
}

// SecurityConfigView is the configuration as the admin API serves it.
type SecurityConfigView struct {
	SecurityConfig
}

const securityConfigKey = "securityConfig"

func (s *SecurityConfigService) GetView() (*SecurityConfigView, error) {
	cfg, err := s.Get()
	if err != nil {
		return nil, err
	}
	return &SecurityConfigView{SecurityConfig: *cfg}, nil
}

// SecurityConfigService reads and writes the security configuration.
type SecurityConfigService struct {
	settings SettingService
}

func (s *SecurityConfigService) Get() (*SecurityConfig, error) {
	cfg := &SecurityConfig{}
	raw, err := s.settings.getString(securityConfigKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return cfg, err
	}
	return cfg, json.Unmarshal([]byte(raw), cfg)
}

func (s *SecurityConfigService) Save(cfg *SecurityConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.settings.saveSetting(securityConfigKey, string(raw))
}
