package service

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
)

const (
	speedTestPings        = 5
	speedTestDownloadSize = 8 << 20
	speedTestUploadBytes  = 4 << 20
	// SpeedTestMaxBytes is the most one speedtest request may move; the node
	// refuses larger ones so the route cannot be used to exhaust bandwidth.
	SpeedTestMaxBytes = 64 << 20
)

// NodeSpeedResult is one on-demand measurement of the master-to-node path.
type NodeSpeedResult struct {
	NodeId int `json:"nodeId" example:"1"`
	// Samples is how many latency pings answered; Lost how many did not.
	Samples      int     `json:"samples" example:"5"`
	Lost         int     `json:"lost" example:"0"`
	LatencyMinMs float64 `json:"latencyMinMs" example:"38.2"`
	LatencyAvgMs float64 `json:"latencyAvgMs" example:"41.7"`
	LatencyMaxMs float64 `json:"latencyMaxMs" example:"47.1"`
	// JitterMs is the mean absolute difference between consecutive pings.
	JitterMs     float64 `json:"jitterMs" example:"3.1"`
	DownloadMbps float64 `json:"downloadMbps" example:"412.5"`
	UploadMbps   float64 `json:"uploadMbps" example:"188.9"`
	// ThroughputError is set when the latency part worked but the transfer did not,
	// typically a node older than the speedtest routes.
	ThroughputError string `json:"throughputError,omitempty" example:""`
}

func mbps(bytes int64, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return math.Round(float64(bytes)*8/d.Seconds()/1e4) / 100
}

func roundMs(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*100) / 100
}

// SpeedTest pings the node's status route speedTestPings times, then moves a
// fixed payload down and up the same authenticated path the panel syncs over.
func (s *NodeService) SpeedTest(ctx context.Context, n *model.Node) (*NodeSpeedResult, error) {
	proxyURL := ""
	if n.OutboundTag != "" {
		if mgr := runtime.GetManager(); mgr != nil {
			proxyURL = mgr.NodeEgressProxyURL(n.Id)
		}
	}
	res := &NodeSpeedResult{NodeId: n.Id}

	var rtts []time.Duration
	for range speedTestPings {
		rtt, err := s.pingNode(ctx, n, proxyURL)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			res.Lost++
			continue
		}
		rtts = append(rtts, rtt)
	}
	res.Samples = len(rtts)
	if res.Samples == 0 {
		return nil, fmt.Errorf("node did not answer any of %d pings", speedTestPings)
	}
	var sum, jitter time.Duration
	minRtt, maxRtt := rtts[0], rtts[0]
	for i, rtt := range rtts {
		sum += rtt
		minRtt, maxRtt = min(minRtt, rtt), max(maxRtt, rtt)
		if i > 0 {
			d := rtt - rtts[i-1]
			if d < 0 {
				d = -d
			}
			jitter += d
		}
	}
	res.LatencyMinMs, res.LatencyMaxMs = roundMs(minRtt), roundMs(maxRtt)
	res.LatencyAvgMs = roundMs(sum / time.Duration(len(rtts)))
	if len(rtts) > 1 {
		res.JitterMs = roundMs(jitter / time.Duration(len(rtts)-1))
	}

	down, err := s.timedDownload(ctx, n, proxyURL, speedTestDownloadSize)
	if err != nil {
		res.ThroughputError = err.Error()
		return res, nil
	}
	res.DownloadMbps = mbps(speedTestDownloadSize, down)
	up, err := s.timedUpload(ctx, n, proxyURL, speedTestUploadBytes)
	if err != nil {
		res.ThroughputError = err.Error()
		return res, nil
	}
	res.UploadMbps = mbps(speedTestUploadBytes, up)
	return res, nil
}

func (s *NodeService) pingNode(ctx context.Context, n *model.Node, proxyURL string) (time.Duration, error) {
	req, client, err := s.nodeRequest(ctx, n, proxyURL, http.MethodGet, "panel/api/server/status", nil)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d from remote panel", resp.StatusCode)
	}
	return time.Since(start), nil
}

// speedtestUnsupported names the usual cause of a missing route.
func speedtestUnsupported(code int) error {
	if code == http.StatusNotFound || code == http.StatusMethodNotAllowed {
		return fmt.Errorf("node has no speed test routes (HTTP %d); update the node panel", code)
	}
	return fmt.Errorf("HTTP %d from remote panel", code)
}

func (s *NodeService) timedDownload(ctx context.Context, n *model.Node, proxyURL string, size int64) (time.Duration, error) {
	req, client, err := s.nodeRequest(ctx, n, proxyURL, http.MethodGet, "panel/api/server/speedtestDownload", nil)
	if err != nil {
		return 0, err
	}
	req.URL.RawQuery = "bytes=" + strconv.FormatInt(size, 10)
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, speedtestUnsupported(resp.StatusCode)
	}
	got, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, err
	}
	if got != size {
		return 0, fmt.Errorf("node sent %d of %d bytes", got, size)
	}
	return time.Since(start), nil
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func (s *NodeService) timedUpload(ctx context.Context, n *model.Node, proxyURL string, size int64) (time.Duration, error) {
	req, client, err := s.nodeRequest(ctx, n, proxyURL, http.MethodPost, "panel/api/server/speedtestUpload", io.LimitReader(zeroReader{}, size))
	if err != nil {
		return 0, err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return 0, speedtestUnsupported(resp.StatusCode)
	}
	return time.Since(start), nil
}
