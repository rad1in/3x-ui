// Package vpnnat routes a VPN server's client network to the internet with
// iptables: masquerade, forward and MSS clamping, keyed on the client network
// so it serves tun-based (OpenVPN) and policy-based (IPsec) servers alike.
package vpnnat

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// Router is what VPN managers call; tests swap Default out so they never
// touch the host firewall.
type Router interface {
	Up(network netip.Prefix) error
	Down(network netip.Prefix)
}

var Default Router = iptablesRouter{}

type iptablesRouter struct{}

func rules(network netip.Prefix) [][]string {
	n := network.Masked().String()
	return [][]string{
		{"-t", "nat", "POSTROUTING", "-s", n, "!", "-d", n, "-j", "MASQUERADE"},
		{"-t", "filter", "FORWARD", "-s", n, "-j", "ACCEPT"},
		{"-t", "filter", "FORWARD", "-d", n, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		// Tunnels shrink the path MTU; without clamping, TCP through them stalls on big packets.
		{"-t", "mangle", "FORWARD", "-s", n, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
	}
}

func iptables(op string, rule []string) error {
	args := append([]string{"-w", rule[0], rule[1], op, rule[2]}, rule[3:]...)
	out, err := exec.CommandContext(context.Background(), "iptables", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (iptablesRouter) Up(network netip.Prefix) error {
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0o644); err != nil {
		logger.Warning("vpnnat: enable ip_forward:", err)
	}
	for _, rule := range rules(network) {
		if iptables("-C", rule) == nil {
			continue
		}
		op := "-A"
		if rule[1] == "FORWARD" {
			op = "-I" // ahead of a distribution's default DROP rules
		}
		if err := iptables(op, rule); err != nil {
			return err
		}
	}
	return nil
}

func (iptablesRouter) Down(network netip.Prefix) {
	for _, rule := range rules(network) {
		for iptables("-C", rule) == nil {
			if err := iptables("-D", rule); err != nil {
				logger.Debug("vpnnat: remove rule", strings.Join(rule, " "), err)
				break
			}
		}
	}
}
