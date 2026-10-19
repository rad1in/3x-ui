package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

func setupAuditDB(t *testing.T) *AuditService {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	return &AuditService{}
}

func recordAll(t *testing.T, s *AuditService, rows ...model.AuditLog) {
	t.Helper()
	for _, r := range rows {
		if err := s.Record(r); err != nil {
			t.Fatalf("record %+v: %v", r, err)
		}
	}
}

func actions(page *AuditLogPage) []string {
	out := make([]string, 0, len(page.Items))
	for _, it := range page.Items {
		out = append(out, it.Action)
	}
	return out
}

func TestAuditListNewestFirstWithSearchAndResultFilter(t *testing.T) {
	s := setupAuditDB(t)
	recordAll(t, s,
		model.AuditLog{CreatedAt: 1000, Actor: "admin", Ip: "203.0.113.5", Action: "POST /clients/add", Success: true},
		model.AuditLog{CreatedAt: 2000, Actor: "admin", Ip: "203.0.113.5", Action: "POST /clients/del/:email", Target: "email=bob@x", Success: false},
		model.AuditLog{CreatedAt: 3000, Actor: "ops-bot", Ip: "198.51.100.7", Action: "POST /inbounds/add", Success: true},
	)

	cases := []struct {
		name      string
		params    AuditListParams
		wantTotal int64
		want      []string
	}{
		{"all, newest first", AuditListParams{}, 3, []string{"POST /inbounds/add", "POST /clients/del/:email", "POST /clients/add"}},
		{"search matches target", AuditListParams{Search: "bob@x"}, 1, []string{"POST /clients/del/:email"}},
		{"search matches actor", AuditListParams{Search: "ops-bot"}, 1, []string{"POST /inbounds/add"}},
		{"search matches ip", AuditListParams{Search: "203.0.113"}, 2, []string{"POST /clients/del/:email", "POST /clients/add"}},
		{"failures only", AuditListParams{Result: "fail"}, 1, []string{"POST /clients/del/:email"}},
		{"successes only", AuditListParams{Result: "ok"}, 2, []string{"POST /inbounds/add", "POST /clients/add"}},
		{"second page", AuditListParams{Page: 2, PageSize: 2}, 3, []string{"POST /clients/add"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := s.List(tc.params)
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != tc.wantTotal {
				t.Errorf("total = %d, want %d", page.Total, tc.wantTotal)
			}
			if got := actions(page); !equalStrings(got, tc.want) {
				t.Errorf("actions = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAuditSearchTreatsLikeWildcardsLiterally(t *testing.T) {
	s := setupAuditDB(t)
	recordAll(t, s,
		model.AuditLog{CreatedAt: 1, Actor: "admin", Action: "POST /clients/add", Target: "email=a_b@x", Success: true},
		model.AuditLog{CreatedAt: 2, Actor: "admin", Action: "POST /clients/add", Target: "email=axb@x", Success: true},
	)
	page, err := s.List(AuditListParams{Search: "a_b"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].Target != "email=a_b@x" {
		t.Fatalf("search for a_b should not treat _ as a wildcard, got %+v", page.Items)
	}
}

func TestAuditPruneDropsOnlyOldRows(t *testing.T) {
	s := setupAuditDB(t)
	recordAll(t, s,
		model.AuditLog{CreatedAt: 100, Action: "POST /a", Success: true},
		model.AuditLog{CreatedAt: 200, Action: "POST /b", Success: true},
		model.AuditLog{CreatedAt: 300, Action: "POST /c", Success: true},
	)
	removed, err := s.Prune(250)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}
	page, _ := s.List(AuditListParams{})
	if got := actions(page); !equalStrings(got, []string{"POST /c"}) {
		t.Errorf("remaining = %v", got)
	}
}

func TestAuditRecordsLoginAttemptsFromTheEventBus(t *testing.T) {
	s := setupAuditDB(t)
	at := time.UnixMilli(5000)
	s.HandleEvent(eventbus.Event{Type: eventbus.EventLoginAttempt, Timestamp: at, Data: &eventbus.LoginEventData{
		Username: "admin", IP: "203.0.113.9", Status: "fail", Reason: "wrong password",
	}})
	s.HandleEvent(eventbus.Event{Type: eventbus.EventCPUHigh, Timestamp: at})

	page, err := s.List(AuditListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Fatalf("total = %d, want only the login row", page.Total)
	}
	got := page.Items[0]
	want := model.AuditLog{Id: got.Id, CreatedAt: 5000, Actor: "admin", Ip: "203.0.113.9", Action: AuditActionLogin, Target: "wrong password", Success: false}
	if got != want {
		t.Errorf("row = %+v, want %+v", got, want)
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
