package openvpn

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// netSetup routes the client network to the internet; tests swap it out so
// they never touch the host firewall.
var netSetup router = iptablesRouter{}

type router interface {
	up(inst Instance) error
	down(inst Instance)
}

type iptablesRouter struct{}

func (iptablesRouter) rules(inst Instance) [][]string {
	network := inst.Network.Masked().String()
	return [][]string{
		{"-t", "nat", "POSTROUTING", "-s", network, "!", "-d", network, "-j", "MASQUERADE"},
		{"-t", "filter", "FORWARD", "-i", inst.device(), "-j", "ACCEPT"},
		{"-t", "filter", "FORWARD", "-o", inst.device(), "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
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

func (r iptablesRouter) up(inst Instance) error {
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0o644); err != nil {
		logger.Warning("openvpn: enable ip_forward:", err)
	}
	for _, rule := range r.rules(inst) {
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

func (r iptablesRouter) down(inst Instance) {
	for _, rule := range r.rules(inst) {
		for iptables("-C", rule) == nil {
			if err := iptables("-D", rule); err != nil {
				logger.Debug("openvpn: remove rule", strings.Join(rule, " "), err)
				break
			}
		}
	}
}
