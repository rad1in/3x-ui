package service

import (
	"errors"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type approvalFixture struct {
	svc                *ApprovalService
	alice, bob, viewer *model.User
}

func newApprovalFixture(t *testing.T, gated ...string) *approvalFixture {
	t.Helper()
	setupBulkDB(t)
	f := &approvalFixture{svc: NewApprovalService()}
	if err := database.GetDB().Where("1 = 1").Delete(&model.User{}).Error; err != nil {
		t.Fatal(err)
	}
	f.alice = &model.User{Username: "alice", Password: "x", Role: model.AdminRoleOwner}
	f.bob = &model.User{Username: "bob", Password: "x", Role: model.AdminRoleAdmin}
	f.viewer = &model.User{Username: "vera", Password: "x", Role: model.AdminRoleViewer}
	for _, u := range []*model.User{f.alice, f.bob, f.viewer} {
		if err := database.GetDB().Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := (&SecurityConfigService{}).Save(&SecurityConfig{ApprovalActions: gated}); err != nil {
		t.Fatal(err)
	}
	return f
}

func inboundCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := database.GetDB().Model(&model.Inbound{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func auditActions(t *testing.T) []string {
	t.Helper()
	var rows []model.AuditLog
	if err := database.GetDB().Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, r := range rows {
		out = append(out, r.Actor+":"+r.Action)
	}
	return out
}

func TestApprovalRequiredOnlyForGatedActionsWithASecondAdmin(t *testing.T) {
	f := newApprovalFixture(t, ApprovalInboundDelete)

	if on, err := f.svc.Required(f.alice, ApprovalInboundDelete); err != nil || !on {
		t.Fatalf("gated action with a second admin: on=%v err=%v", on, err)
	}
	if on, _ := f.svc.Required(f.alice, ApprovalNodeDelete); on {
		t.Fatal("an action the owner did not gate was held for approval")
	}
	// bob is the only other owner/admin; a viewer is not an approver.
	if err := database.GetDB().Delete(f.bob).Error; err != nil {
		t.Fatal(err)
	}
	if on, _ := f.svc.Required(f.alice, ApprovalInboundDelete); on {
		t.Fatal("the gate locked out the only admin: nobody could ever approve")
	}
}

func TestApprovalDeletesTheInboundOnlyAfterAnotherAdminApproves(t *testing.T) {
	f := newApprovalFixture(t, ApprovalInboundDelete)
	ib := mkInbound(t, 24001, model.VLESS, `{"clients":[]}`)

	row, err := f.svc.Request(f.alice, "203.0.113.5", ApprovalInboundDelete, "inbound #1", InboundDeleteApproval{Ids: []int{ib.Id}})
	if err != nil {
		t.Fatal(err)
	}
	if inboundCount(t) != 1 {
		t.Fatal("the inbound was deleted before anyone approved")
	}
	if _, err := f.svc.Approve(f.alice, "203.0.113.5", row.Id); !errors.Is(err, ErrApprovalSelf) {
		t.Fatalf("self approval: got %v", err)
	}
	if _, err := f.svc.Approve(f.viewer, "198.51.100.1", row.Id); !errors.Is(err, ErrApprovalForbidden) {
		t.Fatalf("viewer approval: got %v", err)
	}
	if inboundCount(t) != 1 {
		t.Fatal("a refused approval still deleted the inbound")
	}

	done, err := f.svc.Approve(f.bob, "198.51.100.2", row.Id)
	if err != nil || done.Status != ApprovalApproved || done.DecidedByName != "bob" {
		t.Fatalf("approve: %+v err=%v", done, err)
	}
	if inboundCount(t) != 0 {
		t.Fatal("approved deletion did not run")
	}
	if _, err := f.svc.Approve(f.bob, "198.51.100.2", row.Id); !errors.Is(err, ErrApprovalClosed) {
		t.Fatalf("second approval must not run the action again: %v", err)
	}
	want := []string{"alice:approval.requested", "bob:approval.approved"}
	if got := auditActions(t); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("audit = %v, want %v", got, want)
	}
}

func TestApprovalRejectCancelAndExpiryNeverRunTheAction(t *testing.T) {
	f := newApprovalFixture(t, ApprovalInboundDelete)
	ib := mkInbound(t, 24002, model.VLESS, `{"clients":[]}`)
	request := func() *model.PendingAction {
		row, err := f.svc.Request(f.alice, "", ApprovalInboundDelete, "inbound", InboundDeleteApproval{Ids: []int{ib.Id}})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}

	rejected := request()
	if _, err := f.svc.Reject(f.alice, "", rejected.Id); !errors.Is(err, ErrApprovalSelf) {
		t.Fatalf("requester rejecting through the decision path: %v", err)
	}
	if got, err := f.svc.Reject(f.bob, "", rejected.Id); err != nil || got.Status != ApprovalRejected {
		t.Fatalf("reject: %+v %v", got, err)
	}
	if _, err := f.svc.Approve(f.bob, "", rejected.Id); !errors.Is(err, ErrApprovalClosed) {
		t.Fatalf("approve after reject: %v", err)
	}

	cancelled := request()
	if _, err := f.svc.Cancel(f.bob, "", cancelled.Id); !errors.Is(err, ErrApprovalForbidden) {
		t.Fatalf("someone else cancelling: %v", err)
	}
	if got, err := f.svc.Cancel(f.alice, "", cancelled.Id); err != nil || got.Status != ApprovalCancelled {
		t.Fatalf("cancel: %+v %v", got, err)
	}

	stale := request()
	f.svc.now = func() time.Time { return time.Now().Add(approvalTTL + time.Minute) }
	if _, err := f.svc.Approve(f.bob, "", stale.Id); !errors.Is(err, ErrApprovalClosed) {
		t.Fatalf("approving an expired request: %v", err)
	}
	rows, err := f.svc.List(f.bob)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Id == stale.Id && r.Status != ApprovalExpired {
			t.Fatalf("expired request listed as %q", r.Status)
		}
	}
	if inboundCount(t) != 1 {
		t.Fatal("a rejected, cancelled or expired request deleted the inbound")
	}
}

func TestApprovedBulkClientDeleteAndFailureAreRecorded(t *testing.T) {
	f := newApprovalFixture(t, ApprovalClientsBulkDelete, ApprovalNodeDelete)
	clients := []model.Client{{Email: "carol@x", ID: "33333333-3333-3333-3333-333333333333", SubID: "sc", Enable: true}}
	ib := mkInbound(t, 24003, model.VLESS, clientsSettings(t, clients))
	if err := (&ClientService{}).SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatal(err)
	}

	row, _ := f.svc.Request(f.alice, "", ApprovalClientsBulkDelete, "1 client", ClientsBulkDeleteApproval{Emails: []string{"carol@x"}})
	done, err := f.svc.Approve(f.bob, "", row.Id)
	if err != nil || done.Result != "deleted 1, skipped 0" {
		t.Fatalf("bulk delete: %+v err=%v", done, err)
	}
	var left int64
	database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "carol@x").Count(&left)
	if left != 0 {
		t.Fatal("the approved bulk delete left the client behind")
	}

	// A node that still owns an inbound cannot be deleted, so the approved action fails.
	node := &model.Node{Name: "n1", Address: "127.0.0.1", Port: 2096, ApiToken: "tok", Enable: true, Status: "offline"}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	nodeInbound(t, node.Id, 24004, nil)
	busy, _ := f.svc.Request(f.alice, "", ApprovalNodeDelete, "node 1", NodeDeleteApproval{Id: node.Id})
	failed, err := f.svc.Approve(f.bob, "", busy.Id)
	if err == nil || failed.Status != ApprovalFailed || failed.Result == "" {
		t.Fatalf("a failing action must be recorded as failed with its reason: %+v err=%v", failed, err)
	}
	if got := auditActions(t); got[len(got)-1] != "bob:approval.approved" {
		t.Fatalf("audit tail = %v", got)
	}
	var last model.AuditLog
	database.GetDB().Order("id DESC").First(&last)
	if last.Success {
		t.Fatal("a failed execution was audited as a success")
	}
}

func TestSecurityConfigRejectsUnknownApprovalAction(t *testing.T) {
	newApprovalFixture(t)
	if err := (&SecurityConfigService{}).Save(&SecurityConfig{ApprovalActions: []string{"server.reboot"}}); err == nil {
		t.Fatal("an action with no executor was accepted into the gate list")
	}
}
