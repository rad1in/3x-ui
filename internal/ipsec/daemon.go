package ipsec

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/strongswan/govici/vici"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

const (
	viciDialTimeout = 15 * time.Second
	stopTimeout     = 5 * time.Second
)

// charonPaths are where Debian/Ubuntu (strongswan-charon) and RHEL (strongswan)
// install the daemon, after a copy shipped in the panel's bin folder.
var charonPaths = []string{
	"/usr/lib/ipsec/charon",
	"/usr/libexec/ipsec/charon",
	"/usr/libexec/strongswan/charon",
	"/usr/lib/strongswan/charon",
}

func BinaryPath() (string, error) {
	local := filepath.Join(config.GetBinFolderPath(), "charon")
	for _, p := range append([]string{local}, charonPaths...) {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("strongSwan charon not found; install strongswan-charon (Debian/Ubuntu) or strongswan (RHEL)")
}

// charonPidFile is compiled into charon, so a second daemon on the host (a
// system strongSwan) makes ours exit at once; it is checked up front instead.
const charonPidFile = "/var/run/charon.pid"

// runningCharon reports the live process a charon pid file names, if any.
func runningCharon(pidFile string) (int, bool) {
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	if !processAlive(pid) {
		return 0, false
	}
	return pid, true
}

func runDir() string {
	return filepath.Join(config.GetBinFolderPath(), "ipsec.d")
}

// kernelESP reports whether the kernel can hold an ESP SA. Containers and some
// VPS kernels cannot, and charon then has to run ESP in userspace (libipsec).
func kernelESP() bool {
	add := []string{"xfrm", "state", "add", "src", "198.51.100.1", "dst", "198.51.100.2", "proto", "esp",
		"spi", "0x7fffff01", "mode", "tunnel", "enc", "cbc(aes)", "0x00112233445566778899aabbccddeeff",
		"auth-trunc", "hmac(sha256)", "0x00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff", "128"}
	if exec.CommandContext(context.Background(), "ip", add...).Run() != nil {
		return false
	}
	_ = exec.CommandContext(context.Background(), "ip", "xfrm", "state", "delete", "src", "198.51.100.1",
		"dst", "198.51.100.2", "proto", "esp", "spi", "0x7fffff01").Run()
	return true
}

func strongswanConf(dir string, libipsec bool) string {
	load := "no"
	if libipsec {
		load = "yes"
	}
	return fmt.Sprintf(`charon {
  install_routes = yes
  install_virtual_ip = no
  filelog {
    panel {
      path = %[1]s/charon.log
      append = no
      flush_line = yes
      default = 1
    }
  }
  plugins {
    vici {
      socket = unix://%[1]s/charon.vici
    }
    kernel-libipsec {
      load = %[2]s
    }
    # bypass-lan routes the LAN outside the tunnel device, IKE traffic included
    bypass-lan {
      load = no
    }
    stroke {
      load = no
    }
  }
}
`, dir, load)
}

type daemon struct {
	cmd      *exec.Cmd
	done     chan struct{}
	sess     *vici.Session
	lastLine atomic.Pointer[string]
}

func (d *daemon) setLast(line string) { d.lastLine.Store(&line) }

func (d *daemon) last() string {
	if p := d.lastLine.Load(); p != nil {
		return *p
	}
	return ""
}

func (d *daemon) running() bool {
	select {
	case <-d.done:
		return false
	default:
		return true
	}
}

func startDaemon(libipsec bool) (*daemon, error) {
	bin, err := BinaryPath()
	if err != nil {
		return nil, err
	}
	if pid, ok := runningCharon(charonPidFile); ok {
		return nil, fmt.Errorf("ipsec: another strongSwan charon (pid %d) already runs on this server; stop it (systemctl disable --now strongswan-starter strongswan) so the panel can serve IKEv2", pid)
	}
	dir := runDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	sock := filepath.Join(dir, "charon.vici")
	_ = os.Remove(sock)
	confPath := filepath.Join(dir, "strongswan.conf")
	if err := os.WriteFile(confPath, []byte(strongswanConf(dir, libipsec)), 0o600); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(context.Background(), bin)
	cmd.Env = append(os.Environ(), "STRONGSWAN_CONF="+confPath)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	d := &daemon{cmd: cmd, done: make(chan struct{})}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go d.pump(out)
	go func() {
		if err := cmd.Wait(); err != nil {
			d.setLast(err.Error())
		}
		close(d.done)
	}()
	sess, err := dialVici(sock, d.done)
	if err != nil {
		d.stop()
		return nil, fmt.Errorf("ipsec: %w (%s)", err, d.last())
	}
	d.sess = sess
	return d, nil
}

func (d *daemon) pump(r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		d.setLast(sc.Text())
		logger.Debugf("charon | %s", sc.Text())
	}
}

func dialVici(sock string, done <-chan struct{}) (*vici.Session, error) {
	deadline := time.Now().Add(viciDialTimeout)
	for {
		if _, err := os.Stat(sock); err == nil {
			if sess, err := vici.NewSession(vici.WithSocketPath(sock)); err == nil {
				return sess, nil
			}
		}
		select {
		case <-done:
			return nil, errors.New("charon exited before its control socket opened")
		default:
		}
		if time.Now().After(deadline) {
			return nil, errors.New("charon's control socket did not open")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (d *daemon) stop() {
	if d.sess != nil {
		_ = d.sess.Close()
	}
	if d.running() {
		_ = d.cmd.Process.Signal(os.Interrupt)
		select {
		case <-d.done:
		case <-time.After(stopTimeout):
			_ = d.cmd.Process.Kill()
			<-d.done
		}
	}
}

func (d *daemon) call(cmd string, msg *vici.Message) (*vici.Message, error) {
	if msg == nil {
		msg = vici.NewMessage()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return d.sess.Call(ctx, cmd, msg)
}
