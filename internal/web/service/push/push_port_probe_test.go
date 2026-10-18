package push

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

func TestPortProbeEventsAreFormattedForPush(t *testing.T) {
	msg, ok := Format(eventbus.Event{Type: eventbus.EventPortBlocked, Source: "edge", Data: &eventbus.PortProbeData{
		Host: "203.0.113.7", Port: 8443, Error: "i/o timeout",
	}}, "h")
	if !ok || msg.Title != "[h] Port blocked" || msg.Priority != 4 {
		t.Fatalf("blocked = %+v ok=%v", msg, ok)
	}
	for _, want := range []string{"edge", "203.0.113.7:8443", "i/o timeout"} {
		if !strings.Contains(msg.Body, want) {
			t.Errorf("body lacks %q:\n%s", want, msg.Body)
		}
	}
	back, ok := Format(eventbus.Event{Type: eventbus.EventPortRestored, Source: "edge", Data: &eventbus.PortProbeData{Host: "h", Port: 1}}, "h")
	if !ok || back.Title != "[h] Port restored" || back.Priority != 3 {
		t.Fatalf("restored = %+v ok=%v", back, ok)
	}
}
