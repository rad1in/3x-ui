package service

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestCreateClientsFromTemplateAppliesPreset(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}
	ib := mkInbound(t, 22001, model.VLESS, `{"clients":[]}`)

	tpl, err := svc.SaveClientTemplate(&model.ClientTemplate{
		Name: "30d-50g", TotalGB: 50 << 30, ExpiryDays: 30, LimitIP: 2,
		Group: "trial", InboundIds: []int{ib.Id},
	})
	if err != nil {
		t.Fatalf("SaveClientTemplate: %v", err)
	}

	before := time.Now().UnixMilli()
	res, err := svc.CreateClientsFromTemplate(inboundSvc, tpl.Id, []string{"a@x", "b@x"})
	if err != nil {
		t.Fatalf("CreateClientsFromTemplate: %v", err)
	}
	if len(res.Created) != 2 || len(res.Failed) != 0 {
		t.Fatalf("result = %+v, want 2 created and none failed", res)
	}

	rec := lookupClientRecord(t, "a@x")
	if rec.TotalGB != 50<<30 || rec.LimitIP != 2 || rec.Group != "trial" {
		t.Fatalf("record = %+v, preset not applied", rec)
	}
	wantMin := before + 30*24*3600*1000
	if rec.ExpiryTime < wantMin || rec.ExpiryTime > wantMin+60*1000 {
		t.Fatalf("expiry = %d, want about now+30d (%d)", rec.ExpiryTime, wantMin)
	}
	if a, b := lookupClientRecord(t, "a@x"), lookupClientRecord(t, "b@x"); a.UUID == b.UUID || a.SubID == b.SubID {
		t.Fatalf("clients share credentials: %q/%q", a.UUID, b.UUID)
	}
}

func TestCreateClientsFromTemplateDelayedStartUsesNegativeExpiry(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	ib := mkInbound(t, 22002, model.VLESS, `{"clients":[]}`)
	tpl, err := svc.SaveClientTemplate(&model.ClientTemplate{
		Name: "delayed", ExpiryDays: 7, DelayedStart: true, InboundIds: []int{ib.Id},
	})
	if err != nil {
		t.Fatalf("SaveClientTemplate: %v", err)
	}
	if _, err := svc.CreateClientsFromTemplate(&InboundService{}, tpl.Id, []string{"d@x"}); err != nil {
		t.Fatalf("CreateClientsFromTemplate: %v", err)
	}
	if got := lookupClientRecord(t, "d@x").ExpiryTime; got != -7*24*3600*1000 {
		t.Fatalf("expiry = %d, want %d", got, -7*24*3600*1000)
	}
}

func TestCreateClientsFromTemplateReportsDuplicateEmail(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	ib := mkInbound(t, 22003, model.VLESS, `{"clients":[]}`)
	tpl, err := svc.SaveClientTemplate(&model.ClientTemplate{Name: "t", InboundIds: []int{ib.Id}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateClientsFromTemplate(&InboundService{}, tpl.Id, []string{"dup@x"}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.CreateClientsFromTemplate(&InboundService{}, tpl.Id, []string{"dup@x", "ok@x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 1 || res.Created[0] != "ok@x" || res.Failed["dup@x"] == "" {
		t.Fatalf("result = %+v, want ok@x created and dup@x failed", res)
	}
}

func TestSaveClientTemplateRejectsDuplicateNameAndMissingInbounds(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	if _, err := svc.SaveClientTemplate(&model.ClientTemplate{Name: "x"}); err == nil {
		t.Fatal("template without inbounds accepted")
	}
	if _, err := svc.SaveClientTemplate(&model.ClientTemplate{Name: "x", InboundIds: []int{1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveClientTemplate(&model.ClientTemplate{Name: "x", InboundIds: []int{1}}); err == nil {
		t.Fatal("duplicate template name accepted")
	}
}
