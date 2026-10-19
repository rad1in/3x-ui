package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func speedtestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	a := &ServerController{}
	engine.GET("/download", a.speedtestDownload)
	engine.POST("/upload", a.speedtestUpload)
	return engine
}

func TestSpeedtestDownloadServesExactlyTheRequestedBytes(t *testing.T) {
	engine := speedtestEngine()
	cases := []struct {
		query string
		code  int
		size  int
	}{
		{"?bytes=1000", http.StatusOK, 1000},
		{"?bytes=0", http.StatusBadRequest, 0},
		{"?bytes=abc", http.StatusBadRequest, 0},
		{"", http.StatusBadRequest, 0},
		{"?bytes=" + "67108865", http.StatusBadRequest, 0}, // one over SpeedTestMaxBytes
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/download"+tc.query, nil))
		if rec.Code != tc.code || rec.Body.Len() != tc.size {
			t.Errorf("%q: status %d with %d bytes, want %d with %d", tc.query, rec.Code, rec.Body.Len(), tc.code, tc.size)
		}
	}

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/download?bytes=4096", nil))
	if bytes.Equal(rec.Body.Bytes(), make([]byte, 4096)) {
		t.Error("payload is all zeros, which a compressing proxy would shrink and inflate the measured speed")
	}
}

func TestSpeedtestUploadRefusesOversizedBodies(t *testing.T) {
	engine := speedtestEngine()
	post := func(n int64) int {
		req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(strings.Repeat("x", int(n))))
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := post(2048); code != http.StatusOK {
		t.Errorf("small upload answered %d, want 200", code)
	}
	if code := post(service.SpeedTestMaxBytes + 1); code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized upload answered %d, want 413", code)
	}
}
