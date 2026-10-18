package service

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func ptrI64(v int64) *int64 { return &v }

func mkReseller(t *testing.T, name string, perGB, perDay int64, credit int64) *model.User {
	t.Helper()
	u := &model.User{Username: name, Password: "x", Role: model.AdminRoleOperator, ScopeInboundIds: "[]", ScopeGroups: "[]", PricePerGB: ptrI64(perGB), PricePerDay: ptrI64(perDay)}
	if err := database.GetDB().Create(u).Error; err != nil {
		t.Fatalf("create reseller: %v", err)
	}
	if credit != 0 {
		if _, err := (&ResellerService{}).Topup(u.Id, credit, "seed"); err != nil {
			t.Fatalf("topup: %v", err)
		}
	}
	return u
}

func seedResellerClient(t *testing.T, email string, total, expiry int64) {
	t.Helper()
	rec := &model.ClientRecord{Email: email, SubID: email, TotalGB: total, ExpiryTime: expiry, Enable: true}
	if err := database.GetDB().Create(rec).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}
}

func resellerBalance(t *testing.T, s *ResellerService, id int) int64 {
	t.Helper()
	b, err := s.Balance(id)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	return b
}

func resellerInDays(n int) int64 {
	return time.Now().Add(time.Duration(n)*24*time.Hour + time.Hour).UnixMilli()
}

func TestResellerCreateIsDebitedByPriceList(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	u := mkReseller(t, "r1", 100, 10, 10000)

	res, err := s.Reserve(u, []TermsChange{{Email: "c1", After: ClientTerms{TotalGB: 2 * resellerGiB, ExpiryTime: resellerInDays(30)}}})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if got := resellerBalance(t, s, u.Id); got != 10000-(2*100+31*10) {
		t.Fatalf("balance after reserve = %d, want %d", got, 10000-(2*100+31*10))
	}
	seedResellerClient(t, "c1", 2*resellerGiB, resellerInDays(30))
	if err := res.Settle(); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if got := resellerBalance(t, s, u.Id); got != 10000-(2*100+31*10) {
		t.Fatalf("balance after settle = %d, want the create kept", got)
	}
	var entry model.ResellerEntry
	if err := database.GetDB().Where("kind = ?", model.ResellerCreate).First(&entry).Error; err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if entry.ClientEmail != "c1" || entry.Bytes != 2*resellerGiB || entry.Days != 31 {
		t.Fatalf("entry = %+v, want c1 with 2 GiB and 31 days", entry)
	}
}

func TestResellerCreateThatNeverHappenedIsRefunded(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	u := mkReseller(t, "r1", 100, 10, 10000)

	res, err := s.Reserve(u, []TermsChange{{Email: "ghost", After: ClientTerms{TotalGB: resellerGiB, ExpiryTime: resellerInDays(10)}}})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := res.Settle(); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if got := resellerBalance(t, s, u.Id); got != 10000 {
		t.Fatalf("balance = %d, want the full 10000 back", got)
	}
	var n int64
	database.GetDB().Model(&model.ResellerEntry{}).Where("kind = ?", model.ResellerCreate).Count(&n)
	if n != 0 {
		t.Fatalf("%d create entries left for a client that was never created", n)
	}
}

func TestResellerRenewalChargesOnlyTheExtension(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	u := mkReseller(t, "r1", 100, 10, 10000)
	seedResellerClient(t, "c1", resellerGiB, resellerInDays(10))

	res, err := s.Reserve(u, []TermsChange{{Email: "c1", After: ClientTerms{TotalGB: 3 * resellerGiB, ExpiryTime: resellerInDays(40)}}})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "c1").Updates(map[string]any{"total_gb": 3 * resellerGiB, "expiry_time": resellerInDays(40)})
	if err := res.Settle(); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if got, want := resellerBalance(t, s, u.Id), int64(10000-(2*100+30*10)); got != want {
		t.Fatalf("balance = %d, want %d (2 GiB and 30 days extra)", got, want)
	}
}

func TestResellerSettleBillsWhatWasActuallyApplied(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	u := mkReseller(t, "r1", 100, 10, 10000)
	seedResellerClient(t, "c1", resellerGiB, resellerInDays(10))

	res, err := s.Reserve(u, []TermsChange{{Email: "c1", After: ClientTerms{TotalGB: 3 * resellerGiB, ExpiryTime: resellerInDays(40)}}})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "c1").Update("total_gb", 2*resellerGiB)
	if err := res.Settle(); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if got, want := resellerBalance(t, s, u.Id), int64(10000-100); got != want {
		t.Fatalf("balance = %d, want %d (only the extra GiB applied)", got, want)
	}
}

func TestResellerRefusesWhenCreditIsShort(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	u := mkReseller(t, "r1", 100, 10, 100)

	_, err := s.Reserve(u, []TermsChange{{Email: "c1", After: ClientTerms{TotalGB: 2 * resellerGiB, ExpiryTime: resellerInDays(30)}}})
	if !errors.Is(err, ErrResellerInsufficient) {
		t.Fatalf("err = %v, want ErrResellerInsufficient", err)
	}
	if got := resellerBalance(t, s, u.Id); got != 100 {
		t.Fatalf("balance = %d, want 100 untouched", got)
	}
}

func TestResellerCannotGrantUnlimitedWhenPriced(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	u := mkReseller(t, "r1", 100, 10, 100000)

	for name, after := range map[string]ClientTerms{
		"unlimited traffic": {TotalGB: 0, ExpiryTime: resellerInDays(30)},
		"no expiry":         {TotalGB: resellerGiB, ExpiryTime: 0},
	} {
		if _, err := s.Reserve(u, []TermsChange{{Email: "c1", After: after}}); !errors.Is(err, ErrResellerUnpriced) {
			t.Fatalf("%s: err = %v, want ErrResellerUnpriced", name, err)
		}
	}
}

func TestResellerUnlimitedIsFreeWhenThatDimensionHasNoPrice(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	u := mkReseller(t, "r1", 0, 10, 100000)

	res, err := s.Reserve(u, []TermsChange{{Email: "c1", After: ClientTerms{TotalGB: 0, ExpiryTime: resellerInDays(5)}}})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if got, want := resellerBalance(t, s, u.Id), int64(100000-6*10); got != want {
		t.Fatalf("balance = %d, want %d", got, want)
	}
	_ = res
}

func TestResellerDelayedStartIsBilledByItsDuration(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	u := mkReseller(t, "r1", 0, 10, 1000)

	if _, err := s.Reserve(u, []TermsChange{{Email: "c1", After: ClientTerms{TotalGB: resellerGiB, ExpiryTime: -30 * 24 * 3600 * 1000}}}); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if got, want := resellerBalance(t, s, u.Id), int64(1000-300); got != want {
		t.Fatalf("balance = %d, want %d", got, want)
	}
}

func TestResellerOtherRolesAreNotBilled(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	owner := &model.User{Username: "boss", Role: model.AdminRoleOwner, PricePerGB: ptrI64(100)}

	res, err := s.Reserve(owner, []TermsChange{{Email: "c1", After: ClientTerms{TotalGB: resellerGiB}}})
	if err != nil || res != nil {
		t.Fatalf("Reserve(owner) = %v, %v; want nil, nil", res, err)
	}
	if err := res.Settle(); err != nil {
		t.Fatalf("Settle on a nil reservation: %v", err)
	}
}

func TestResellerWithoutPricesIsNotCharged(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	u := &model.User{Username: "free", Password: "x", Role: model.AdminRoleOperator, ScopeInboundIds: "[]", ScopeGroups: "[]"}
	if err := database.GetDB().Create(u).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reserve(u, []TermsChange{{Email: "c1", After: ClientTerms{}}}); err != nil {
		t.Fatalf("reserve with no prices and no credit: %v", err)
	}
}

func TestResellerChargeDoesNotOverflowOnHugeQuota(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	u := mkReseller(t, "r1", 1<<50, 0, 100)

	_, err := s.Reserve(u, []TermsChange{{Email: "c1", After: ClientTerms{TotalGB: 1 << 50, ExpiryTime: 0}}})
	if !errors.Is(err, ErrResellerInsufficient) {
		t.Fatalf("err = %v, want ErrResellerInsufficient rather than an overflowed price", err)
	}
}

func TestResellerReportSumsSalesPerReseller(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	a := mkReseller(t, "alpha", 100, 10, 10000)
	b := mkReseller(t, "beta", 100, 10, 10000)
	seedResellerClient(t, "a1", resellerGiB, resellerInDays(10))
	for _, tc := range []struct {
		u     *model.User
		email string
	}{{a, "a1"}, {a, "a2"}, {b, "b1"}} {
		after := ClientTerms{TotalGB: resellerGiB, ExpiryTime: resellerInDays(10)}
		if tc.email == "a1" {
			after = ClientTerms{TotalGB: 2 * resellerGiB, ExpiryTime: resellerInDays(20)}
		}
		res, err := s.Reserve(tc.u, []TermsChange{{Email: tc.email, After: after}})
		if err != nil {
			t.Fatal(err)
		}
		if tc.email == "a1" {
			database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", "a1").Updates(map[string]any{"total_gb": 2 * resellerGiB, "expiry_time": resellerInDays(20)})
		} else {
			seedResellerClient(t, tc.email, resellerGiB, resellerInDays(10))
		}
		if err := res.Settle(); err != nil {
			t.Fatal(err)
		}
	}

	rep, err := s.Report(0, 0, nil)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	byName := map[string]ResellerReportRow{}
	for _, r := range rep.Rows {
		byName[r.Username] = r
	}
	got := byName["alpha"]
	if got.Created != 1 || got.Renewed != 1 || got.ToppedUp != 10000 {
		t.Fatalf("alpha = %+v, want 1 created, 1 renewed, 10000 topped up", got)
	}
	if got.Spent != (100+11*10)+(100+10*10) {
		t.Fatalf("alpha spent = %d, want %d", got.Spent, (100+11*10)+(100+10*10))
	}
	if only, err := s.Report(0, 0, &b.Id); err != nil || len(only.Rows) != 1 || only.Rows[0].Username != "beta" {
		t.Fatalf("report for beta only = %+v, %v", only, err)
	}
}

func TestResellerTopupMayNotOverdraw(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	u := mkReseller(t, "r1", 0, 0, 500)

	if _, err := s.Topup(u.Id, -600, "too much"); !errors.Is(err, ErrResellerInsufficient) {
		t.Fatalf("err = %v, want ErrResellerInsufficient", err)
	}
	if _, err := s.Topup(u.Id, -200, "ok"); err != nil {
		t.Fatalf("withdraw 200: %v", err)
	}
	if got := resellerBalance(t, s, u.Id); got != 300 {
		t.Fatalf("balance = %d, want 300", got)
	}
}

func TestResellerWithoutOwnPricesFollowsThePanelList(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	if err := s.SaveConfig(ResellerConfig{PricePerGB: 50, PricePerDay: 5}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	followers := &model.User{Username: "follow", Password: "x", Role: model.AdminRoleOperator, ScopeInboundIds: "[]", ScopeGroups: "[]"}
	own := mkReseller(t, "own", 0, 0, 1000)
	if err := database.GetDB().Create(followers).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Topup(followers.Id, 1000, ""); err != nil {
		t.Fatal(err)
	}
	change := []TermsChange{{Email: "c1", After: ClientTerms{TotalGB: resellerGiB, ExpiryTime: resellerInDays(9)}}}

	if _, err := s.Reserve(followers, change); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if got, want := resellerBalance(t, s, followers.Id), int64(1000-(50+10*5)); got != want {
		t.Fatalf("follower balance = %d, want %d from the panel list", got, want)
	}
	if _, err := s.Reserve(own, change); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if got := resellerBalance(t, s, own.Id); got != 1000 {
		t.Fatalf("balance = %d: an explicit price of 0 must override the panel list", got)
	}
}

func TestResellerConfigRejectsNegativePrices(t *testing.T) {
	setupBulkDB(t)
	s := &ResellerService{}
	for _, cfg := range []ResellerConfig{{PricePerGB: -1}, {PricePerDay: -1}, {LowCredit: -1}} {
		if err := s.SaveConfig(cfg); err == nil {
			t.Fatalf("SaveConfig(%+v) accepted a negative value", cfg)
		}
	}
}

func collectResellerEvents(t *testing.T) *[]eventbus.Event {
	t.Helper()
	bus := eventbus.New(0)
	got := &[]eventbus.Event{}
	var mu sync.Mutex
	bus.Subscribe("reseller-test", func(e eventbus.Event) {
		mu.Lock()
		*got = append(*got, e)
		mu.Unlock()
	})
	SetEventBus(bus)
	t.Cleanup(func() {
		SetEventBus(nil)
		bus.Stop()
	})
	return got
}

func TestResellerLowCreditAlertFiresOnceWhenCreditCrossesTheThreshold(t *testing.T) {
	setupBulkDB(t)
	events := collectResellerEvents(t)
	s := &ResellerService{}
	if err := s.SaveConfig(ResellerConfig{LowCredit: 1000}); err != nil {
		t.Fatal(err)
	}
	u := mkReseller(t, "shop", 0, 10, 1500)
	spend := func(email string, days int) {
		t.Helper()
		res, err := s.Reserve(u, []TermsChange{{Email: email, After: ClientTerms{ExpiryTime: resellerInDays(days - 1)}}})
		if err != nil {
			t.Fatal(err)
		}
		seedResellerClient(t, email, 0, resellerInDays(days-1))
		if err := res.Settle(); err != nil {
			t.Fatal(err)
		}
	}

	spend("a", 30)
	spend("b", 25)
	spend("c", 5)
	time.Sleep(100 * time.Millisecond)
	if len(*events) != 1 {
		t.Fatalf("%d events, want exactly one for the crossing", len(*events))
	}
	e := (*events)[0]
	data, ok := e.Data.(*eventbus.ResellerCreditData)
	if e.Type != eventbus.EventResellerCreditLow || e.Source != "shop" || !ok || data.Balance != 1500-300-250 || data.Threshold != 1000 {
		t.Fatalf("event = %+v (%+v), want reseller.credit_low for shop at balance 950, below 1000", e, data)
	}
}

func TestResellerBulkAdjustIsBilledOnlyForClientsThatGainedTime(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}
	s := &ResellerService{}
	u := mkReseller(t, "r1", 100, 10, 10000)

	base := resellerInDays(30)
	clients := []model.Client{
		{Email: "paid@x", ID: "11111111-1111-1111-1111-111111111111", SubID: "p", Enable: true, ExpiryTime: base, TotalGB: 10 * resellerGiB},
		{Email: "forever@x", ID: "22222222-2222-2222-2222-222222222222", SubID: "f", Enable: true},
	}
	ib := mkInboundStream(t, 30301, model.VLESS, clientsSettings(t, clients), wsStream)
	if err := svc.SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for _, c := range clients {
		database.GetDB().Create(&xray.ClientTraffic{Email: c.Email, Enable: true, ExpiryTime: c.ExpiryTime, Total: c.TotalGB})
	}

	emails := []string{"paid@x", "forever@x", "missing@x"}
	changes, err := s.PlanAdjust(emails, 7, resellerGiB)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	res, err := s.Reserve(u, changes)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, _, err := svc.BulkAdjust(inboundSvc, emails, 7, resellerGiB, "", nil, ""); err != nil {
		t.Fatalf("BulkAdjust: %v", err)
	}
	if err := res.Settle(); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if got, want := resellerBalance(t, s, u.Id), int64(10000-(100+7*10)); got != want {
		t.Fatalf("balance = %d, want %d: only paid@x gained a GiB and 7 days", got, want)
	}
}
