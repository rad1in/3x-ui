package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math/rand/v2"
	"net"
	"net/netip"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/util/netsafe"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"

	"gorm.io/gorm"
)

const (
	cleanIPMaxTargets   = 2048
	cleanIPMaxTokens    = 1024
	cleanIPConcurrency  = 64
	cleanIPMaxSamples   = 5
	cleanIPMaxTimeoutMs = 5000
	// cleanIPSpaceCap stops the range-size sum from overflowing on wide IPv6 prefixes.
	cleanIPSpaceCap = uint64(1) << 62
)

// CleanIPScanRequest is a probe run over a list or range of CDN addresses. An
// SNI turns the TCP connect into a verified TLS handshake for that name.
type CleanIPScanRequest struct {
	Targets   string `json:"targets" example:"104.16.0.0/24, 172.64.32.1"`
	Port      int    `json:"port" example:"443"`
	SNI       string `json:"sni" example:"cdn.example.com"`
	Samples   int    `json:"samples" example:"3"`
	TimeoutMs int    `json:"timeoutMs" example:"2000"`
}

// CleanIPStatus is the progress of the running or most recent scan.
type CleanIPStatus struct {
	Running    bool   `json:"running" example:"false"`
	Total      int    `json:"total" example:"256"`
	Done       int    `json:"done" example:"256"`
	StartedAt  int64  `json:"startedAt" example:"1760000000000"`
	FinishedAt int64  `json:"finishedAt" example:"1760000042000"`
	Error      string `json:"error" example:""`
}

type cleanIPRange struct {
	start netip.Addr
	size  uint64
}

func addrAdd(a netip.Addr, n uint64) netip.Addr {
	b := a.As16()
	for i := 15; i >= 0 && n > 0; i-- {
		sum := uint64(b[i]) + n&0xff
		b[i] = byte(sum)
		n = n>>8 + sum>>8
	}
	out := netip.AddrFrom16(b)
	if a.Is4() {
		return out.Unmap()
	}
	return out
}

func parseCleanIPToken(tok string) (cleanIPRange, error) {
	if lo, hi, ok := strings.Cut(tok, "-"); ok {
		a, errA := netip.ParseAddr(strings.TrimSpace(lo))
		b, errB := netip.ParseAddr(strings.TrimSpace(hi))
		if errA != nil || errB != nil || !a.Is4() || !b.Is4() || b.Compare(a) < 0 {
			return cleanIPRange{}, common.NewErrorf("invalid IPv4 range %q", tok)
		}
		a4, b4 := a.As4(), b.As4()
		span := uint64(uint32(b4[0])<<24|uint32(b4[1])<<16|uint32(b4[2])<<8|uint32(b4[3])) -
			uint64(uint32(a4[0])<<24|uint32(a4[1])<<16|uint32(a4[2])<<8|uint32(a4[3]))
		return cleanIPRange{start: a, size: span + 1}, nil
	}
	if strings.Contains(tok, "/") {
		p, err := netip.ParsePrefix(tok)
		if err != nil {
			return cleanIPRange{}, common.NewErrorf("invalid CIDR %q", tok)
		}
		hostBits := p.Addr().BitLen() - p.Bits()
		size := cleanIPSpaceCap
		if hostBits < 62 {
			size = uint64(1) << hostBits
		}
		return cleanIPRange{start: p.Masked().Addr(), size: size}, nil
	}
	a, err := netip.ParseAddr(tok)
	if err != nil {
		return cleanIPRange{}, common.NewErrorf("invalid IP address %q", tok)
	}
	return cleanIPRange{start: a.Unmap(), size: 1}, nil
}

// expandCleanIPTargets lists the addresses in spec (IPs, CIDRs and a-b ranges
// separated by commas or whitespace). A space larger than limit is sampled at random.
func expandCleanIPTargets(spec string, limit int) ([]netip.Addr, error) {
	tokens := strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\r' || r == '\t'
	})
	if len(tokens) == 0 {
		return nil, common.NewError("no targets given")
	}
	if len(tokens) > cleanIPMaxTokens {
		return nil, common.NewErrorf("too many entries (max %d)", cleanIPMaxTokens)
	}
	ranges := make([]cleanIPRange, 0, len(tokens))
	var total uint64
	for _, tok := range tokens {
		r, err := parseCleanIPToken(tok)
		if err != nil {
			return nil, err
		}
		ranges = append(ranges, r)
		total = min(total+r.size, cleanIPSpaceCap)
	}

	seen := map[netip.Addr]struct{}{}
	var out []netip.Addr
	add := func(a netip.Addr) {
		if _, dup := seen[a]; !dup {
			seen[a] = struct{}{}
			out = append(out, a)
		}
	}
	if total <= uint64(limit) {
		for _, r := range ranges {
			for i := uint64(0); i < r.size; i++ {
				add(addrAdd(r.start, i))
			}
		}
		return out, nil
	}
	for len(out) < limit {
		idx := rand.Uint64N(total)
		for _, r := range ranges {
			if idx < r.size {
				add(addrAdd(r.start, idx))
				break
			}
			idx -= r.size
		}
	}
	return out, nil
}

func classifyCleanIPError(err error) string {
	var hostErr x509.HostnameError
	var authErr x509.UnknownAuthorityError
	var invalidErr x509.CertificateInvalidError
	var netErr net.Error
	switch {
	case err == nil:
		return ""
	case errors.As(err, &hostErr), errors.As(err, &authErr), errors.As(err, &invalidErr):
		return "certificate"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	case strings.Contains(err.Error(), "refused"):
		return "refused"
	case strings.Contains(err.Error(), "reset"):
		return "reset"
	default:
		return "failed"
	}
}

type cleanIPScanner struct {
	dial    func(ctx context.Context, network, addr string) (net.Conn, error)
	blocked func(net.IP) bool
	roots   *x509.CertPool

	mu     sync.Mutex
	status CleanIPStatus
}

func newCleanIPScanner() *cleanIPScanner {
	return &cleanIPScanner{
		dial:    (&net.Dialer{}).DialContext,
		blocked: netsafe.IsBlockedIP,
	}
}

var defaultCleanIPScanner = newCleanIPScanner()

func (s *cleanIPScanner) Status() CleanIPStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *cleanIPScanner) setStatus(f func(*CleanIPStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.status)
}

func (s *cleanIPScanner) attempt(ctx context.Context, ip netip.Addr, port int, sni string) (time.Duration, error) {
	start := time.Now()
	conn, err := s.dial(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	if sni != "" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: sni, RootCAs: s.roots, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return 0, err
		}
	}
	return time.Since(start), nil
}

func (s *cleanIPScanner) probe(ip netip.Addr, req CleanIPScanRequest, timeout time.Duration) model.CleanIPResult {
	res := model.CleanIPResult{IP: ip.String(), Port: req.Port, ScannedAt: time.Now().UnixMilli()}
	if s.blocked(net.IP(ip.AsSlice())) {
		res.Reason, res.LossPct = "blocked", 100
		return res
	}
	var total time.Duration
	failed := 0
	for range req.Samples {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		d, err := s.attempt(ctx, ip, req.Port, req.SNI)
		cancel()
		if err != nil {
			failed++
			res.Reason = classifyCleanIPError(err)
			continue
		}
		total += d
	}
	res.LossPct = failed * 100 / req.Samples
	if failed < req.Samples {
		res.Ok, res.Reason = true, ""
		res.LatencyMs = int(total.Milliseconds()) / (req.Samples - failed)
	}
	return res
}

func normalizeCleanIPRequest(req *CleanIPScanRequest) error {
	if req.Port == 0 {
		req.Port = 443
	}
	if req.Port < 1 || req.Port > 65535 {
		return common.NewError("invalid port")
	}
	req.SNI = strings.TrimSpace(req.SNI)
	if req.Samples <= 0 {
		req.Samples = 3
	}
	req.Samples = min(req.Samples, cleanIPMaxSamples)
	if req.TimeoutMs <= 0 {
		req.TimeoutMs = 2000
	}
	req.TimeoutMs = min(req.TimeoutMs, cleanIPMaxTimeoutMs)
	return nil
}

// Start validates the request and scans in the background; one scan runs at a time.
func (s *cleanIPScanner) Start(req CleanIPScanRequest) error {
	if err := normalizeCleanIPRequest(&req); err != nil {
		return err
	}
	targets, err := expandCleanIPTargets(req.Targets, cleanIPMaxTargets)
	if err != nil {
		return err
	}
	if err := s.begin(len(targets)); err != nil {
		return err
	}
	go func() {
		if err := s.scan(targets, req); err != nil {
			logger.Warning("clean IP scan failed:", err)
		}
	}()
	return nil
}

// run scans synchronously.
func (s *cleanIPScanner) run(req CleanIPScanRequest) error {
	if err := normalizeCleanIPRequest(&req); err != nil {
		return err
	}
	targets, err := expandCleanIPTargets(req.Targets, cleanIPMaxTargets)
	if err != nil {
		return err
	}
	if err := s.begin(len(targets)); err != nil {
		return err
	}
	return s.scan(targets, req)
}

func (s *cleanIPScanner) begin(total int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Running {
		return common.NewError("a scan is already running")
	}
	s.status = CleanIPStatus{Running: true, Total: total, StartedAt: time.Now().UnixMilli()}
	return nil
}

func (s *cleanIPScanner) scan(targets []netip.Addr, req CleanIPScanRequest) (err error) {
	defer func() {
		s.setStatus(func(st *CleanIPStatus) {
			st.Running, st.FinishedAt = false, time.Now().UnixMilli()
			if err != nil {
				st.Error = err.Error()
			}
		})
	}()
	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	results := make([]model.CleanIPResult, len(targets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(cleanIPConcurrency, len(targets)) {
		wg.Go(func() {
			for i := range jobs {
				results[i] = s.probe(targets[i], req, timeout)
				s.setStatus(func(st *CleanIPStatus) { st.Done++ })
			}
		})
	}
	for i := range targets {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return saveCleanIPResults(results)
}

func saveCleanIPResults(rows []model.CleanIPResult) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&model.CleanIPResult{}).Error; err != nil {
			return err
		}
		return tx.CreateInBatches(&rows, 200).Error
	})
}

// Results lists the last scan, reachable addresses first by latency.
func (s *cleanIPScanner) Results() ([]model.CleanIPResult, error) {
	var rows []model.CleanIPResult
	if err := database.GetDB().Find(&rows).Error; err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Ok != b.Ok {
			return a.Ok
		}
		if a.LossPct != b.LossPct {
			return a.LossPct < b.LossPct
		}
		if a.LatencyMs != b.LatencyMs {
			return a.LatencyMs < b.LatencyMs
		}
		return a.IP < b.IP
	})
	return rows, nil
}

// CleanIPService scans CDN address ranges from the panel host for the Hosts page.
type CleanIPService struct{}

func (CleanIPService) Start(req CleanIPScanRequest) error { return defaultCleanIPScanner.Start(req) }
func (CleanIPService) Status() CleanIPStatus              { return defaultCleanIPScanner.Status() }
func (CleanIPService) Results() ([]model.CleanIPResult, error) {
	return defaultCleanIPScanner.Results()
}

// ApplyCleanIP points a subscription host at ip: it replaces or extends the
// addresses of host group groupId, or adds a new host to inbound inboundId.
func (s *HostService) ApplyCleanIP(ip, groupId string, inboundId int, mode string) ([]*model.Host, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return nil, common.NewError("not an IP address")
	}
	if mode == "" {
		mode = "replace"
	}
	if mode != "replace" && mode != "add" {
		return nil, common.NewError("mode must be replace or add")
	}
	ip = addr.Unmap().String()
	if groupId == "" {
		if inboundId <= 0 {
			return nil, common.NewError("choose a host or an inbound")
		}
		return s.AddHostGroup(&entity.HostGroup{InboundIds: []int{inboundId}, Remark: "clean-ip", Hosts: []string{ip}})
	}
	g, err := s.GetHostGroup(groupId)
	if err != nil {
		return nil, err
	}
	if mode == "replace" {
		g.Hosts = []string{ip}
	} else if !slices.Contains(g.Hosts, formatHostAddr(ip, g.Port)) {
		g.Hosts = append(g.Hosts, ip)
	}
	return s.UpdateHostGroup(groupId, g)
}
