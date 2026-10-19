package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func seedOrder(t *testing.T, planID int, status string) *model.Order {
	t.Helper()
	o := &model.Order{
		Token: randomOrderToken(), PlanId: planID, Kind: model.OrderKindNew,
		Amount: 100, Currency: "USD", Status: status, ClientEmail: "c@x",
	}
	if err := database.GetDB().Create(o).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	return o
}

func TestListOrdersFiltersByStatusNewestFirst(t *testing.T) {
	setupBulkDB(t)
	first := seedOrder(t, 1, model.OrderStatusReview)
	seedOrder(t, 1, model.OrderStatusPaid)
	third := seedOrder(t, 1, model.OrderStatusReview)

	page, err := (&OrderService{}).ListOrders(model.OrderStatusReview, 10, 0)
	if err != nil {
		t.Fatalf("ListOrders: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 2 || page.Items[0].Id != third.Id || page.Items[1].Id != first.Id {
		t.Fatalf("page = %+v, want the 2 review orders newest first", page)
	}
	small, err := (&OrderService{}).ListOrders("", 1, 1)
	if err != nil {
		t.Fatalf("ListOrders paged: %v", err)
	}
	if small.Total != 3 || len(small.Items) != 1 {
		t.Fatalf("paged = %+v, want total 3 and one row", small)
	}
}
