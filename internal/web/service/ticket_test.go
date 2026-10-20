package service

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
)

func ticketFixture(t *testing.T, port int) (*shopFixture, model.ClientRecord) {
	t.Helper()
	f := newShopFixture(t, port)
	return f, seedClient(t, f, "alice")
}

func TestOpenTicketNeedsProofOfTheAccountOrItsOrderToken(t *testing.T) {
	f, rec := ticketFixture(t, 29201)
	svc := &TicketService{}
	order := f.newOrder(t, "newbie")

	for _, tc := range []struct {
		name string
		auth TicketAuth
		code string
	}{
		{"wrong subscription id", TicketAuth{Email: "alice", Proof: "nope"}, ErrProofInvalid},
		{"no proof at all", TicketAuth{Email: "alice"}, ErrProofInvalid},
		{"unknown account", TicketAuth{Email: "ghost", Proof: rec.SubID}, ErrProofInvalid},
		{"unknown order token", TicketAuth{OrderToken: "deadbeef"}, ErrOrderNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Open(tc.auth, "Help", "text")
			wantSalesError(t, err, tc.code)
		})
	}

	byProof, err := svc.Open(TicketAuth{Email: "alice", Proof: rec.SubID}, "Help", "It drops")
	if err != nil || byProof.Ticket.ClientEmail != "alice" || byProof.Ticket.Status != model.TicketOpen || len(byProof.Messages) != 1 {
		t.Fatalf("Open by proof = %+v, %v", byProof, err)
	}
	byToken, err := svc.Open(TicketAuth{OrderToken: order.Token}, "Where is my account", "Paid yesterday")
	if err != nil || byToken.Ticket.ClientEmail != "newbie" {
		t.Fatalf("Open by order token = %+v, %v; want the order's account", byToken, err)
	}
}

func TestCustomersOnlySeeTheirOwnTickets(t *testing.T) {
	f, alice := ticketFixture(t, 29202)
	bob := seedClient(t, f, "bob")
	svc := &TicketService{}
	a, _ := svc.Open(TicketAuth{Email: "alice", Proof: alice.SubID}, "Mine", "a")
	if _, err := svc.Open(TicketAuth{Email: "bob", Proof: bob.SubID}, "Theirs", "b"); err != nil {
		t.Fatal(err)
	}

	list, err := svc.CustomerList(TicketAuth{Email: "alice", Proof: alice.SubID})
	if err != nil || len(list) != 1 || list[0].Id != a.Ticket.Id {
		t.Fatalf("alice's list = %v, %v; want only her ticket", list, err)
	}
	_, err = svc.CustomerGet(TicketAuth{Email: "bob", Proof: bob.SubID}, a.Ticket.Id)
	wantSalesError(t, err, ErrTicketNotFound)
	_, err = svc.CustomerReply(TicketAuth{Email: "bob", Proof: bob.SubID}, a.Ticket.Id, "hi")
	wantSalesError(t, err, ErrTicketNotFound)
}

func TestTicketConversationAndStatusFollowWhoSpokeLast(t *testing.T) {
	_, alice := ticketFixture(t, 29203)
	svc := &TicketService{}
	auth := TicketAuth{Email: "alice", Proof: alice.SubID}
	opened, _ := svc.Open(auth, "Help", "first")

	answered, err := svc.AdminReply(opened.Ticket.Id, "try again")
	if err != nil || answered.Ticket.Status != model.TicketAnswered || len(answered.Messages) != 2 || answered.Messages[1].Author != model.TicketByAdmin {
		t.Fatalf("AdminReply = %+v, %v", answered, err)
	}
	back, err := svc.CustomerReply(auth, opened.Ticket.Id, "still broken")
	if err != nil || back.Ticket.Status != model.TicketOpen || len(back.Messages) != 3 {
		t.Fatalf("CustomerReply = %+v, %v; want the ticket open again", back, err)
	}
	if _, err := svc.Close(opened.Ticket.Id); err != nil {
		t.Fatal(err)
	}
	_, err = svc.CustomerReply(auth, opened.Ticket.Id, "more")
	wantSalesError(t, err, ErrTicketClosed)
	_, err = svc.AdminReply(opened.Ticket.Id, "more")
	wantSalesError(t, err, ErrTicketClosed)
	reopened, err := svc.Reopen(opened.Ticket.Id)
	if err != nil || reopened.Status != model.TicketOpen {
		t.Fatalf("Reopen = %+v, %v", reopened, err)
	}
}

func TestTicketTextIsPlainAndBounded(t *testing.T) {
	_, alice := ticketFixture(t, 29204)
	svc := &TicketService{}
	auth := TicketAuth{Email: "alice", Proof: alice.SubID}

	for _, tc := range []struct{ name, subject, body string }{
		{"empty subject", "  ", "x"}, {"empty body", "s", " \n "},
		{"long subject", strings.Repeat("s", maxTicketSubject+1), "x"},
		{"long body", "s", strings.Repeat("b", maxTicketBody+1)},
		{"only control characters", "\x00\x07", "\x1b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Open(auth, tc.subject, tc.body)
			wantSalesError(t, err, ErrTicketInvalid)
		})
	}

	got, err := svc.Open(auth, "<b>hi</b>\x00", "<script>alert(1)</script>\r\nline\x07 two‮!")
	if err != nil {
		t.Fatal(err)
	}
	if got.Ticket.Subject != "<b>hi</b>" {
		t.Errorf("subject = %q, want the markup kept as literal text and the NUL dropped", got.Ticket.Subject)
	}
	if want := "<script>alert(1)</script>\nline two!"; got.Messages[0].Body != want {
		t.Errorf("body = %q, want %q (CRLF folded, control and bidi-override characters dropped)", got.Messages[0].Body, want)
	}
}

func TestTicketLimitsStopFlooding(t *testing.T) {
	_, alice := ticketFixture(t, 29205)
	svc := &TicketService{}
	auth := TicketAuth{Email: "alice", Proof: alice.SubID}

	var last *TicketDetail
	for i := 0; i < maxOpenTickets; i++ {
		d, err := svc.Open(auth, "t", "b")
		if err != nil {
			t.Fatalf("Open %d: %v", i, err)
		}
		last = d
	}
	_, err := svc.Open(auth, "one too many", "b")
	wantSalesError(t, err, ErrTicketLimit)

	if _, err := svc.Close(last.Ticket.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Open(auth, "after closing one", "b"); err != nil {
		t.Fatalf("Open after closing: %v", err)
	}
}

func TestCustomerMessageRateIsCapped(t *testing.T) {
	_, alice := ticketFixture(t, 29206)
	svc := &TicketService{}
	auth := TicketAuth{Email: "alice", Proof: alice.SubID}
	d, err := svc.Open(auth, "t", "b")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < maxCustomerMessagesPerHour; i++ {
		if _, err := svc.CustomerReply(auth, d.Ticket.Id, "again"); err != nil {
			t.Fatalf("reply %d: %v", i, err)
		}
	}
	_, err = svc.CustomerReply(auth, d.Ticket.Id, "flood")
	wantSalesError(t, err, ErrRateLimited)
	if _, err := svc.AdminReply(d.Ticket.Id, "the admin is never throttled"); err != nil {
		t.Fatalf("AdminReply: %v", err)
	}
}

func TestTicketEventsReachTheNotifier(t *testing.T) {
	_, alice := ticketFixture(t, 29207)
	bus := eventbus.New(0)
	defer bus.Stop()
	var mu sync.Mutex
	var got []eventbus.Event
	bus.Subscribe("t", func(e eventbus.Event) { mu.Lock(); got = append(got, e); mu.Unlock() })
	old := eventBus
	SetEventBus(bus)
	defer SetEventBus(old)

	svc := &TicketService{}
	auth := TicketAuth{Email: "alice", Proof: alice.SubID}
	d, _ := svc.Open(auth, "Help <b>", "body")
	if _, err := svc.CustomerReply(auth, d.Ticket.Id, "more"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdminReply(d.Ticket.Id, "no event for admin replies"); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || got[0].Type != eventbus.EventTicketOpened || got[1].Type != eventbus.EventTicketReplied {
		t.Fatalf("events = %+v, want opened then replied (and none for the admin's reply)", got)
	}
	if data := got[0].Data.(*eventbus.TicketEventData); data.Subject != "Help <b>" || data.Email != "alice" || data.TicketId != d.Ticket.Id {
		t.Fatalf("event data = %+v", data)
	}
}

func TestAdminTicketListFiltersByStatus(t *testing.T) {
	_, alice := ticketFixture(t, 29208)
	svc := &TicketService{}
	auth := TicketAuth{Email: "alice", Proof: alice.SubID}
	a, _ := svc.Open(auth, "one", "b")
	b, _ := svc.Open(auth, "two", "b")
	if _, err := svc.Close(a.Ticket.Id); err != nil {
		t.Fatal(err)
	}

	open, err := svc.List(model.TicketOpen, 10, 0)
	if err != nil || open.Total != 1 || open.Items[0].Id != b.Ticket.Id {
		t.Fatalf("open list = %+v, %v", open, err)
	}
	// Same-millisecond writes tie on updated_at; pin the older ticket so the order is decided by time.
	if err := database.GetDB().Model(&model.Ticket{}).Where("id = ?", b.Ticket.Id).Update("updated_at", 1).Error; err != nil {
		t.Fatal(err)
	}
	all, _ := svc.List("", 10, 0)
	if all.Total != 2 || all.Items[0].Id != a.Ticket.Id {
		t.Fatalf("all = %+v, want the most recently active (just closed) ticket first", all)
	}
	_, err = svc.Get(999)
	wantSalesError(t, err, ErrTicketNotFound)
}
