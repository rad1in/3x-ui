package discord

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

func TestFormatEmbed_PortBlockedAndRestored(t *testing.T) {
	settingService := setupTestDB(t)
	sub := NewSubscriber(settingService, NewDiscordService(settingService))
	data := &eventbus.PortProbeData{Host: "203.0.113.7", Port: 8443, Error: "i/o timeout"}

	blocked, ok := sub.FormatEmbed(eventbus.Event{Type: eventbus.EventPortBlocked, Source: "edge", Data: data})
	if !ok || blocked.Title != "Port Blocked" || blocked.Color != ColorRed {
		t.Fatalf("blocked embed = %+v ok=%v", blocked, ok)
	}
	values := map[string]string{}
	for _, f := range blocked.Fields {
		values[f.Name] = f.Value
	}
	if values["Inbound"] != "edge" || values["Address"] != "203.0.113.7:8443" || values["Error"] != "i/o timeout" {
		t.Fatalf("blocked fields = %v", values)
	}

	restored, ok := sub.FormatEmbed(eventbus.Event{Type: eventbus.EventPortRestored, Source: "edge",
		Data: &eventbus.PortProbeData{Host: "2001:db8::1", Port: 8443}})
	if !ok || restored.Title != "Port Restored" || restored.Color != ColorGreen {
		t.Fatalf("restored embed = %+v ok=%v", restored, ok)
	}
	for _, f := range restored.Fields {
		if f.Name == "Address" && f.Value != "[2001:db8::1]:8443" {
			t.Errorf("IPv6 address = %q, want it bracketed", f.Value)
		}
		if f.Name == "Error" {
			t.Errorf("restored embed carries an error field: %+v", f)
		}
	}
}
