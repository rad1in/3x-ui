//go:build !windows

package ipsec

import "syscall"

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
