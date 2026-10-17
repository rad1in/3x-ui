package panel

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// HostedAccount is the login and address the master assigns a hosted panel.
type HostedAccount struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	WebPort     int    `json:"webPort"`
	SubPort     int    `json:"subPort"`
	WebBasePath string `json:"webBasePath"`
}

// RunHostedInit applies an account read from r to this process's own database.
// The master runs it as `x-ui hosted-init` so the password never sits in argv.
func RunHostedInit(r io.Reader) error {
	if !hosted.Enabled() {
		return errors.New("hosted-init only runs inside a hosted panel")
	}
	var account HostedAccount
	if err := json.NewDecoder(r).Decode(&account); err != nil {
		return err
	}
	if err := database.InitDB(config.GetDBPath()); err != nil {
		return err
	}
	defer func() { _ = database.CloseDB() }()
	return ApplyHostedAccount(account)
}

// ApplyHostedAccount writes the address, and the login when a password is
// given; a new password also clears 2FA so the owner's reset always works.
func ApplyHostedAccount(a HostedAccount) error {
	settings := &service.SettingService{}
	if err := settings.SetHostedAddress(a.WebPort, a.SubPort, a.WebBasePath); err != nil {
		return err
	}
	if a.Password == "" {
		return nil
	}
	if err := (&UserService{}).UpdateFirstUser(a.Username, a.Password); err != nil {
		return err
	}
	if err := settings.SetTwoFactorEnable(false); err != nil {
		return err
	}
	return settings.SetTwoFactorToken("")
}
