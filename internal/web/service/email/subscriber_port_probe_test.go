package email

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func TestPortBlockedMailNamesTheInboundAndAddress(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	sub := NewSubscriber(service.SettingService{}, nil)
	subject, body := sub.formatMessage(eventbus.Event{
		Type:   eventbus.EventPortBlocked,
		Source: "<edge>",
		Data:   &eventbus.PortProbeData{Host: "203.0.113.7", Port: 8443, Error: "i/o timeout"},
	})
	if !strings.Contains(subject, "Port 8443 of <edge> stopped answering") {
		t.Fatalf("subject = %q", subject)
	}
	for _, want := range []string{"203.0.113.7:8443", "i/o timeout", "&lt;edge&gt;"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	_, restored := sub.formatMessage(eventbus.Event{
		Type: eventbus.EventPortRestored, Source: "edge",
		Data: &eventbus.PortProbeData{Host: "203.0.113.7", Port: 8443},
	})
	if !strings.Contains(restored, "answers again") || strings.Contains(restored, "i/o timeout") {
		t.Fatalf("restored body:\n%s", restored)
	}
}
