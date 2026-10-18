package service

import (
	"encoding/json"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// CardPayConfig is the destination card for manual card-to-card payments.
type CardPayConfig struct {
	Enabled      bool   `json:"enabled"`
	CardNumber   string `json:"cardNumber" example:"6037991812345678"`
	HolderName   string `json:"holderName" example:"Ali Rezaei"`
	BankName     string `json:"bankName" example:"Melli"`
	Instructions string `json:"instructions"`
}

// SalesConfig is everything the operator configures about selling. It is one
// JSON setting; later payment methods add their own sections.
type SalesConfig struct {
	PublicEnabled bool          `json:"publicEnabled"`
	PublicBaseURL string        `json:"publicBaseUrl" example:"https://shop.example.com"`
	Card          CardPayConfig `json:"card"`
}

const salesConfigKey = "salesConfig"

// SalesConfigService reads and writes the sales configuration.
type SalesConfigService struct {
	settings SettingService
}

func (s *SalesConfigService) Get() (*SalesConfig, error) {
	raw, err := s.settings.getString(salesConfigKey)
	if err != nil {
		return nil, err
	}
	cfg := &SalesConfig{}
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save validates and stores cfg; only an enabled section is validated, so a
// half-filled form can be parked disabled.
func (s *SalesConfigService) Save(cfg *SalesConfig) error {
	if cfg == nil {
		return common.NewError("empty sales config")
	}
	cfg.PublicBaseURL = strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/")
	card := &cfg.Card
	card.HolderName = strings.TrimSpace(card.HolderName)
	card.BankName = strings.TrimSpace(card.BankName)
	card.Instructions = strings.TrimSpace(card.Instructions)
	card.CardNumber = digitsOnly(card.CardNumber)
	if card.Enabled && len(card.CardNumber) != 16 {
		return common.NewError("card number must be 16 digits")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.settings.saveSetting(salesConfigKey, string(raw))
}

// digitsOnly keeps the digits of s, folding Persian and Arabic-Indic digits
// to ASCII so a card number pasted from a Persian keyboard still validates.
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= '\u06F0' && r <= '\u06F9':
			b.WriteRune('0' + r - '\u06F0')
		case r >= '\u0660' && r <= '\u0669':
			b.WriteRune('0' + r - '\u0660')
		}
	}
	return b.String()
}
