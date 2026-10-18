package service

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func mkScopedOperator(t *testing.T, name string, inbounds []int, groups []string) *model.User {
	t.Helper()
	rawInbounds, _ := json.Marshal(inbounds)
	rawGroups, _ := json.Marshal(groups)
	u := &model.User{Username: name, Password: "x", Role: model.AdminRoleOperator, ScopeInboundIds: string(rawInbounds), ScopeGroups: string(rawGroups)}
	if err := database.GetDB().Create(u).Error; err != nil {
		t.Fatalf("create operator: %v", err)
	}
	return u
}

func visibleTo(t *testing.T, u *model.User, emails ...string) []string {
	t.Helper()
	got, err := ScopeForUser(u).VisibleEmails(emails)
	if err != nil {
		t.Fatalf("VisibleEmails: %v", err)
	}
	return got
}

func inboundsOfClient(t *testing.T, svc *ClientService, email string) []int {
	t.Helper()
	rec, err := svc.GetRecordByEmail(nil, email)
	if err != nil {
		t.Fatalf("record %s: %v", email, err)
	}
	ids, err := svc.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		t.Fatalf("inbounds of %s: %v", email, err)
	}
	slices.Sort(ids)
	return ids
}

func seedGroupedClients(t *testing.T, svc *ClientService, group string, emails ...string) {
	t.Helper()
	clients := make([]model.Client, 0, len(emails))
	for i, e := range emails {
		clients = append(clients, model.Client{Email: e, ID: "aaaaaaaa-0000-0000-0000-00000000000" + string(rune('1'+i)), SubID: "sub-" + e, Enable: true, Group: group})
	}
	ib := mkInbound(t, 31000+len(emails), model.VLESS, clientsSettings(t, clients))
	if err := svc.SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestTransferMovesGroupClientsToTheTargetsGroup(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	seedGroupedClients(t, svc, "team-a", "c1@x", "c2@x")
	from := mkScopedOperator(t, "a", nil, []string{"team-a"})
	to := mkScopedOperator(t, "b", nil, []string{"team-b"})

	res, _, err := (&ClientTransferService{}).Transfer(&InboundService{}, ClientTransferRequest{FromAdminId: from.Id, ToAdminId: to.Id, Group: "team-b"})
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if len(res.Moved) != 2 || len(res.Skipped) != 0 {
		t.Fatalf("result = %+v, want both clients moved", res)
	}
	if got := visibleTo(t, from, "c1@x", "c2@x"); len(got) != 0 {
		t.Fatalf("source still sees %v", got)
	}
	if got := visibleTo(t, to, "c1@x", "c2@x"); len(got) != 2 {
		t.Fatalf("target sees %v, want both", got)
	}
}

func TestTransferOfNamedClientsLeavesTheRestWithTheSource(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	seedGroupedClients(t, svc, "team-a", "c1@x", "c2@x")
	from := mkScopedOperator(t, "a", nil, []string{"team-a"})
	to := mkScopedOperator(t, "b", nil, []string{"team-b"})

	res, _, err := (&ClientTransferService{}).Transfer(&InboundService{}, ClientTransferRequest{FromAdminId: from.Id, ToAdminId: to.Id, Emails: []string{"c1@x", "outsider@x"}, Group: "team-b"})
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if len(res.Moved) != 1 || res.Moved[0] != "c1@x" {
		t.Fatalf("moved = %v, want only c1@x", res.Moved)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Email != "outsider@x" {
		t.Fatalf("skipped = %+v, want outsider@x reported as not the source's", res.Skipped)
	}
	if got := visibleTo(t, from, "c1@x", "c2@x"); len(got) != 1 || got[0] != "c2@x" {
		t.Fatalf("source sees %v, want only c2@x", got)
	}
}

func TestTransferMovesInboundClientsToTheTargetsInbound(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	clients := []model.Client{{Email: "c1@x", ID: "11111111-1111-1111-1111-111111111111", SubID: "s1", Enable: true}}
	ibA := mkInbound(t, 31101, model.VLESS, clientsSettings(t, clients))
	ibB := mkInbound(t, 31102, model.VLESS, `{"clients":[]}`)
	if err := svc.SyncInbound(nil, ibA.Id, clients); err != nil {
		t.Fatal(err)
	}
	from := mkScopedOperator(t, "a", []int{ibA.Id}, nil)
	to := mkScopedOperator(t, "b", []int{ibB.Id}, nil)

	res, _, err := (&ClientTransferService{}).Transfer(&InboundService{}, ClientTransferRequest{FromAdminId: from.Id, ToAdminId: to.Id})
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if len(res.Moved) != 1 || len(res.Skipped) != 0 {
		t.Fatalf("result = %+v", res)
	}
	if got := inboundsOfClient(t, svc, "c1@x"); !slices.Equal(got, []int{ibB.Id}) {
		t.Fatalf("client is on inbounds %v, want only the target's %d", got, ibB.Id)
	}
	if len(visibleTo(t, from, "c1@x")) != 0 || len(visibleTo(t, to, "c1@x")) != 1 {
		t.Fatal("the client did not change hands")
	}
}

func TestTransferRefusesClientsOnAnInboundBothAdminsShare(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	clients := []model.Client{{Email: "c1@x", ID: "11111111-1111-1111-1111-111111111111", SubID: "s1", Enable: true}}
	shared := mkInbound(t, 31201, model.VLESS, clientsSettings(t, clients))
	if err := svc.SyncInbound(nil, shared.Id, clients); err != nil {
		t.Fatal(err)
	}
	from := mkScopedOperator(t, "a", []int{shared.Id}, nil)
	to := mkScopedOperator(t, "b", []int{shared.Id}, nil)

	res, _, err := (&ClientTransferService{}).Transfer(&InboundService{}, ClientTransferRequest{FromAdminId: from.Id, ToAdminId: to.Id})
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if len(res.Moved) != 0 || len(res.Skipped) != 1 || res.Skipped[0].Reason != TransferSharedInbound {
		t.Fatalf("result = %+v, want the client skipped as sitting on a shared inbound", res)
	}
}

func TestTransferNeedsATargetGroupWhenGroupClientsMove(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	seedGroupedClients(t, svc, "team-a", "c1@x")
	from := mkScopedOperator(t, "a", nil, []string{"team-a"})
	to := mkScopedOperator(t, "b", nil, []string{"b1", "b2"})
	transfer := &ClientTransferService{}

	res, _, err := transfer.Transfer(&InboundService{}, ClientTransferRequest{FromAdminId: from.Id, ToAdminId: to.Id})
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if len(res.Moved) != 0 || len(res.Skipped) != 1 || res.Skipped[0].Reason != TransferNeedsGroup {
		t.Fatalf("result = %+v, want the client skipped for lack of a target group", res)
	}
	res, _, err = transfer.Transfer(&InboundService{}, ClientTransferRequest{FromAdminId: from.Id, ToAdminId: to.Id, Group: "elsewhere"})
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if len(res.Moved) != 0 || len(res.Skipped) != 1 || res.Skipped[0].Reason != TransferNeedsGroup {
		t.Fatalf("result = %+v, want a group outside the target's scope refused", res)
	}
}

func TestTransferFollowsTheOnlyGroupOfTheTarget(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	seedGroupedClients(t, svc, "team-a", "c1@x")
	from := mkScopedOperator(t, "a", nil, []string{"team-a"})
	to := mkScopedOperator(t, "b", nil, []string{"team-b"})

	res, _, err := (&ClientTransferService{}).Transfer(&InboundService{}, ClientTransferRequest{FromAdminId: from.Id, ToAdminId: to.Id})
	if err != nil || len(res.Moved) != 1 {
		t.Fatalf("Transfer = %+v, %v; want the client moved into the target's only group", res, err)
	}
}

func TestTransferNeedsTwoDifferentOperators(t *testing.T) {
	setupBulkDB(t)
	a := mkScopedOperator(t, "a", nil, []string{"x"})
	owner := &model.User{Username: "boss", Password: "x", Role: model.AdminRoleOwner, ScopeInboundIds: "[]", ScopeGroups: "[]"}
	if err := database.GetDB().Create(owner).Error; err != nil {
		t.Fatal(err)
	}
	transfer := &ClientTransferService{}

	for name, req := range map[string]ClientTransferRequest{
		"same admin":      {FromAdminId: a.Id, ToAdminId: a.Id},
		"owner as source": {FromAdminId: owner.Id, ToAdminId: a.Id},
		"owner as target": {FromAdminId: a.Id, ToAdminId: owner.Id},
		"unknown admin":   {FromAdminId: a.Id, ToAdminId: 999},
	} {
		if _, _, err := transfer.Transfer(&InboundService{}, req); !errors.Is(err, ErrTransferAdmins) {
			t.Fatalf("%s: err = %v, want ErrTransferAdmins", name, err)
		}
	}
}
