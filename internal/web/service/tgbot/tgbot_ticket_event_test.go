package tgbot

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

// A customer's ticket text reaches the admins' HTML-mode chat, so markup in it
// must arrive escaped.
func TestTicketEventMessageEscapesCustomerText(t *testing.T) {
	msg := (&Tgbot{}).formatEventMessage(eventbus.Event{
		Type:   eventbus.EventTicketOpened,
		Source: "ticket-3",
		Data: &eventbus.TicketEventData{
			TicketId: 3, Email: "alice", Subject: "<b>urgent</b>", Excerpt: `<a href="http://evil">click</a>`,
		},
	})
	if strings.Contains(msg, "<a href") || strings.Contains(msg, "<b>urgent") {
		t.Fatalf("message carries live markup from the customer: %q", msg)
	}
	for _, want := range []string{"New support ticket", "#3 from alice", "&lt;b&gt;urgent&lt;/b&gt;", "&lt;a href=&#34;http://evil&#34;&gt;click&lt;/a&gt;"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}
}
