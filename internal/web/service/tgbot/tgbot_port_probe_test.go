package tgbot

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

func TestPortProbeEventsAreFormatted(t *testing.T) {
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	_ = bundle.AddMessages(language.MustParse("en-US"),
		&i18n.Message{ID: "tgbot.messages.eventPortBlocked", Other: "{{ .Name }} port {{ .Port }} is blocked"},
		&i18n.Message{ID: "tgbot.messages.eventPortRestored", Other: "{{ .Name }} port {{ .Port }} is back"},
		&i18n.Message{ID: "tgbot.messages.eventErrorDetail", Other: "Error: {{ .Error }}"},
	)
	orig := locale.LocalizerBot
	t.Cleanup(func() { locale.LocalizerBot = orig })
	locale.LocalizerBot = i18n.NewLocalizer(bundle, "en-US")
	tb := &Tgbot{}

	blocked := tb.formatEventMessage(eventbus.Event{Type: eventbus.EventPortBlocked, Source: "edge",
		Data: &eventbus.PortProbeData{Port: 8443, Error: "i/o timeout"}})
	for _, want := range []string{"edge port 8443 is blocked", "Error: i/o timeout"} {
		if !strings.Contains(blocked, want) {
			t.Errorf("blocked message lacks %q:\n%s", want, blocked)
		}
	}
	restored := tb.formatEventMessage(eventbus.Event{Type: eventbus.EventPortRestored, Source: "edge",
		Data: &eventbus.PortProbeData{Port: 8443}})
	if !strings.Contains(restored, "edge port 8443 is back") || strings.Contains(restored, "Error") {
		t.Errorf("restored message = %s", restored)
	}
	if got := tb.formatEventMessage(eventbus.Event{Type: eventbus.EventPortBlocked, Source: "edge"}); got != "" {
		t.Errorf("an event without data produced %q", got)
	}
}
