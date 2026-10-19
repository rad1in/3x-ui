package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func fakeSpeedNode(t *testing.T, withThroughput bool, uploaded *atomic.Int64) *model.Node {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/panel/api/server/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"obj":{}}`))
	})
	if withThroughput {
		mux.HandleFunc("/panel/api/server/speedtestDownload", func(w http.ResponseWriter, r *http.Request) {
			n, _ := strconv.Atoi(r.URL.Query().Get("bytes"))
			_, _ = w.Write(make([]byte, n))
		})
		mux.HandleFunc("/panel/api/server/speedtestUpload", func(w http.ResponseWriter, r *http.Request) {
			n, _ := io.Copy(io.Discard, r.Body)
			uploaded.Store(n)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"obj":{}}`))
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(u.Port())
	return &model.Node{
		Id: 1, Name: "speed", Scheme: "http", Address: u.Hostname(), Port: port,
		BasePath: "/", Enable: true, AllowPrivateAddress: true, TlsVerifyMode: "skip",
	}
}

func TestSpeedTestMeasuresLatencyAndThroughput(t *testing.T) {
	var uploaded atomic.Int64
	n := fakeSpeedNode(t, true, &uploaded)

	res, err := (&NodeService{}).SpeedTest(context.Background(), n)
	if err != nil {
		t.Fatalf("SpeedTest: %v", err)
	}
	if res.Samples != speedTestPings || res.Lost != 0 {
		t.Fatalf("samples/lost = %d/%d, want %d/0", res.Samples, res.Lost, speedTestPings)
	}
	if res.LatencyMinMs > res.LatencyAvgMs || res.LatencyAvgMs > res.LatencyMaxMs {
		t.Fatalf("latency min/avg/max = %v/%v/%v is not ordered", res.LatencyMinMs, res.LatencyAvgMs, res.LatencyMaxMs)
	}
	if res.DownloadMbps <= 0 || res.UploadMbps <= 0 || res.ThroughputError != "" {
		t.Fatalf("throughput = down %v up %v err %q", res.DownloadMbps, res.UploadMbps, res.ThroughputError)
	}
	if got := uploaded.Load(); got != speedTestUploadBytes {
		t.Fatalf("node received %d upload bytes, want %d", got, speedTestUploadBytes)
	}
}

// A node older than this feature has no speedtest routes; its latency must
// still be reported, with the throughput gap named instead of an error.
func TestSpeedTestWithoutThroughputRoutesStillReportsLatency(t *testing.T) {
	n := fakeSpeedNode(t, false, nil)

	res, err := (&NodeService{}).SpeedTest(context.Background(), n)
	if err != nil {
		t.Fatalf("SpeedTest: %v", err)
	}
	if res.Samples != speedTestPings || res.DownloadMbps != 0 || res.UploadMbps != 0 {
		t.Fatalf("result = %+v, want latency only", res)
	}
	if res.ThroughputError == "" {
		t.Fatal("ThroughputError is empty for a node without speedtest routes")
	}
}
