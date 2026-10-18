package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestWalletPaymentNeedsAProofEvenForAClientWithoutASubscriptionId(t *testing.T) {
	f := shopWithWallet(t, 29016)
	seedClient(t, f, "alice")
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "alice").Update("sub_id", "").Error; err != nil {
		t.Fatalf("clear sub id: %v", err)
	}
	if _, err := (&WalletService{}).Adjust("alice", "IRT", 250000, ""); err != nil {
		t.Fatalf("credit: %v", err)
	}
	_, _, err := f.orders.Checkout(&InboundService{}, CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindRenew, Email: "alice", Gateway: GatewayWallet})
	wantSalesError(t, err, ErrProofInvalid)
}
