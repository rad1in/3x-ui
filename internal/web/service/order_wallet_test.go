package service

import (
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// shopWithWallet is the card fixture with wallet payments and 10% referral
// bonuses switched on, and a priced plan (250000 IRT).
func shopWithWallet(t *testing.T, port int) *shopFixture {
	t.Helper()
	f := newShopFixture(t, port)
	cfg := &SalesConfig{
		WalletEnabled: true,
		Card:          CardPayConfig{Enabled: true, CardNumber: "6037991812345678"},
		Referral:      ReferralConfig{Enabled: true, BonusPercent: 10},
	}
	if err := (&SalesConfigService{}).Save(cfg); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	return f
}

func seedClient(t *testing.T, f *shopFixture, email string) model.ClientRecord {
	t.Helper()
	if _, err := (&ClientService{}).Create(&InboundService{}, &ClientCreatePayload{
		Client: model.Client{Email: email, Enable: true, ExpiryTime: time.Now().UnixMilli() + 5*24*3600*1000}, InboundIds: f.tpl.InboundIds,
	}); err != nil {
		t.Fatalf("seed client %s: %v", email, err)
	}
	return lookupClientRecord(t, email)
}

func saveCode(t *testing.T, code model.DiscountCode) *model.DiscountCode {
	t.Helper()
	saved, err := (&DiscountService{}).SaveDiscount(&code)
	if err != nil {
		t.Fatalf("SaveDiscount: %v", err)
	}
	return saved
}

func (f *shopFixture) orderWith(in CreateOrderInput) (*model.Order, error) {
	in.PlanId = f.plan.Id
	if in.Kind == "" {
		in.Kind = model.OrderKindNew
	}
	if in.Gateway == "" {
		in.Gateway = GatewayCard
	}
	return f.orders.CreateOrder(in)
}

func balanceOf(t *testing.T, email, currency string) int64 {
	t.Helper()
	got, err := balanceIn(database.GetDB(), email, currency)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	return got
}

func TestPercentDiscountLowersTheAmountAndKeepsTheListPrice(t *testing.T) {
	f := shopWithWallet(t, 29001)
	saveCode(t, model.DiscountCode{Code: "welcome10", Kind: model.DiscountPercent, Value: 10, Enabled: true})

	o, err := f.orderWith(CreateOrderInput{Email: "alice", DiscountCode: "  Welcome10 "})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if o.Amount != 225000 || o.ListAmount != 250000 || o.DiscountCode != "WELCOME10" {
		t.Fatalf("order = amount %d list %d code %q, want 225000 / 250000 / WELCOME10", o.Amount, o.ListAmount, o.DiscountCode)
	}
	quote, err := f.orders.Quote(QuoteInput{PlanId: f.plan.Id, DiscountCode: "welcome10", Email: "bob"})
	if err != nil || quote.Final != 225000 || quote.Discount != 25000 || quote.List != 250000 {
		t.Fatalf("quote = %+v, %v; want the same numbers the order got", quote, err)
	}
}

func TestFixedDiscountMustMatchTheCurrencyAndLeaveSomethingToPay(t *testing.T) {
	f := shopWithWallet(t, 29002)
	saveCode(t, model.DiscountCode{Code: "BIG", Kind: model.DiscountFixed, Value: 400000, Currency: "IRT", Enabled: true})
	saveCode(t, model.DiscountCode{Code: "BUCK", Kind: model.DiscountFixed, Value: 500, Currency: "USD", Enabled: true})
	saveCode(t, model.DiscountCode{Code: "FIFTY", Kind: model.DiscountFixed, Value: 50000, Currency: "IRT", Enabled: true})

	_, err := f.orderWith(CreateOrderInput{Email: "alice", DiscountCode: "BIG"})
	wantSalesError(t, err, ErrDiscountInvalid)
	_, err = f.orderWith(CreateOrderInput{Email: "alice", DiscountCode: "BUCK"})
	wantSalesError(t, err, ErrDiscountInvalid)
	o, err := f.orderWith(CreateOrderInput{Email: "alice", DiscountCode: "FIFTY"})
	if err != nil || o.Amount != 200000 {
		t.Fatalf("order = %+v, %v; want 50000 off a 250000 price", o, err)
	}
}

func TestDiscountRefusals(t *testing.T) {
	f := shopWithWallet(t, 29003)
	saveCode(t, model.DiscountCode{Code: "OFF", Kind: model.DiscountPercent, Value: 10, Enabled: false})
	saveCode(t, model.DiscountCode{Code: "OLD", Kind: model.DiscountPercent, Value: 10, Enabled: true, ExpiresAt: time.Now().Add(-time.Hour).UnixMilli()})
	saveCode(t, model.DiscountCode{Code: "OTHER", Kind: model.DiscountPercent, Value: 10, Enabled: true, PlanIds: []int{f.plan.Id + 100}})
	for _, code := range []string{"NOPE", "OFF", "OLD", "OTHER"} {
		_, err := f.orderWith(CreateOrderInput{Email: "alice", DiscountCode: code})
		wantSalesError(t, err, ErrDiscountInvalid)
	}
	if n := countOrders(t); n != 0 {
		t.Fatalf("%d orders created by refused codes", n)
	}
}

func countOrders(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := database.GetDB().Model(&model.Order{}).Count(&n).Error; err != nil {
		t.Fatalf("count orders: %v", err)
	}
	return n
}

func TestDiscountUseLimitHoldsUntilTheOrderIsClosed(t *testing.T) {
	f := shopWithWallet(t, 29004)
	saveCode(t, model.DiscountCode{Code: "ONE", Kind: model.DiscountPercent, Value: 10, Enabled: true, MaxUses: 1})

	first, err := f.orderWith(CreateOrderInput{Email: "alice", DiscountCode: "ONE"})
	if err != nil {
		t.Fatalf("first use: %v", err)
	}
	_, err = f.orderWith(CreateOrderInput{Email: "bob", DiscountCode: "ONE"})
	wantSalesError(t, err, ErrDiscountUsed)

	if _, err := f.orders.RejectOrder(first.Id, "no payment"); err != nil {
		t.Fatalf("RejectOrder: %v", err)
	}
	if _, err := f.orderWith(CreateOrderInput{Email: "bob", DiscountCode: "ONE"}); err != nil {
		t.Fatalf("the use must come back when the order is rejected: %v", err)
	}
}

func TestDiscountUseLimitHoldsUnderConcurrentOrders(t *testing.T) {
	f := shopWithWallet(t, 29005)
	saveCode(t, model.DiscountCode{Code: "ONE", Kind: model.DiscountPercent, Value: 10, Enabled: true, MaxUses: 1})

	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.orderWith(CreateOrderInput{Email: "user" + string(rune('a'+i)) + "xx", DiscountCode: "ONE"})
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("%d orders got a one-use code (%v), want exactly one", ok, errs)
	}
}

func TestOncePerCustomerCodeCountsTheCustomersOwnOrders(t *testing.T) {
	f := shopWithWallet(t, 29006)
	saveCode(t, model.DiscountCode{Code: "ONCE", Kind: model.DiscountPercent, Value: 10, Enabled: true, OncePerCustomer: true})
	seedClient(t, f, "alice")
	seedClient(t, f, "bob")

	if _, err := f.orderWith(CreateOrderInput{Kind: model.OrderKindRenew, Email: "alice", DiscountCode: "ONCE"}); err != nil {
		t.Fatalf("first use by alice: %v", err)
	}
	_, err := f.orderWith(CreateOrderInput{Kind: model.OrderKindRenew, Email: "alice", DiscountCode: "ONCE"})
	wantSalesError(t, err, ErrDiscountUsed)
	if _, err := f.orderWith(CreateOrderInput{Kind: model.OrderKindRenew, Email: "bob", DiscountCode: "ONCE"}); err != nil {
		t.Fatalf("bob may use the code once too: %v", err)
	}
}

func TestSaveDiscountValidation(t *testing.T) {
	setupBulkDB(t)
	svc := &DiscountService{}
	bad := []model.DiscountCode{
		{Code: "ab", Kind: model.DiscountPercent, Value: 10},
		{Code: "HAS SPACE", Kind: model.DiscountPercent, Value: 10},
		{Code: "PCT0", Kind: model.DiscountPercent, Value: 0},
		{Code: "PCT100", Kind: model.DiscountPercent, Value: 100},
		{Code: "FIX0", Kind: model.DiscountFixed, Value: 0, Currency: "USD"},
		{Code: "FIXCUR", Kind: model.DiscountFixed, Value: 5, Currency: "XYZ"},
		{Code: "KIND", Kind: "bogus", Value: 5},
		{Code: "NEG", Kind: model.DiscountPercent, Value: 5, MaxUses: -1},
	}
	for _, c := range bad {
		if _, err := svc.SaveDiscount(&c); err == nil {
			t.Fatalf("SaveDiscount(%+v) succeeded", c)
		}
	}
	if _, err := svc.SaveDiscount(&model.DiscountCode{Code: "dup", Kind: model.DiscountPercent, Value: 5, Enabled: true}); err != nil {
		t.Fatalf("SaveDiscount: %v", err)
	}
	if _, err := svc.SaveDiscount(&model.DiscountCode{Code: "DUP", Kind: model.DiscountPercent, Value: 5}); err == nil {
		t.Fatal("a code differing only by case was accepted as new")
	}
	list, err := svc.ListDiscounts()
	if err != nil || len(list) != 1 || list[0].Code != "DUP" {
		t.Fatalf("list = %+v, %v", list, err)
	}
}

func TestWalletAdjustAndBalances(t *testing.T) {
	f := shopWithWallet(t, 29007)
	seedClient(t, f, "alice")
	w := &WalletService{}

	if _, err := w.Adjust("ghost", "USD", 100, ""); err == nil {
		t.Fatal("credited a wallet of a client that does not exist")
	}
	if _, err := w.Adjust("alice", "USD", 500, "welcome"); err != nil {
		t.Fatalf("credit: %v", err)
	}
	if _, err := w.Adjust("alice", "IRT", 70000, ""); err != nil {
		t.Fatalf("credit IRT: %v", err)
	}
	_, err := w.Adjust("alice", "USD", -501, "")
	wantSalesError(t, err, ErrWalletInsufficient)
	if _, err := w.Adjust("alice", "USD", -200, ""); err != nil {
		t.Fatalf("debit: %v", err)
	}
	got, err := w.Balances("alice")
	if err != nil || len(got) != 2 || got[0] != (WalletBalance{"IRT", 70000}) || got[1] != (WalletBalance{"USD", 300}) {
		t.Fatalf("balances = %+v, %v; want IRT 70000 and USD 300", got, err)
	}
}

func TestWalletPaymentRenewsAndDebitsTheDiscountedPrice(t *testing.T) {
	f := shopWithWallet(t, 29008)
	rec := seedClient(t, f, "alice")
	saveCode(t, model.DiscountCode{Code: "TEN", Kind: model.DiscountPercent, Value: 10, Enabled: true})
	w := &WalletService{}
	if _, err := w.Adjust("alice", "IRT", 300000, ""); err != nil {
		t.Fatalf("credit: %v", err)
	}

	o, _, err := f.orders.Checkout(&InboundService{}, CreateOrderInput{
		PlanId: f.plan.Id, Kind: model.OrderKindRenew, Email: "alice", Gateway: GatewayWallet, Proof: rec.SubID, DiscountCode: "TEN",
	})
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if o.Status != model.OrderStatusPaid || o.FulfilledAt == 0 || o.Amount != 225000 {
		t.Fatalf("order = %+v, want paid, delivered, 225000", o)
	}
	if got := balanceOf(t, "alice", "IRT"); got != 75000 {
		t.Fatalf("balance = %d, want 300000 - 225000", got)
	}
	if exp := lookupClientRecord(t, "alice").ExpiryTime; exp < rec.ExpiryTime+29*24*3600*1000 {
		t.Fatalf("expiry = %d, want the plan days added to the remaining time", exp)
	}
}

func TestWalletPaymentRefusals(t *testing.T) {
	f := shopWithWallet(t, 29009)
	rec := seedClient(t, f, "alice")
	if _, err := (&WalletService{}).Adjust("alice", "IRT", 100000, ""); err != nil {
		t.Fatalf("credit: %v", err)
	}
	renew := func(proof string) CreateOrderInput {
		return CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindRenew, Email: "alice", Gateway: GatewayWallet, Proof: proof}
	}

	_, _, err := f.orders.Checkout(&InboundService{}, renew("wrong"))
	wantSalesError(t, err, ErrProofInvalid)
	_, _, err = f.orders.Checkout(&InboundService{}, renew(""))
	wantSalesError(t, err, ErrProofInvalid)
	_, _, err = f.orders.Checkout(&InboundService{}, renew(rec.SubID))
	wantSalesError(t, err, ErrWalletInsufficient)
	_, _, err = f.orders.Checkout(&InboundService{}, CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindNew, Email: "newbie", Gateway: GatewayWallet, Proof: rec.SubID})
	wantSalesError(t, err, ErrGatewayUnavailable)

	if balanceOf(t, "alice", "IRT") != 100000 || countOrders(t) != 0 {
		t.Fatalf("balance %d, orders %d; a refused wallet payment must change nothing", balanceOf(t, "alice", "IRT"), countOrders(t))
	}
}

func TestWalletBalanceCannotBeSpentTwiceConcurrently(t *testing.T) {
	f := shopWithWallet(t, 29010)
	rec := seedClient(t, f, "alice")
	if _, err := (&WalletService{}).Adjust("alice", "IRT", 250000, ""); err != nil {
		t.Fatalf("credit: %v", err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, errs[i] = f.orders.Checkout(&InboundService{}, CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindRenew, Email: "alice", Gateway: GatewayWallet, Proof: rec.SubID})
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 || balanceOf(t, "alice", "IRT") != 0 {
		t.Fatalf("%d payments succeeded (%v), balance %d; want one payment and a zero balance", ok, errs, balanceOf(t, "alice", "IRT"))
	}
}

func TestRejectingAWalletOrderRefundsItOnce(t *testing.T) {
	f := shopWithWallet(t, 29011)
	rec := seedClient(t, f, "alice")
	if _, err := (&WalletService{}).Adjust("alice", "IRT", 250000, ""); err != nil {
		t.Fatalf("credit: %v", err)
	}
	o, err := f.orders.CreateOrder(CreateOrderInput{PlanId: f.plan.Id, Kind: model.OrderKindRenew, Email: "alice", Gateway: GatewayWallet, Proof: rec.SubID})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if balanceOf(t, "alice", "IRT") != 0 {
		t.Fatalf("balance = %d, want the order to hold the money", balanceOf(t, "alice", "IRT"))
	}
	if _, err := f.orders.RejectOrder(o.Id, "admin decision"); err != nil {
		t.Fatalf("RejectOrder: %v", err)
	}
	if got := balanceOf(t, "alice", "IRT"); got != 250000 {
		t.Fatalf("balance = %d, want the refund", got)
	}
	_, err = f.orders.RejectOrder(o.Id, "again")
	wantSalesError(t, err, ErrOrderState)
	if got := balanceOf(t, "alice", "IRT"); got != 250000 {
		t.Fatalf("balance = %d after a second reject, want no second refund", got)
	}
}

func TestReferralCodeIsStablePerClient(t *testing.T) {
	f := shopWithWallet(t, 29012)
	seedClient(t, f, "alice")
	seedClient(t, f, "bob")
	r := &ReferralService{}
	a1, err := r.CodeFor("alice")
	if err != nil {
		t.Fatalf("CodeFor: %v", err)
	}
	a2, _ := r.CodeFor("alice")
	b, _ := r.CodeFor("bob")
	if a1 != a2 || a1 == b || len(a1) != referralLength {
		t.Fatalf("codes = %q %q %q, want stable, distinct, %d long", a1, a2, b, referralLength)
	}
	if _, err := r.CodeFor("ghost"); err == nil {
		t.Fatal("a code was issued to a client that does not exist")
	}
	if who, err := r.Referrer(" " + lower(a1) + " "); err != nil || who != "alice" {
		t.Fatalf("Referrer = %q, %v", who, err)
	}
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func TestReferredPurchasePaysTheReferrerOnceOnDelivery(t *testing.T) {
	f := shopWithWallet(t, 29013)
	seedClient(t, f, "alice")
	code, _ := (&ReferralService{}).CodeFor("alice")

	o, err := f.orderWith(CreateOrderInput{Email: "newbie", ReferralCode: code})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if o.ReferrerEmail != "alice" || o.ReferralCode != code {
		t.Fatalf("order = %+v, want the referrer recorded", o)
	}
	if balanceOf(t, "alice", "IRT") != 0 {
		t.Fatal("the bonus was paid before the sale was delivered")
	}
	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("FulfilOrder: %v", err)
	}
	if got := balanceOf(t, "alice", "IRT"); got != 25000 {
		t.Fatalf("referrer balance = %d, want 10%% of 250000", got)
	}
	(&ReferralService{}).creditBonus(o)
	if got := balanceOf(t, "alice", "IRT"); got != 25000 {
		t.Fatalf("referrer balance = %d after a second credit, want the bonus booked once", got)
	}
}

func TestReferralRefusalsAndSwitchedOff(t *testing.T) {
	f := shopWithWallet(t, 29014)
	seedClient(t, f, "alice")
	code, _ := (&ReferralService{}).CodeFor("alice")

	_, err := f.orderWith(CreateOrderInput{Email: "newbie", ReferralCode: "NOSUCH12"})
	wantSalesError(t, err, ErrReferralInvalid)
	renew, err := f.orderWith(CreateOrderInput{Kind: model.OrderKindRenew, Email: "alice", ReferralCode: code})
	if err != nil || renew.ReferrerEmail != "" {
		t.Fatalf("renewal order = %+v, %v; a referral only applies to a first purchase", renew, err)
	}

	if err := (&SalesConfigService{}).Save(&SalesConfig{Card: CardPayConfig{Enabled: true, CardNumber: "6037991812345678"}}); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	if stray, err := f.orderWith(CreateOrderInput{Email: "stray", ReferralCode: "NOSUCH12"}); err != nil || stray.ReferrerEmail != "" {
		t.Fatalf("order = %+v, %v; a referral code is ignored, not validated, while referrals are off", stray, err)
	}
	o, err := f.orderWith(CreateOrderInput{Email: "newbie", ReferralCode: code})
	if err != nil {
		t.Fatalf("CreateOrder with referrals off: %v", err)
	}
	if _, err := f.orders.FulfilOrder(&InboundService{}, o.Id); err != nil {
		t.Fatalf("FulfilOrder: %v", err)
	}
	if got := balanceOf(t, "alice", "IRT"); got != 0 {
		t.Fatalf("referrer balance = %d, want no bonus while referrals are off", got)
	}
}

func TestSelfReferralIsRefused(t *testing.T) {
	f := shopWithWallet(t, 29015)
	seedClient(t, f, "alice")
	code, _ := (&ReferralService{}).CodeFor("alice")
	// A new username equal to the referrer cannot exist (taken), so a referrer
	// reusing their own code on a fresh order is the case that matters.
	if _, err := (&ClientService{}).DeleteByEmail(&InboundService{}, "alice", false); err != nil {
		t.Fatalf("remove client: %v", err)
	}
	_, err := f.orderWith(CreateOrderInput{Email: "alice", ReferralCode: code})
	wantSalesError(t, err, ErrReferralInvalid)
}

func TestBookOnceIgnoresARepeatedRef(t *testing.T) {
	setupBulkDB(t)
	ref := "referral:order:7"
	for range 2 {
		entry := &model.WalletEntry{Email: "alice", Currency: "USD", Amount: 100, Kind: model.WalletReferral, Ref: &ref}
		if err := database.GetDB().Transaction(func(tx *gorm.DB) error { return bookOnce(tx, entry) }); err != nil {
			t.Fatalf("bookOnce: %v", err)
		}
	}
	if got := balanceOf(t, "alice", "USD"); got != 100 {
		t.Fatalf("balance = %d, want the entry booked once", got)
	}
}
