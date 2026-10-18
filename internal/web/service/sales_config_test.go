package service

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/crypto/nodetoken"
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

func onlineConfig() *SalesConfig {
	return &SalesConfig{
		PublicBaseURL: "https://shop.example.com/",
		Zarinpal:      ZarinpalConfig{Enabled: true, MerchantId: "11111111-2222-3333-4444-555555555555"},
		IDPay:         IDPayConfig{Enabled: true, ApiKey: "idpay-key"},
	}
}

func TestSalesConfigViewNeverCarriesSecretsAndBlankSaveKeepsThem(t *testing.T) {
	setupBulkDB(t)
	svc := &SalesConfigService{}
	if err := svc.Save(onlineConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	view, err := svc.GetView()
	if err != nil {
		t.Fatalf("GetView: %v", err)
	}
	if view.Zarinpal.MerchantId != "" || view.IDPay.ApiKey != "" || !view.HasZarinpalMerchantId || !view.HasIdpayApiKey {
		t.Fatalf("view = %+v, want blank secrets and Has flags set", view)
	}
	if view.PublicBaseURL != "https://shop.example.com" {
		t.Fatalf("base url = %q, want the trailing slash trimmed", view.PublicBaseURL)
	}

	resave := &SalesConfig{PublicBaseURL: "https://shop.example.com", Zarinpal: ZarinpalConfig{Enabled: true}, IDPay: IDPayConfig{Enabled: true, ApiKey: "rotated"}}
	if err := svc.Save(resave); err != nil {
		t.Fatalf("Save with blank merchant id: %v", err)
	}
	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Zarinpal.MerchantId != "11111111-2222-3333-4444-555555555555" || got.IDPay.ApiKey != "rotated" {
		t.Fatalf("secrets = %q / %q, want the kept merchant id and the rotated key", got.Zarinpal.MerchantId, got.IDPay.ApiKey)
	}
}

func TestSalesConfigKeepsOneCryptoSecretWhenTheOtherIsReplaced(t *testing.T) {
	setupBulkDB(t)
	svc := &SalesConfigService{}
	full := &SalesConfig{PublicBaseURL: "https://s.example.com", Crypto: CryptoConfig{Enabled: true, ApiKey: "np-key", IpnSecret: "np-ipn"}}
	if err := svc.Save(full); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rotate := &SalesConfig{PublicBaseURL: "https://s.example.com", Crypto: CryptoConfig{Enabled: true, IpnSecret: "np-ipn-2"}}
	if err := svc.Save(rotate); err != nil {
		t.Fatalf("Save rotate: %v", err)
	}
	got, _ := svc.Get()
	if got.Crypto.ApiKey != "np-key" || got.Crypto.IpnSecret != "np-ipn-2" {
		t.Fatalf("crypto = %+v, want the API key kept and the IPN secret rotated", got.Crypto)
	}
	view, _ := svc.GetView()
	if view.Crypto.ApiKey != "" || view.Crypto.IpnSecret != "" || !view.HasCryptoApiKey || !view.HasCryptoIpnSecret {
		t.Fatalf("view = %+v, want blank secrets with both Has flags", view)
	}
}

func TestSalesConfigClearSecretsEmptiesOnlyTheNamedOne(t *testing.T) {
	setupBulkDB(t)
	svc := &SalesConfigService{}
	if err := svc.Save(onlineConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	next := &SalesConfig{PublicBaseURL: "https://shop.example.com", IDPay: IDPayConfig{Enabled: true}, ClearSecrets: []string{secretZarinpalMerchant}}
	if err := svc.Save(next); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _ := svc.Get()
	if got.Zarinpal.MerchantId != "" || got.IDPay.ApiKey != "idpay-key" || len(got.ClearSecrets) != 0 {
		t.Fatalf("config = %+v, want only the merchant id cleared and no clear list stored", got)
	}
}

func TestSalesConfigRejectsOnlineMethodsThatCannotWork(t *testing.T) {
	setupBulkDB(t)
	svc := &SalesConfigService{}
	cases := []struct {
		name string
		cfg  SalesConfig
		want string
	}{
		{"no base url", SalesConfig{Zarinpal: ZarinpalConfig{Enabled: true, MerchantId: "m"}}, "public base URL"},
		{"base url without scheme", SalesConfig{PublicBaseURL: "shop.example.com", IDPay: IDPayConfig{Enabled: true, ApiKey: "k"}}, "public base URL"},
		{"zarinpal without merchant", SalesConfig{PublicBaseURL: "https://s.example.com", Zarinpal: ZarinpalConfig{Enabled: true}}, "merchant id"},
		{"crypto without ipn secret", SalesConfig{PublicBaseURL: "https://s.example.com", Crypto: CryptoConfig{Enabled: true, ApiKey: "k"}}, "IPN secret"},
		{"idpay without key", SalesConfig{PublicBaseURL: "https://s.example.com", IDPay: IDPayConfig{Enabled: true}}, "API key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.Save(&tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Save error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestSalesConfigSecretsAreEncryptedAtRestWhenEncryptionIsOn(t *testing.T) {
	setupBulkDB(t)
	key := [32]byte{1, 2, 3}
	codec, err := nodetoken.NewCodec(nodetoken.ModeRequired, &nodetoken.Keyring{ActiveID: "k1", Keys: map[string][32]byte{"k1": key}})
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	nodetoken.Init(codec)
	t.Cleanup(func() {
		off, _ := nodetoken.NewCodec(nodetoken.ModeOff, nil)
		nodetoken.Init(off)
	})

	svc := &SalesConfigService{}
	if err := svc.Save(onlineConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := svc.settings.getString(salesConfigKey)
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	if strings.Contains(raw, "11111111-2222") || strings.Contains(raw, "idpay-key") || !strings.Contains(raw, "enc:v1:") {
		t.Fatalf("stored config = %s, want the secrets encrypted", raw)
	}
	got, err := svc.Get()
	if err != nil || got.Zarinpal.MerchantId != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("Get after encryption = %+v, %v; want the merchant id back in the clear", got, err)
	}
}
