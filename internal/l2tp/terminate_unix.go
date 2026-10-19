//go:build !windows

package l2tp

import "syscall"

func terminate(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}
