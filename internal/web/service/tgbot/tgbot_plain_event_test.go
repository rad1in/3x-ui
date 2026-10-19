package tgbot

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

func TestLoginLocationAlertIsAnEscapedTelegramMessage(t *testing.T) {
	bot := &Tgbot{}
	msg := bot.formatEventMessage(eventbus.Event{
		Type:   eventbus.EventLoginNewLocation,
		Source: "<b>root</b>",
		Data:   &eventbus.NewLoginLocationData{Username: "<b>root</b>", IP: "198.51.100.9"},
	})
	if !strings.Contains(msg, "<b>New login location</b>") {
		t.Fatalf("message lacks the title:\n%s", msg)
	}
	if !strings.Contains(msg, "&lt;b&gt;root&lt;/b&gt; signed in from a new address: 198.51.100.9") {
		t.Fatalf("message does not carry the escaped alert:\n%s", msg)
	}
}
