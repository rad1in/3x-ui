package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func signedLinkRequest(t *testing.T, subID, body string) (*entity.Msg, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "subId", Value: subID}}
	c.Request = httptest.NewRequest(http.MethodPost, "http://panel.example.com/", bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	(&ClientController{}).signedSubLink(c)
	var msg entity.Msg
	if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	return &msg, w
}

func TestSignedSubLinkEndpointIssuesLinksOnlyWhileSigningIsOn(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))

	if msg, _ := signedLinkRequest(t, "sub-1", `{"ttlSeconds": 60}`); msg.Success {
		t.Fatal("a link was issued while signing is off")
	}
	if err := (&service.SecurityConfigService{}).Save(&service.SecurityConfig{SubSigningEnable: true}); err != nil {
		t.Fatal(err)
	}
	msg, w := signedLinkRequest(t, "sub-1", `{"ttlSeconds": 60}`)
	if !msg.Success {
		t.Fatalf("issuing failed: %s", w.Body.String())
	}
	link := msg.Obj.(map[string]any)["url"].(string)
	u, err := url.Parse(link)
	if err != nil || !strings.HasSuffix(u.Path, "/sub-1") || u.Query().Get("sig") == "" || u.Query().Get("exp") == "0" {
		t.Fatalf("link %q lacks the id, signature or expiry", link)
	}
	if msg, _ := signedLinkRequest(t, "sub-1", `{"ttlSeconds": -5}`); msg.Success {
		t.Fatal("a negative lifetime was accepted")
	}
}
