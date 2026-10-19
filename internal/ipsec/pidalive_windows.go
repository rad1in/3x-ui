//go:build windows

package ipsec

// charon only runs on Unix hosts, so no pid it names can be live here.
func processAlive(int) bool {
	return false
}
