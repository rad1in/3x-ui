package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func mkTemplate(t *testing.T, port int) *model.ClientTemplate {
	t.Helper()
	ib := mkInbound(t, port, model.VLESS, `{"clients":[]}`)
	tpl, err := (&ClientService{}).SaveClientTemplate(&model.ClientTemplate{
		Name: fmt.Sprintf("tpl-%d", port), InboundIds: []int{ib.Id},
	})
	if err != nil {
		t.Fatalf("SaveClientTemplate: %v", err)
	}
	return tpl
}

func TestSavePlanValidation(t *testing.T) {
	setupBulkDB(t)
	tpl := mkTemplate(t, 23001)
	svc := &PlanService{}
	good := model.Plan{Name: "Monthly", Price: 250000, Currency: "IRT", TotalGB: 50 << 30, DurationDays: 30, TemplateId: tpl.Id, Enabled: true}

	cases := []struct {
		name    string
		mutate  func(*model.Plan)
		wantErr string
	}{
		{"blank name", func(p *model.Plan) { p.Name = "  " }, "plan name is required"},
		{"unknown template", func(p *model.Plan) { p.TemplateId = 9999 }, "client template not found"},
		{"unknown currency", func(p *model.Plan) { p.Currency = "XYZ" }, "unsupported currency"},
		{"negative price", func(p *model.Plan) { p.Price = -1 }, "price cannot be negative"},
		{"negative duration", func(p *model.Plan) { p.DurationDays = -3 }, "quota, duration and IP limit cannot be negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := good
			tc.mutate(&p)
			_, err := svc.SavePlan(&p)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("SavePlan error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}

	saved, err := svc.SavePlan(&good)
	if err != nil {
		t.Fatalf("SavePlan(good): %v", err)
	}
	if saved.Id == 0 || !saved.Enabled {
		t.Fatalf("saved = %+v, want an id and Enabled kept true", saved)
	}
	dup := good
	dup.Id = 0
	if _, err := svc.SavePlan(&dup); err == nil || !strings.Contains(err.Error(), "plan name already exists") {
		t.Fatalf("duplicate name error = %v", err)
	}
}

func TestListPlansEnabledOnlyKeepsSortOrder(t *testing.T) {
	setupBulkDB(t)
	tpl := mkTemplate(t, 23002)
	svc := &PlanService{}
	for _, p := range []model.Plan{
		{Name: "b-second", Price: 2, Currency: "USD", TemplateId: tpl.Id, Enabled: true, SortOrder: 2},
		{Name: "a-hidden", Price: 1, Currency: "USD", TemplateId: tpl.Id, Enabled: false, SortOrder: 0},
		{Name: "c-first", Price: 3, Currency: "USD", TemplateId: tpl.Id, Enabled: true, SortOrder: 1},
	} {
		if _, err := svc.SavePlan(&p); err != nil {
			t.Fatalf("SavePlan(%s): %v", p.Name, err)
		}
	}
	all, err := svc.ListPlans(false)
	if err != nil || len(all) != 3 {
		t.Fatalf("ListPlans(false) = %v, %v; want 3 plans", all, err)
	}
	shop, err := svc.ListPlans(true)
	if err != nil {
		t.Fatalf("ListPlans(true): %v", err)
	}
	if len(shop) != 2 || shop[0].Name != "c-first" || shop[1].Name != "b-second" {
		t.Fatalf("shop list = %+v, want [c-first b-second]", shop)
	}
}

func TestDeletePlanRefusedWhileOrderOpen(t *testing.T) {
	setupBulkDB(t)
	tpl := mkTemplate(t, 23003)
	svc := &PlanService{}
	plan, err := svc.SavePlan(&model.Plan{Name: "p", Price: 1, Currency: "USD", TemplateId: tpl.Id, Enabled: true})
	if err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	order := &model.Order{Token: "tok", PlanId: plan.Id, Kind: model.OrderKindNew, Amount: 1, Currency: "USD", Status: model.OrderStatusReview}
	if err := database.GetDB().Create(order).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}
	if err := svc.DeletePlan(plan.Id); err == nil || !strings.Contains(err.Error(), "open orders") {
		t.Fatalf("DeletePlan error = %v, want refusal naming open orders", err)
	}
	if err := database.GetDB().Model(order).Update("status", model.OrderStatusPaid).Error; err != nil {
		t.Fatalf("close order: %v", err)
	}
	if err := svc.DeletePlan(plan.Id); err != nil {
		t.Fatalf("DeletePlan after the order closed: %v", err)
	}
}
