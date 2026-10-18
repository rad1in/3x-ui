package websocket

import (
	"encoding/json"
	"testing"
	"time"
)

func receiveTypes(t *testing.T, c *Client, wait time.Duration) []string {
	t.Helper()
	var got []string
	deadline := time.After(wait)
	for {
		select {
		case raw, ok := <-c.Send:
			if !ok {
				return append(got, "closed")
			}
			var m struct {
				Type    MessageType     `json:"type"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("bad frame: %v", err)
			}
			if m.Type == MessageTypeInvalidate {
				var p map[string]string
				_ = json.Unmarshal(m.Payload, &p)
				got = append(got, "invalidate:"+p["type"])
			} else {
				got = append(got, string(m.Type))
			}
		case <-deadline:
			return got
		}
	}
}

// An operator's socket must never carry another operator's clients or the full
// inbound list: data pushes reach it only as a refetch hint or not at all.
func TestHubKeepsUnscopedDataFromRestrictedClients(t *testing.T) {
	h := NewHub()
	defer h.Stop()
	go h.Run()

	full := NewClient("owner")
	restricted := NewClient("operator")
	restricted.Restricted = true
	h.Register(full)
	h.Register(restricted)
	waitClientCount(t, h, 2)

	h.Broadcast(MessageTypeInbounds, []map[string]string{{"remark": "secret"}})
	h.Broadcast(MessageTypeClientStats, []map[string]string{{"email": "other@x"}})
	h.Broadcast(MessageTypeTraffic, map[string]string{"email": "other@x"})
	h.Broadcast(MessageTypeStatus, map[string]string{"cpu": "1"})

	wantFull := []string{"inbounds", "client_stats", "traffic", "status"}
	wantRestricted := []string{"invalidate:inbounds", "status"}
	if got := receiveTypes(t, full, 200*time.Millisecond); !equalStrings(got, wantFull) {
		t.Errorf("full client got %v, want %v", got, wantFull)
	}
	if got := receiveTypes(t, restricted, 200*time.Millisecond); !equalStrings(got, wantRestricted) {
		t.Errorf("restricted client got %v, want %v", got, wantRestricted)
	}
}

// Deleting or re-scoping an admin must cut that admin's live push stream,
// and only theirs.
func TestHubDisconnectUserClosesOnlyThatUsersSockets(t *testing.T) {
	h := NewHub()
	defer h.Stop()
	go h.Run()

	mine := NewClient("a")
	mine.UserId = 7
	other := NewClient("b")
	other.UserId = 8
	h.Register(mine)
	h.Register(other)
	waitClientCount(t, h, 2)

	h.DisconnectUser(7)
	waitClientCount(t, h, 1)
	if got := receiveTypes(t, mine, 100*time.Millisecond); !equalStrings(got, []string{"closed"}) {
		t.Fatalf("disconnected user's socket got %v, want it closed", got)
	}
	h.Broadcast(MessageTypeStatus, map[string]string{"cpu": "1"})
	if got := receiveTypes(t, other, 200*time.Millisecond); !equalStrings(got, []string{"status"}) {
		t.Fatalf("other user's socket got %v, want it still served", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
