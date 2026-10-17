package sub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func seedOutlineSub(t *testing.T, subID string) map[string]int {
	t.Helper()
	seedProtoInbound(t, subID, "ss-aead", 11001, model.Shadowsocks,
		fmt.Sprintf(`{"method":"chacha20-poly1305","network":"tcp,udp","clients":[{"password":"aeadpw","email":"aead@e","subId":%q,"enable":true}]}`, subID),
		`{"network":"tcp","security":"none"}`, model.ClientRecord{Email: "aead@e", Password: "aeadpw"})
	seedProtoInbound(t, subID, "ss-2022", 11002, model.Shadowsocks,
		fmt.Sprintf(`{"method":"2022-blake3-aes-128-gcm","password":"AAAAAAAAAAAAAAAAAAAAAA==","clients":[{"password":"BBBBBBBBBBBBBBBBBBBBBB==","email":"s22@e","subId":%q,"enable":true}]}`, subID),
		`{"network":"tcp","security":"none"}`, model.ClientRecord{Email: "s22@e", Password: "BBBBBBBBBBBBBBBBBBBBBB=="})
	seedProtoInbound(t, subID, "ss-udp", 11003, model.Shadowsocks,
		fmt.Sprintf(`{"method":"aes-256-gcm","network":"udp","clients":[{"password":"udppw","email":"udp@e","subId":%q,"enable":true}]}`, subID),
		`{"network":"tcp","security":"none"}`, model.ClientRecord{Email: "udp@e", Password: "udppw"})
	seedSubInbound(t, subID, "vl", 11004, 1, `{"network":"tcp","security":"none"}`)
	ids := map[string]int{}
	var inbounds []model.Inbound
	if err := database.GetDB().Find(&inbounds).Error; err != nil {
		t.Fatal(err)
	}
	for _, ib := range inbounds {
		ids[ib.Tag] = ib.Id
	}
	return ids
}

func TestOutlineDynamicAccessKey(t *testing.T) {
	seedSubDB(t)
	ids := seedOutlineSub(t, "s1")
	gin.SetMode(gin.TestMode)
	router := newSubscriptionTestRouter(subscriptionTestRouterConfig{})
	get := func(subID string, inbound int) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("https://sub.example.com/sub/%s/outline/%d", subID, inbound), nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp
	}

	t.Run("AEAD inbound serves the Outline key JSON", func(t *testing.T) {
		resp := get("s1", ids["ss-aead"])
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
		}
		if got := resp.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Fatalf("Content-Type = %q", got)
		}
		var key map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &key); err != nil {
			t.Fatalf("body is not JSON: %v\n%s", err, resp.Body.String())
		}
		want := map[string]any{"server": "203.0.113.5", "server_port": float64(11001), "password": "aeadpw", "method": "chacha20-ietf-poly1305"}
		if len(key) != len(want) {
			t.Fatalf("key = %v, want exactly %v", key, want)
		}
		for k, v := range want {
			if key[k] != v {
				t.Fatalf("key[%q] = %v, want %v (key %v)", k, key[k], v, key)
			}
		}
	})

	for name, tc := range map[string]struct {
		subID   string
		inbound int
	}{
		"Shadowsocks 2022, which Outline cannot speak":     {"s1", ids["ss-2022"]},
		"a UDP-only inbound Outline cannot reach over TCP": {"s1", ids["ss-udp"]},
		"a VLESS inbound":            {"s1", ids["vl"]},
		"another subscription's key": {"other", ids["ss-aead"]},
	} {
		t.Run(name+" is 404", func(t *testing.T) {
			if resp := get(tc.subID, tc.inbound); resp.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404; body=%s", resp.Code, resp.Body.String())
			}
		})
	}
}

func TestOutlineKeysOnSubPage(t *testing.T) {
	seedSubDB(t)
	ids := seedOutlineSub(t, "s1")
	gin.SetMode(gin.TestMode)
	keys := func(t *testing.T) []OutlineKey {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "https://sub.example.com/sub/s1?format=info", nil)
		resp := httptest.NewRecorder()
		newSubscriptionTestRouter(subscriptionTestRouterConfig{}).ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d; body=%s", resp.Code, resp.Body.String())
		}
		var info struct {
			Outline []OutlineKey `json:"outline"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &info); err != nil {
			t.Fatal(err)
		}
		return info.Outline
	}

	if got := keys(t); len(got) != 0 {
		t.Fatalf("a plain-HTTP subscription URL must offer no ssconf key (Outline fetches over HTTPS only), got %+v", got)
	}

	if err := database.GetDB().Create(&model.Setting{Key: "subURI", Value: "https://sub.example.com/sub/"}).Error; err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("ssconf://sub.example.com/sub/s1/outline/%d#ss-aead", ids["ss-aead"])
	if got := keys(t); len(got) != 1 || got[0].URL != want || got[0].Remark != "ss-aead" {
		t.Fatalf("outline = %+v, want one key %q", got, want)
	}
}
