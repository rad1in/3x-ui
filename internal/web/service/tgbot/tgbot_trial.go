package tgbot

import (
	"errors"
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/mymmrac/telego"
)

type trialOutcome int

const (
	trialUnavailable trialOutcome = iota
	trialUsed
	trialFailed
	trialGranted
)

// grantTrial hands out the configured trial once per Telegram id and returns
// the new client's email on success.
func (t *Tgbot) grantTrial(tgID int64) (trialOutcome, string) {
	enabled, err := t.settingService.GetTgTrialEnable()
	if err != nil || !enabled {
		return trialUnavailable, ""
	}
	templateID, err := t.settingService.GetTgTrialTemplateId()
	if err != nil || templateID <= 0 {
		return trialUnavailable, ""
	}
	email, needRestart, err := t.clientService.CreateTrialClient(&t.inboundService, templateID, tgID)
	if needRestart {
		t.xrayService.SetToNeedRestart()
	}
	switch {
	case errors.Is(err, service.ErrTrialAlreadyUsed):
		return trialUsed, ""
	case err != nil:
		logger.Warning("tgbot: trial for", tgID, "failed:", err)
		return trialFailed, ""
	}
	return trialGranted, email
}

// handleTrial answers /trial. A stranger gets nothing while trials are off, so
// the bot does not reveal a feature it is not offering.
func (t *Tgbot) handleTrial(chatId int64, from *telego.User) {
	if !t.allowInviteAttempt(from) {
		return
	}
	outcome, email := t.grantTrial(from.ID)
	switch outcome {
	case trialUnavailable:
		if t.levelOf(from.ID) != levelStranger {
			t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.trialDisabled"))
		}
	case trialUsed:
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.trialUsed"))
	case trialFailed:
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.trialFailed"))
	case trialGranted:
		t.SendMsgToTgbot(chatId, t.trialReadyMessage())
		t.sendClientSubLinks(chatId, email)
	}
}

func (t *Tgbot) trialReadyMessage() string {
	templateID, _ := t.settingService.GetTgTrialTemplateId()
	tpl, err := t.clientService.GetClientTemplate(templateID)
	if err != nil {
		return t.I18nBot("tgbot.messages.trialFailed")
	}
	quota := t.I18nBot("tgbot.unlimited")
	if tpl.TotalGB > 0 {
		quota = common.FormatTraffic(tpl.TotalGB)
	}
	return t.I18nBot("tgbot.messages.trialReady",
		"Hours=="+strconv.Itoa(tpl.ExpiryDays*24),
		"Quota=="+quota)
}
