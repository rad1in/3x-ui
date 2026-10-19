// Package sockkill ends established TCP connections to a local port from given
// peers with `ss -K`. Xray cannot drop a session already authenticated, so this
// is the only way to disconnect a live client without restarting the core.
package sockkill

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"time"
)

const killTimeout = 5 * time.Second

// ErrUnsupported means the host cannot destroy sockets: no ss binary, no
// privilege, or a kernel without CONFIG_INET_DIAG_DESTROY.
var ErrUnsupported = errors.New("socket destroy is not available on this host")

// Kill destroys the established TCP connections from each ip to the local
// port and returns how many sockets were closed.
func Kill(ctx context.Context, port int, ips []string) (int, error) {
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port %d", port)
	}
	if len(ips) == 0 {
		return 0, nil
	}
	ssPath, err := exec.LookPath("ss")
	if err != nil {
		return 0, ErrUnsupported
	}
	killed := 0
	var firstErr error
	for _, raw := range ips {
		ip := net.ParseIP(raw)
		if ip == nil {
			continue
		}
		n, err := killOne(ctx, ssPath, port, ip)
		killed += n
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return killed, firstErr
}

func killOne(ctx context.Context, ssPath string, port int, ip net.IP) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, killTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, ssPath, "-K", "-H", "-t", "state", "established",
		"dst", ip.String(), "sport", "=", ":"+strconv.Itoa(port))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("%w: %v: %s", ErrUnsupported, err, bytes.TrimSpace(stderr.Bytes()))
	}
	return bytes.Count(stdout.Bytes(), []byte("\n")), nil
}
