package tgbot

import (
	"strconv"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/mymmrac/telego"
)

func setSetting(t *testing.T, key, value string) {
	t.Helper()
	row := model.Setting{Key: key}
	if err := database.GetDB().Where("key = ?", key).Assign(model.Setting{Value: value}).FirstOrCreate(&row).Error; err != nil {
		t.Fatalf("set %s: %v", key, err)
	}
}

func enableTrial(t *testing.T) {
	t.Helper()
	var inbound model.Inbound
	if err := database.GetDB().First(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	tpl, err := (&service.ClientService{}).SaveClientTemplate(&model.ClientTemplate{
		Name: "trial", TotalGB: 1 << 30, ExpiryDays: 1, InboundIds: []int{inbound.Id},
	})
	if err != nil {
		t.Fatal(err)
	}
	setSetting(t, "tgTrialEnable", "true")
	setSetting(t, "tgTrialTemplateId", strconv.Itoa(tpl.Id))
}

func TestGrantTrialOffUntilEnabledThenOncePerTelegramID(t *testing.T) {
	tb, _ := newLinksCallbackTgbot(t, ownerMail)

	if outcome, _ := tb.grantTrial(880001); outcome != trialUnavailable {
		t.Fatalf("outcome with trials off = %d, want trialUnavailable", outcome)
	}
	enableTrial(t)

	outcome, email := tb.grantTrial(880001)
	if outcome != trialGranted || email != "trial-880001" {
		t.Fatalf("first /trial = %d %q, want granted trial-880001", outcome, email)
	}
	var rec model.ClientRecord
	if err := database.GetDB().Where("email = ?", email).First(&rec).Error; err != nil || rec.TgID != 880001 {
		t.Fatalf("trial client = %+v, %v; want bound to telegram 880001", rec, err)
	}
	if outcome, _ := tb.grantTrial(880001); outcome != trialUsed {
		t.Fatalf("second /trial = %d, want trialUsed", outcome)
	}
}

func TestHandleTrialStaysSilentForStrangersWhileOff(t *testing.T) {
	tb, calls := newLinksCallbackTgbot(t, ownerMail)
	tb.handleTrial(880002, &telego.User{ID: 880002})
	if n := calls("sendMessage"); n != 0 {
		t.Fatalf("stranger got %d replies while trials are off, want 0", n)
	}
	seedClientRecord(t, "bound@x", "sub-bound", 4242)
	tb.handleTrial(4242, &telego.User{ID: 4242})
	if n := calls("sendMessage"); n != 1 {
		t.Fatalf("a bound client got %d replies, want the one 'not available' notice", n)
	}
}
