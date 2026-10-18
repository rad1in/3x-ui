package l2tp

import "github.com/mhsanaei/3x-ui/v3/internal/pppsecrets"

// chapSecretsPath is fixed in pppd 2.4, so the panel keeps its logins in a
// marked block of the system file and leaves every other line alone.
var chapSecretsPath = pppsecrets.Path

func writeChapSecrets(path string, users map[string]string) error {
	entries := make([]pppsecrets.Entry, 0, len(users))
	for user, secret := range users {
		entries = append(entries, pppsecrets.Entry{User: user, Server: pppsecrets.Any, Secret: secret})
	}
	return pppsecrets.Write(path, "l2tp", entries)
}
