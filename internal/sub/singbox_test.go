package sub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	wgutil "github.com/mhsanaei/3x-ui/v3/internal/util/wireguard"
)

func seedProtoInbound(t *testing.T, subID, tag string, port int, protocol model.Protocol, settings, stream string, rec model.ClientRecord) {
	t.Helper()
	db := database.GetDB()
	ib := &model.Inbound{UserId: 1, Tag: tag, Enable: true, Listen: "203.0.113.5", Port: port,
		Protocol: protocol, Remark: tag, Settings: settings, StreamSettings: stream}
	if err := db.Create(ib).Error; err != nil {
		t.Fatalf("seed inbound %s: %v", tag, err)
	}
	rec.SubID, rec.Enable = subID, true
	if err := db.Create(&rec).Error; err != nil {
		t.Fatalf("seed client %s: %v", rec.Email, err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: ib.Id, FlowOverride: rec.Flow}).Error; err != nil {
		t.Fatalf("seed client_inbound %s: %v", tag, err)
	}
}

func seedSingboxSub(t *testing.T, subID string) {
	t.Helper()
	seedSubInbound(t, subID, "vws", 10001, 1, wsTLSStream)
	seedProtoInbound(t, subID, "vreal", 10002, model.VLESS,
		fmt.Sprintf(`{"clients":[{"id":"aaaaaaaa-0000-4000-8000-000000000002","email":"vreal@e","subId":%q,"flow":"xtls-rprx-vision","enable":true}],"decryption":"none"}`, subID),
		`{"network":"tcp","security":"reality","realitySettings":{"serverNames":["real.sni"],"shortIds":["abcd"],"settings":{"publicKey":"Z84J2IelR9ch3k8VtlVhhs5ycBUlXA7wHBWcBrjqnAw","fingerprint":"firefox"}}}`,
		model.ClientRecord{Email: "vreal@e", UUID: "aaaaaaaa-0000-4000-8000-000000000002", Flow: "xtls-rprx-vision"})
	seedProtoInbound(t, subID, "vx", 10003, model.VLESS,
		fmt.Sprintf(`{"clients":[{"id":"aaaaaaaa-0000-4000-8000-000000000003","email":"vx@e","subId":%q,"enable":true}],"decryption":"none"}`, subID),
		xhttpRealityStream, model.ClientRecord{Email: "vx@e", UUID: "aaaaaaaa-0000-4000-8000-000000000003"})
	seedProtoInbound(t, subID, "tro", 10004, model.Trojan,
		fmt.Sprintf(`{"clients":[{"password":"trojanpw","email":"tro@e","subId":%q,"enable":true}]}`, subID),
		`{"network":"grpc","security":"tls","grpcSettings":{"serviceName":"svc"},"tlsSettings":{"serverName":"t.sni"}}`,
		model.ClientRecord{Email: "tro@e", Password: "trojanpw"})
	seedProtoInbound(t, subID, "ss", 10005, model.Shadowsocks,
		fmt.Sprintf(`{"method":"2022-blake3-aes-128-gcm","password":"AAAAAAAAAAAAAAAAAAAAAA==","clients":[{"password":"BBBBBBBBBBBBBBBBBBBBBB==","email":"ss@e","subId":%q,"enable":true}]}`, subID),
		`{"network":"tcp","security":"none"}`, model.ClientRecord{Email: "ss@e", Password: "BBBBBBBBBBBBBBBBBBBBBB=="})
	seedProtoInbound(t, subID, "hy", 10006, model.Hysteria,
		fmt.Sprintf(`{"version":2,"clients":[{"auth":"hyauth","email":"hy@e","subId":%q,"enable":true}]}`, subID),
		`{"network":"hysteria","security":"tls","tlsSettings":{"serverName":"hy.sni","alpn":["h3"]},"finalmask":{"udp":[{"type":"salamander","settings":{"password":"obfspw"}},{"type":"udphop","settings":{"remotePorts":"20000-20010"}}]}}`,
		model.ClientRecord{Email: "hy@e", Auth: "hyauth"})
	serverPriv, _, err := wgutil.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	clientPriv, _, err := wgutil.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	seedProtoInbound(t, subID, "wg", 10007, model.WireGuard,
		fmt.Sprintf(`{"secretKey":%q,"mtu":1420,"clients":[{"email":"wg@e","subId":%q,"privateKey":%q,"allowedIPs":["10.0.0.2/32","fd00::2/128"],"keepAlive":25,"enable":true}]}`, serverPriv, subID, clientPriv),
		`{}`, model.ClientRecord{Email: "wg@e", PrivateKey: clientPriv, AllowedIPs: "10.0.0.2/32,fd00::2/128"})
}

func byPort(t *testing.T, outbounds []map[string]any, port int) map[string]any {
	t.Helper()
	for _, o := range outbounds {
		if p, _ := o["server_port"].(float64); int(p) == port {
			return o
		}
	}
	return nil
}

func TestSingboxProfileCarriesEveryRepresentableNode(t *testing.T) {
	initSubDB(t)
	seedSingboxSub(t, "sb1")
	w := fetchClashSub(t, clashSubRouter(t), "/singbox/sb1")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("status %d, type %q: %s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	var config struct {
		Outbounds []map[string]any `json:"outbounds"`
		Endpoints []map[string]any `json:"endpoints"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &config); err != nil {
		t.Fatal(err)
	}
	if byPort(t, config.Outbounds, 10003) != nil {
		t.Fatal("the xhttp node reached the profile; sing-box has no xhttp transport")
	}
	selector, auto := config.Outbounds[0], config.Outbounds[1]
	if selector["tag"] != "proxy" || selector["default"] != "auto" || auto["type"] != "urltest" || len(auto["outbounds"].([]any)) != 6 {
		t.Fatalf("groups = %v / %v; want a selector defaulting to a url-test over the 6 nodes", selector, auto)
	}
	if len(config.Endpoints) != 1 || config.Endpoints[0]["type"] != "wireguard" ||
		fmt.Sprint(config.Endpoints[0]["address"]) != "[10.0.0.2/32 fd00::2/128]" {
		t.Fatalf("endpoints = %v; want the WireGuard node as an endpoint with both tunnel addresses", config.Endpoints)
	}

	checks := []struct {
		port int
		want map[string]any
	}{
		{10001, map[string]any{"type": "vless", "transport": map[string]any{"type": "ws", "path": "/base", "headers": map[string]any{"Host": "base.host"}},
			"tls": map[string]any{"enabled": true, "server_name": "base.sni"}}},
		{10002, map[string]any{"type": "vless", "flow": "xtls-rprx-vision", "tls": map[string]any{"enabled": true, "server_name": "real.sni",
			"reality": map[string]any{"enabled": true, "public_key": "Z84J2IelR9ch3k8VtlVhhs5ycBUlXA7wHBWcBrjqnAw", "short_id": "abcd"}, "utls": map[string]any{"enabled": true, "fingerprint": "firefox"}}}},
		{10004, map[string]any{"type": "trojan", "password": "trojanpw", "transport": map[string]any{"type": "grpc", "service_name": "svc"}}},
		{10005, map[string]any{"type": "shadowsocks", "method": "2022-blake3-aes-128-gcm", "password": "AAAAAAAAAAAAAAAAAAAAAA==:BBBBBBBBBBBBBBBBBBBBBB=="}},
		{10006, map[string]any{"type": "hysteria2", "password": "hyauth", "obfs": map[string]any{"type": "salamander", "password": "obfspw"},
			"server_ports": []any{"20000:20010"}, "tls": map[string]any{"enabled": true, "server_name": "hy.sni", "alpn": []any{"h3"}}}},
	}
	for _, c := range checks {
		got := byPort(t, config.Outbounds, c.port)
		if got == nil {
			t.Fatalf("no outbound for port %d", c.port)
		}
		for key, want := range c.want {
			wantJSON, _ := json.Marshal(want)
			gotJSON, _ := json.Marshal(got[key])
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("port %d %s = %s, want %s", c.port, key, gotJSON, wantJSON)
			}
		}
	}

	if bin := os.Getenv("SINGBOX_BINARY"); bin != "" {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, w.Body.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(bin, "check", "-c", path).CombinedOutput(); err != nil {
			t.Fatalf("sing-box check rejected the profile: %v\n%s", err, out)
		}
	}
}

func TestSingboxWithNothingRepresentableIsRefused(t *testing.T) {
	initSubDB(t)
	seedProtoInbound(t, "sb2", "only-xhttp", 10010, model.VLESS,
		`{"clients":[{"id":"aaaaaaaa-0000-4000-8000-000000000010","email":"x@e","subId":"sb2","enable":true}],"decryption":"none"}`,
		xhttpRealityStream, model.ClientRecord{Email: "x@e", UUID: "aaaaaaaa-0000-4000-8000-000000000010"})
	w := fetchClashSub(t, clashSubRouter(t), "/singbox/sb2")
	if w.Code != http.StatusUnprocessableEntity || !slices.Contains([]string{errNoSingboxProxies.Error()}, w.Body.String()) {
		t.Fatalf("status %d body %q; want 422 explaining sing-box cannot carry the nodes", w.Code, w.Body.String())
	}
}

// The subscription page offers the sing-box profile next to the Clash one.
func TestSubPageListsSingboxURL(t *testing.T) {
	initSubDB(t)
	seedSingboxSub(t, "sb3")
	w := fetchClashSub(t, clashSubRouter(t), "/sub/sb3?format=info")
	var info map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	if got := info["subSingboxUrl"]; got != "http://sub.example.com:2096/singbox/sb3" {
		t.Fatalf("subSingboxUrl = %v; body %s", got, w.Body.String())
	}
}
