package service

import (
	"strings"
	"testing"
)

func TestSalesConfigDefaultsOffAndNormalizesCardNumber(t *testing.T) {
	setupBulkDB(t)
	svc := &SalesConfigService{}

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get on a fresh panel: %v", err)
	}
	if got.Card.Enabled || got.PublicEnabled {
		t.Fatalf("fresh config = %+v, want everything off", got)
	}

	in := SalesConfig{Card: CardPayConfig{Enabled: true, CardNumber: "6037 ۹۹۱۸-١٢٣٤ 5678", HolderName: " Ali "}}
	if err := svc.Save(&in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err = svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Card.CardNumber != "6037991812345678" || got.Card.HolderName != "Ali" {
		t.Fatalf("card = %+v, want digits only and trimmed holder", got.Card)
	}
}

func TestSalesConfigRejectsEnabledCardWithoutValidNumber(t *testing.T) {
	setupBulkDB(t)
	svc := &SalesConfigService{}
	for _, number := range []string{"", "1234", "6037-9918-1234-56ab"} {
		err := svc.Save(&SalesConfig{Card: CardPayConfig{Enabled: true, CardNumber: number}})
		if err == nil || !strings.Contains(err.Error(), "card number") {
			t.Fatalf("Save(card %q) error = %v, want a card number error", number, err)
		}
	}
	if err := svc.Save(&SalesConfig{Card: CardPayConfig{Enabled: false, CardNumber: "junk"}}); err != nil {
		t.Fatalf("a disabled card is not validated: %v", err)
	}
}
