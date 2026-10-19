package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
)

func joinRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	r := gin.New()
	NewNodeJoinController(r.Group("/base"))
	return r
}

func postJoin(r *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/base/join/node", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.7:4000"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestJoinEndpointRefusesAnUnknownToken(t *testing.T) {
	r := joinRouter(t)
	w := postJoin(r, `{"token":"nope","name":"n1","address":"203.0.113.9","port":2053,"apiToken":"x"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"success":false`) || !strings.Contains(w.Body.String(), "invalid or expired join token") {
		t.Fatalf("response = %d %s", w.Code, w.Body.String())
	}
}

func TestJoinEndpointRejectsMalformedAndOversizedBodies(t *testing.T) {
	r := joinRouter(t)
	if w := postJoin(r, `not json`); w.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: %d", w.Code)
	}
	huge := `{"token":"` + strings.Repeat("a", 20<<10) + `"}`
	if w := postJoin(r, huge); w.Code != http.StatusBadRequest {
		t.Fatalf("oversized body: %d, want 400", w.Code)
	}
}

func TestJoinEndpointThrottlesGuessing(t *testing.T) {
	r := joinRouter(t)
	var last int
	for range joinLimit + 3 {
		last = postJoin(r, `{"token":"guess"}`).Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("attempt %d answered %d, want 429", joinLimit+3, last)
	}
}
