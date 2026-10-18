package sstp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

const pppdStopTimeout = 2 * time.Second

// BinaryPath prefers a pppd shipped in the panel's bin folder over the system one.
func BinaryPath() (string, error) {
	local := filepath.Join(config.GetBinFolderPath(), "pppd")
	if fi, err := os.Stat(local); err == nil && !fi.IsDir() {
		return local, nil
	}
	for _, p := range []string{"/usr/sbin/pppd", "/usr/local/sbin/pppd"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("pppd"); err == nil {
		return p, nil
	}
	return "", errors.New("pppd not found; install the ppp package")
}

func runDir() string {
	return filepath.Join(config.GetBinFolderPath(), "sstp.d")
}

// hostCanRunPPP says why this host cannot serve PPP-based VPNs, if it cannot.
func hostCanRunPPP() error {
	if _, err := os.Stat("/dev/ppp"); err != nil {
		return errors.New("this kernel has no PPP support (/dev/ppp is missing), which SSTP needs")
	}
	_, err := BinaryPath()
	return err
}

// pppdProc is a pppd speaking async HDLC on its standard input and output.
type pppdProc struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *os.File
	done   chan struct{}
}

func (p *pppdProc) Read(b []byte) (int, error)  { return p.stdout.Read(b) }
func (p *pppdProc) Write(b []byte) (int, error) { return p.stdin.Write(b) }
func (p *pppdProc) Done() <-chan struct{}       { return p.done }

// Close hangs pppd up by closing its terminal, then makes sure it is gone.
func (p *pppdProc) Close() error {
	defer p.stdout.Close()
	_ = p.stdin.Close()
	select {
	case <-p.done:
	case <-time.After(pppdStopTimeout):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
	return nil
}

func spawnPppd(inst Instance, local, remote netip.Addr) (pppConn, error) {
	bin, err := BinaryPath()
	if err != nil {
		return nil, err
	}
	dir := runDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	opts := filepath.Join(dir, fmt.Sprintf("options-%d", inst.Id))
	if err := os.WriteFile(opts, []byte(inst.pppOptions()), 0o600); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(context.Background(), bin, "notty", "nodetach", "file", opts,
		"name", inst.pppName(), fmt.Sprintf("%s:%s", local, remote))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	// An os.Pipe, not StdoutPipe: Wait would close the latter and could cut off
	// the last frames, such as the one telling the client its login failed.
	stdout, outW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = outW
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	err = cmd.Start()
	_ = outW.Close()
	if err != nil {
		_ = stdout.Close()
		return nil, err
	}
	p := &pppdProc{cmd: cmd, stdin: stdin, stdout: stdout, done: make(chan struct{})}
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			logger.Debugf("sstp pppd %d | %s", inst.Id, sc.Text())
		}
	}()
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}
