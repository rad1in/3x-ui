package controller

import (
	"encoding/json"
	"errors"
	"html/template"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
	"github.com/mhsanaei/3x-ui/v3/internal/util/totp"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/tgbot"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

// loginPasskeys holds the open passkey ceremonies shared by the login and
// the account-settings controllers.
var loginPasskeys = service.NewPasskeyService()

// passkeyBeginLimiter caps unauthenticated ceremony starts per address so
// they cannot fill the in-memory ceremony store.
var passkeyBeginLimiter = newLoginLimiter(30, time.Minute, 5*time.Minute)

// passkeyRelyingParty derives the WebAuthn origin and RP ID from the address
// the browser used; a trusted proxy may supply the public host and scheme.
func passkeyRelyingParty(c *gin.Context) (origin, rpID string) {
	host := c.Request.Host
	https := c.Request.TLS != nil
	if isTrustedForwardedRequest(c) {
		if h, _, _ := strings.Cut(c.GetHeader("X-Forwarded-Host"), ","); strings.TrimSpace(h) != "" {
			host = strings.TrimSpace(h)
		}
		if strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
			https = true
		}
	}
	scheme := "http"
	if https {
		scheme = "https"
	}
	name, port, err := net.SplitHostPort(host)
	if err != nil {
		name, port = strings.Trim(host, "[]"), ""
	}
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	authority := host
	if port == "" {
		authority = name
		if strings.Contains(name, ":") {
			authority = "[" + name + "]"
		}
	}
	return scheme + "://" + authority, name
}

// passkeyLoginStatus tells the login page whether to offer the passkey button.
func (a *IndexController) passkeyLoginStatus(c *gin.Context) {
	cfg, err := a.security.Get()
	jsonObj(c, err == nil && cfg.PasskeyLoginEnable, nil)
}

func (a *IndexController) passkeyLoginBegin(c *gin.Context) {
	cfg, err := a.security.Get()
	if err != nil || !cfg.PasskeyLoginEnable {
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.passkeyDisabled"))
		return
	}
	remoteIP := getRemoteIp(c)
	if _, ok := passkeyBeginLimiter.allow(remoteIP, ""); !ok {
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.passkeyFailed"))
		return
	}
	passkeyBeginLimiter.registerFailure(remoteIP, "")
	origin, rpID := passkeyRelyingParty(c)
	opts, cid, err := a.passkeys.BeginLogin(0, true, origin, rpID)
	if err != nil {
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.passkeyFailed"))
		return
	}
	c.JSON(http.StatusOK, entity.Msg{Success: true, Obj: gin.H{"options": opts, "challengeId": cid}})
}

type passkeyFinishForm struct {
	ChallengeId   string                        `json:"challengeId"`
	Credential    service.PasskeyCredentialJSON `json:"credential"`
	TwoFactorCode string                        `json:"twoFactorCode"`
}

// passkeyLoginFinish signs in with a discoverable passkey. The authenticator
// must verify the user (PIN or biometric), so the passkey stands for password
// plus a second factor; the panel authenticator code is still required when set.
func (a *IndexController) passkeyLoginFinish(c *gin.Context) {
	var form passkeyFinishForm
	if err := c.ShouldBindJSON(&form); err != nil {
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.invalidFormData"))
		return
	}
	cfg, err := a.security.Get()
	if err != nil || !cfg.PasskeyLoginEnable {
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.passkeyDisabled"))
		return
	}
	remoteIP := getRemoteIp(c)
	timeStr := time.Now().Format("2006-01-02 15:04:05")
	a.applyLockoutSettings()
	if blockedUntil, ok := defaultLoginLimiter.allow(remoteIP, ""); !ok {
		logger.Warningf("failed passkey login: IP=%q, reason=%q, blocked_until=%s", remoteIP, "too many failed attempts", blockedUntil.Format(time.RFC3339))
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.passkeyFailed"))
		return
	}
	fail := func(username, reason, msgKey string) {
		if blockedUntil, blocked := defaultLoginLimiter.registerFailure(remoteIP, ""); blocked {
			logger.Warningf("failed passkey login: IP=%q, reason=%q, blocked_until=%s", remoteIP, reason, blockedUntil.Format(time.RFC3339))
		} else {
			logger.Warningf("failed passkey login: IP=%q, reason=%q", remoteIP, reason)
		}
		a.tgbot.UserLoginNotify(tgbot.LoginAttempt{Username: username, IP: remoteIP, Time: timeStr, Status: tgbot.LoginFail, Reason: reason})
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, msgKey))
	}

	user, err := a.passkeys.FinishLogin(form.ChallengeId, form.Credential)
	if err != nil {
		fail("passkey", "invalid passkey", "pages.login.toasts.passkeyFailed")
		return
	}
	safeUser := template.HTMLEscapeString(user.Username)
	if user.Role == model.AdminRoleOwner {
		if on, _ := a.settingService.GetTwoFactorEnable(); on {
			token, _ := a.settingService.GetTwoFactorToken()
			if !totp.VerifyWithSkew(token, form.TwoFactorCode, time.Now()) {
				fail(safeUser, "invalid 2FA code", "pages.login.toasts.passkeyNeedsTwoFactor")
				return
			}
		}
	}
	if on, message := a.settingService.GetMaintenance(); on && user.Role != model.AdminRoleOwner {
		if message == "" {
			message = I18nWeb(c, "pages.login.toasts.maintenance")
		}
		pureJsonMsg(c, http.StatusOK, false, message)
		return
	}
	a.establishSession(c, user, "", remoteIP, safeUser, timeStr)
}

// passPasskeyStep asks an admin who has passkeys for one after the password
// when the owner turned the second-factor option on.
func (a *IndexController) passPasskeyStep(c *gin.Context, user *model.User, form LoginForm, remoteIP string) bool {
	cfg, err := a.security.Get()
	if err != nil || !cfg.PasskeySecondFactor {
		return true
	}
	has, err := a.passkeys.HasAny(user.Id)
	if err != nil || !has {
		return err == nil
	}
	origin, rpID := passkeyRelyingParty(c)
	if form.PasskeyChallengeId == "" {
		opts, cid, err := a.passkeys.BeginLogin(user.Id, false, origin, rpID)
		if err != nil {
			pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.passkeyFailed"))
			return false
		}
		c.JSON(http.StatusOK, entity.Msg{Msg: I18nWeb(c, "pages.login.toasts.passkeyRequired"),
			Obj: gin.H{"passkeyRequired": true, "passkeyOptions": opts, "passkeyChallengeId": cid}})
		return false
	}
	var cred service.PasskeyCredentialJSON
	var got *model.User
	err = json.Unmarshal([]byte(form.PasskeyCredential), &cred)
	if err == nil {
		got, err = a.passkeys.FinishLogin(form.PasskeyChallengeId, cred)
	}
	if err != nil || got.Id != user.Id {
		if _, blocked := defaultLoginLimiter.registerFailure(remoteIP, user.Username); blocked {
			logger.Warningf("failed login: username=%q, IP=%q, reason=%q", user.Username, remoteIP, "invalid passkey")
		}
		pureJsonMsg(c, http.StatusOK, false, I18nWeb(c, "pages.login.toasts.passkeyFailed"))
		return false
	}
	return true
}

// PasskeyController lets an admin register, list and remove their own passkeys.
type PasskeyController struct {
	passkeys *service.PasskeyService
}

func NewPasskeyController(g *gin.RouterGroup) *PasskeyController {
	a := &PasskeyController{passkeys: loginPasskeys}
	g.GET("/list", a.list)
	g.POST("/register/begin", a.registerBegin)
	g.POST("/register/finish", a.registerFinish)
	g.POST("/del/:id", a.del)
	return a
}

func (a *PasskeyController) list(c *gin.Context) {
	rows, err := a.passkeys.List(session.GetLoginUser(c).Id)
	jsonObj(c, rows, err)
}

type passkeyRegisterBeginForm struct {
	Password string `json:"password"`
}

// registerBegin asks for the account password again, so a hijacked session
// cannot plant a passkey that outlives it.
func (a *PasskeyController) registerBegin(c *gin.Context) {
	user := session.GetLoginUser(c)
	var form passkeyRegisterBeginForm
	if err := c.ShouldBindJSON(&form); err != nil || !crypto.CheckPasswordHash(user.Password, form.Password) {
		jsonMsg(c, I18nWeb(c, "pages.settings.security.passkeys.wrongPassword"), errors.New("wrong password"))
		return
	}
	origin, rpID := passkeyRelyingParty(c)
	opts, cid, err := a.passkeys.BeginRegistration(user, origin, rpID)
	jsonObj(c, gin.H{"options": opts, "challengeId": cid}, err)
}

type passkeyRegisterFinishForm struct {
	ChallengeId string                        `json:"challengeId"`
	Name        string                        `json:"name"`
	Credential  service.PasskeyCredentialJSON `json:"credential"`
}

func (a *PasskeyController) registerFinish(c *gin.Context) {
	var form passkeyRegisterFinishForm
	if err := c.ShouldBindJSON(&form); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	row, err := a.passkeys.FinishRegistration(session.GetLoginUser(c), form.ChallengeId, strings.TrimSpace(form.Name), form.Credential)
	jsonMsgObj(c, I18nWeb(c, "pages.settings.security.passkeys.added"), row, err)
}

func (a *PasskeyController) del(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "pages.settings.security.passkeys.removed"), a.passkeys.Delete(session.GetLoginUser(c).Id, id))
}
