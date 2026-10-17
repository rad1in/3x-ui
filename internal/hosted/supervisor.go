package hosted

import (
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

const stopGrace = 15 * time.Second

// Panel is one hosted panel process the master keeps running.
type Panel struct {
	ID      int
	Dir     string
	Env     []string
	LogPath string
}

// ProcState is the supervisor's view of one panel process.
type ProcState struct {
	Running   bool   `json:"running" example:"true"`
	Pid       int    `json:"pid" example:"4242"`
	Restarts  int    `json:"restarts" example:"0"`
	LastError string `json:"lastError" example:""`
	StartedAt int64  `json:"startedAt" example:"1760000000000"`
}

type proc struct {
	panel Panel
	stop  chan struct{}
	done  chan struct{}
	cmd   *exec.Cmd
	state ProcState
}

// Supervisor runs hosted panels as child processes and restarts them when they die.
type Supervisor struct {
	command                func(Panel) *exec.Cmd
	backoffMin, backoffMax time.Duration

	mu    sync.Mutex
	procs map[int]*proc
	last  map[int]ProcState
}

func NewSupervisor(command func(Panel) *exec.Cmd) *Supervisor {
	return &Supervisor{
		command:    command,
		backoffMin: time.Second,
		backoffMax: time.Minute,
		procs:      map[int]*proc{},
		last:       map[int]ProcState{},
	}
}

// Start runs the panel unless it already runs; a crashed panel is restarted with backoff.
func (s *Supervisor) Start(p Panel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.procs[p.ID]; ok {
		return
	}
	pr := &proc{panel: p, stop: make(chan struct{}), done: make(chan struct{})}
	s.procs[p.ID] = pr
	go s.run(pr)
}

func (s *Supervisor) run(pr *proc) {
	defer close(pr.done)
	backoff := s.backoffMin
	for {
		started := time.Now()
		err := s.runOnce(pr)
		s.mu.Lock()
		pr.cmd = nil
		pr.state.Running, pr.state.Pid = false, 0
		if err != nil {
			pr.state.LastError = err.Error()
		}
		s.mu.Unlock()
		if time.Since(started) > time.Minute {
			backoff = s.backoffMin
		}
		select {
		case <-pr.stop:
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, s.backoffMax)
		s.mu.Lock()
		pr.state.Restarts++
		s.mu.Unlock()
	}
}

func (s *Supervisor) runOnce(pr *proc) error {
	cmd := s.command(pr.panel)
	if pr.panel.LogPath != "" {
		logFile, err := os.OpenFile(pr.panel.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer logFile.Close()
		cmd.Stdout, cmd.Stderr = logFile, logFile
	}
	s.mu.Lock()
	select {
	case <-pr.stop:
		s.mu.Unlock()
		return nil
	default:
	}
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		return err
	}
	pr.cmd = cmd
	pr.state.Running, pr.state.Pid = true, cmd.Process.Pid
	pr.state.StartedAt = time.Now().UnixMilli()
	s.mu.Unlock()
	return cmd.Wait()
}

// Stop ends the panel and waits for it, killing it if it ignores the interrupt.
func (s *Supervisor) Stop(id int) {
	s.mu.Lock()
	pr, ok := s.procs[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	delete(s.procs, id)
	close(pr.stop)
	if pr.cmd != nil {
		interrupt(pr.cmd.Process)
	}
	s.mu.Unlock()

	select {
	case <-pr.done:
	case <-time.After(stopGrace):
		s.mu.Lock()
		if pr.cmd != nil {
			_ = pr.cmd.Process.Kill()
		}
		s.mu.Unlock()
		<-pr.done
	}
	s.mu.Lock()
	s.last[id] = pr.state
	s.mu.Unlock()
}

func (s *Supervisor) StopAll() {
	s.mu.Lock()
	ids := make([]int, 0, len(s.procs))
	for id := range s.procs {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Go(func() { s.Stop(id) })
	}
	wg.Wait()
}

// State reports the live process, or how a stopped one last ended.
func (s *Supervisor) State(id int) ProcState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pr, ok := s.procs[id]; ok {
		return pr.state
	}
	return s.last[id]
}

func interrupt(p *os.Process) {
	if runtime.GOOS == "windows" {
		_ = p.Kill()
		return
	}
	_ = p.Signal(os.Interrupt)
}

// ExitWithParent calls onGone once this process is re-parented, so a hosted
// panel never outlives the master that owns its ports and limits.
func ExitWithParent(onGone func()) {
	go watchParent(os.Getppid, 2*time.Second, onGone)
}

func watchParent(getppid func() int, every time.Duration, onGone func()) {
	parent := getppid()
	for {
		time.Sleep(every)
		if getppid() != parent {
			onGone()
			return
		}
	}
}
