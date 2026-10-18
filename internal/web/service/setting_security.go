package service

import (
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
)

// SecuritySettingsChanged reports whether a settings save touches what only an
// owner may change: login factors, sessions, and LDAP, which can log anyone in.
func SecuritySettingsChanged(stored, submitted *entity.AllSetting, clearLdapPassword bool) bool {
	secretChanged := func(stored, submitted string) bool {
		submitted = strings.TrimSpace(submitted)
		return submitted != "" && submitted != stored
	}
	return stored.TwoFactorEnable != submitted.TwoFactorEnable ||
		secretChanged(stored.TwoFactorToken, submitted.TwoFactorToken) ||
		stored.SessionMaxAge != submitted.SessionMaxAge ||
		stored.PanelAllowedCidrs != submitted.PanelAllowedCidrs ||
		stored.LdapEnable != submitted.LdapEnable ||
		stored.LdapHost != submitted.LdapHost ||
		stored.LdapPort != submitted.LdapPort ||
		stored.LdapUseTLS != submitted.LdapUseTLS ||
		stored.LdapInsecureSkipVerify != submitted.LdapInsecureSkipVerify ||
		stored.LdapBindDN != submitted.LdapBindDN ||
		secretChanged(stored.LdapPassword, submitted.LdapPassword) ||
		(clearLdapPassword && stored.LdapPassword != "") ||
		stored.LdapBaseDN != submitted.LdapBaseDN ||
		stored.LdapUserFilter != submitted.LdapUserFilter ||
		stored.LdapUserAttr != submitted.LdapUserAttr
}
