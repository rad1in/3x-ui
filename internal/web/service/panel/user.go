package panel

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
	ldaputil "github.com/mhsanaei/3x-ui/v3/internal/util/ldap"
	"github.com/mhsanaei/3x-ui/v3/internal/util/totp"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// UserService provides business logic for user management and authentication.
// It handles user creation, login, password management, and 2FA operations.
type UserService struct {
	settingService service.SettingService
}

// GetPrimaryOwner returns the oldest owner account: the one API tokens act as
// and the CLI resets.
func (s *UserService) GetPrimaryOwner() (*model.User, error) {
	db := database.GetDB()

	user := &model.User{}
	err := db.Model(model.User{}).
		Where("role = ?", model.AdminRoleOwner).
		Order("id ASC").
		First(user).
		Error
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) CheckUser(username string, password string, twoFactorCode string) (*model.User, error) {
	db := database.GetDB()

	user := &model.User{}

	err := db.Model(model.User{}).
		Where("username = ?", username).
		First(user).
		Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("invalid credentials")
	} else if err != nil {
		logger.Warning("check user err:", err)
		return nil, err
	}

	if !crypto.CheckPasswordHash(user.Password, password) {
		ldapEnabled, _ := s.settingService.GetLdapEnable()
		if !ldapEnabled {
			return nil, errors.New("invalid credentials")
		}

		host, _ := s.settingService.GetLdapHost()
		port, _ := s.settingService.GetLdapPort()
		useTLS, _ := s.settingService.GetLdapUseTLS()
		skipVerify, _ := s.settingService.GetLdapInsecureSkipVerify()
		bindDN, _ := s.settingService.GetLdapBindDN()
		ldapPass, _ := s.settingService.GetLdapPassword()
		baseDN, _ := s.settingService.GetLdapBaseDN()
		userFilter, _ := s.settingService.GetLdapUserFilter()
		userAttr, _ := s.settingService.GetLdapUserAttr()

		cfg := ldaputil.Config{
			Host:               host,
			Port:               port,
			UseTLS:             useTLS,
			InsecureSkipVerify: skipVerify,
			BindDN:             bindDN,
			Password:           ldapPass,
			BaseDN:             baseDN,
			UserFilter:         userFilter,
			UserAttr:           userAttr,
		}
		ok, err := ldaputil.AuthenticateUser(cfg, username, password)
		if err != nil || !ok {
			return nil, errors.New("invalid credentials")
		}
	}

	twoFactorEnable, err := s.settingService.GetTwoFactorEnable()
	if err != nil {
		logger.Warning("check two factor err:", err)
		return nil, err
	}

	// The panel has one authenticator secret, set up by and bound to its owners.
	if twoFactorEnable && user.Role == model.AdminRoleOwner {
		twoFactorToken, err := s.settingService.GetTwoFactorToken()
		if err != nil {
			logger.Warning("check two factor token err:", err)
			return nil, err
		}

		if !totp.VerifyWithSkew(twoFactorToken, twoFactorCode, time.Now()) {
			return nil, errors.New("invalid 2fa code")
		}
	}

	return user, nil
}

func (s *UserService) BumpLoginEpoch() error {
	db := database.GetDB()
	return db.Model(model.User{}).
		Where("1 = 1").
		Update("login_epoch", gorm.Expr("login_epoch + 1")).
		Error
}

func (s *UserService) UpdateUser(id int, username string, password string) error {
	db := database.GetDB()
	hashedPassword, err := crypto.HashPasswordAsBcrypt(password)
	if err != nil {
		return err
	}

	twoFactorEnable, err := s.settingService.GetTwoFactorEnable()
	if err != nil {
		return err
	}
	var role string
	if err := db.Model(model.User{}).Where("id = ?", id).Pluck("role", &role).Error; err != nil {
		return err
	}

	// Only an owner's credentials are bound to the panel authenticator.
	if twoFactorEnable && role == model.AdminRoleOwner {
		_ = s.settingService.SetTwoFactorEnable(false)
		_ = s.settingService.SetTwoFactorToken("")
	}

	return db.Model(model.User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"username":    username,
			"password":    hashedPassword,
			"login_epoch": gorm.Expr("login_epoch + 1"),
		}).
		Error
}

func (s *UserService) UpdateFirstUser(username string, password string) error {
	if username == "" {
		return errors.New("username can not be empty")
	} else if password == "" {
		return errors.New("password can not be empty")
	}
	hashedPassword, er := crypto.HashPasswordAsBcrypt(password)

	if er != nil {
		return er
	}

	db := database.GetDB()
	user, err := s.GetPrimaryOwner()
	if database.IsNotFound(err) {
		user = &model.User{Role: model.AdminRoleOwner}
		user.Username = username
		user.Password = hashedPassword
		return db.Model(model.User{}).Create(user).Error
	} else if err != nil {
		return err
	}
	user.Username = username
	user.Password = hashedPassword
	user.LoginEpoch++
	return db.Save(user).Error
}
