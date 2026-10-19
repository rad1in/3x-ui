package tgbot

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

func TestResellerCreditAlertIsAnEscapedTelegramMessage(t *testing.T) {
	bot := &Tgbot{}
	msg := bot.formatEventMessage(eventbus.Event{
		Type:   eventbus.EventResellerCreditLow,
		Source: "<b>shop</b>",
		Data:   &eventbus.ResellerCreditData{Username: "<b>shop</b>", Balance: 950, Threshold: 1000},
	})
	if !strings.Contains(msg, "<b>Reseller credit is low</b>") {
		t.Fatalf("message lacks the title:\n%s", msg)
	}
	if !strings.Contains(msg, "Reseller &lt;b&gt;shop&lt;/b&gt; is low on credit: 950 left, alert level 1000") {
		t.Fatalf("message does not carry the escaped alert:\n%s", msg)
	}
}
