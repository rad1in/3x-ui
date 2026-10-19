package service

import (
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/tableexport"
)

// maxOrderExportRows bounds one export so a huge history cannot exhaust memory.
const maxOrderExportRows = 50000

func reportStamp(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04")
}

// ExportOrders returns the orders of one status (every status when empty),
// newest first, up to maxOrderExportRows.
func (s *OrderService) ExportOrders(status string) ([]model.Order, error) {
	q := database.GetDB().Model(&model.Order{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var orders []model.Order
	if err := q.Order("id desc").Limit(maxOrderExportRows).Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

// OrdersTable leaves out the order token and pay link, which grant access to an order.
func OrdersTable(orders []model.Order, status string, now time.Time) tableexport.Table {
	title := "Orders"
	if status != "" {
		title += " (" + status + ")"
	}
	t := tableexport.Table{
		Title:    title,
		Subtitle: "Generated " + now.Format(usageDayLayout) + " - " + strconv.Itoa(len(orders)) + " orders",
		Columns: []tableexport.Column{
			{Header: "Id"}, {Header: "Created"}, {Header: "Status"}, {Header: "Kind"}, {Header: "Plan"},
			{Header: "Client"}, {Header: "Contact"}, {Header: "Amount (minor units)"}, {Header: "Currency"},
			{Header: "Gateway"}, {Header: "Discount code"}, {Header: "Paid"}, {Header: "Fulfilled"},
		},
	}
	for _, o := range orders {
		t.Rows = append(t.Rows, []any{
			o.Id, reportStamp(o.CreatedAt), o.Status, o.Kind, o.PlanName, o.ClientEmail, o.Contact,
			o.Amount, o.Currency, o.Gateway, o.DiscountCode, reportStamp(o.PaidAt), reportStamp(o.FulfilledAt),
		})
	}
	return t
}

// InboundTrafficTable summarises each inbound's traffic against its quota.
func InboundTrafficTable(inbounds []*model.Inbound, now time.Time) tableexport.Table {
	t := tableexport.Table{
		Title:    "Inbound traffic summary",
		Subtitle: "Generated " + now.Format(usageDayLayout) + " - " + strconv.Itoa(len(inbounds)) + " inbounds",
		Columns: []tableexport.Column{
			{Header: "Id"}, {Header: "Remark"}, {Header: "Protocol"}, {Header: "Port"}, {Header: "Enabled"},
			bytesColumn("Up"), bytesColumn("Down"), quotaColumn("Quota"), {Header: "Expiry"}, {Header: "Clients"},
		},
	}
	for _, in := range inbounds {
		t.Rows = append(t.Rows, []any{
			in.Id, in.Remark, string(in.Protocol), in.Port, in.Enable,
			in.Up, in.Down, in.Total, reportExpiry(in.ExpiryTime), len(in.ClientStats),
		})
	}
	return t
}
