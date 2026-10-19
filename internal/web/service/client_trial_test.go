package service

import (
	"errors"
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func trialTemplate(t *testing.T, svc *ClientService, port int) *model.ClientTemplate {
	t.Helper()
	ib := mkInbound(t, port, model.VLESS, `{"clients":[]}`)
	tpl, err := svc.SaveClientTemplate(&model.ClientTemplate{
		Name: "trial", TotalGB: 1 << 30, ExpiryDays: 1, LimitIP: 1, InboundIds: []int{ib.Id},
	})
	if err != nil {
		t.Fatal(err)
	}
	return tpl
}

func TestCreateTrialClientBindsTelegramIDAndAppliesTemplate(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	tpl := trialTemplate(t, svc, 25001)

	email, _, err := svc.CreateTrialClient(&InboundService{}, tpl.Id, 777001)
	if err != nil {
		t.Fatalf("CreateTrialClient: %v", err)
	}
	if email != "trial-777001" {
		t.Fatalf("email = %q, want trial-777001", email)
	}
	rec := lookupClientRecord(t, email)
	if rec.TgID != 777001 || rec.TotalGB != 1<<30 || rec.LimitIP != 1 || rec.ExpiryTime <= 0 {
		t.Fatalf("record = %+v, want tg 777001, 1 GiB, 1 IP, dated expiry", rec)
	}
}

func TestCreateTrialClientGrantsOncePerTelegramIDEvenAfterDeletion(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}
	tpl := trialTemplate(t, svc, 25002)

	email, _, err := svc.CreateTrialClient(inboundSvc, tpl.Id, 777002)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CreateTrialClient(inboundSvc, tpl.Id, 777002); !errors.Is(err, ErrTrialAlreadyUsed) {
		t.Fatalf("second trial err = %v, want ErrTrialAlreadyUsed", err)
	}
	if _, err := svc.DeleteByEmail(inboundSvc, email, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CreateTrialClient(inboundSvc, tpl.Id, 777002); !errors.Is(err, ErrTrialAlreadyUsed) {
		t.Fatalf("trial after deleting the client err = %v, want ErrTrialAlreadyUsed", err)
	}
	if _, _, err := svc.CreateTrialClient(inboundSvc, tpl.Id, 777003); err != nil {
		t.Fatalf("another telegram id must still get a trial: %v", err)
	}
}

func TestCreateTrialClientConcurrentRequestsCreateOne(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	tpl := trialTemplate(t, svc, 25003)

	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := svc.CreateTrialClient(&InboundService{}, tpl.Id, 777004)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	granted := 0
	for err := range results {
		if err == nil {
			granted++
		}
	}
	var n int64
	database.GetDB().Model(&model.ClientRecord{}).Where("tg_id = ?", 777004).Count(&n)
	if granted != 1 || n != 1 {
		t.Fatalf("granted %d trials and %d clients, want exactly one of each", granted, n)
	}
}

func TestCreateTrialClientFailureReleasesTheGrant(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	tpl := &model.ClientTemplate{Name: "broken", InboundIds: []int{9999}}
	tpl, err := svc.SaveClientTemplate(tpl)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.CreateTrialClient(&InboundService{}, tpl.Id, 777005); err == nil {
		t.Fatal("trial on a missing inbound succeeded")
	}
	var grants int64
	database.GetDB().Model(&model.ClientTrial{}).Where("tg_id = ?", 777005).Count(&grants)
	if grants != 0 {
		t.Fatal("a failed trial consumed the user's one free trial")
	}
}
