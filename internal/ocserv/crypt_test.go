package ocserv

import (
	"strings"
	"testing"
)

// Expected hashes come from `openssl passwd -6 -salt <salt> <password>`, the
// reference the system crypt(3) agrees with.
func TestSHA512CryptMatchesOpenSSL(t *testing.T) {
	for _, tc := range []struct{ salt, password, want string }{
		{"abcdefghijklmnop", "secret", "$6$abcdefghijklmnop$J/AWykHqo2Tx5UtavGnFc3ytI33la50JpzLTarSWVhkIXK6wOjNwwZjsrIw2UgmrER2EKrSHCeQyAINEEXAk1/"},
		{"short", "p", "$6$short$xsN.pyVdfrC.NFyKpj0McNWK.c7vyPl2HSThfag1cHbdashqoSVl0MA6oh/E0xWLDvwMfzmsRsh7L5XY7PjTH1"},
		{"0123456789abcdef", "Tr0ub4dor&3", "$6$0123456789abcdef$qyuvBrh3303/TMqALTZatADhsJXNYwV5CagfQDpITVMVeXagfR6LXvFzrp4nYExqIqa8h7vJxFQDNOVMZdjVv1"},
		// Longer than the 64-byte digest, which takes the partial-block path.
		{"saltsalt", "alice@example.com-password-that-is-quite-long-0123456789-0123456789-0123456789-abcdefghij", "$6$saltsalt$uFSzwmXPYnyqcGZLozK6GK4amRRlgDe5hBh8d50ltY28BjRohWPQ8NjOrtqX8eMlleO9Q82kR2Lvc790sVcYj1"},
	} {
		if got := sha512Crypt(tc.password, tc.salt); got != tc.want {
			t.Errorf("sha512Crypt(%q, %q) =\n%s\nwant\n%s", tc.password, tc.salt, got, tc.want)
		}
	}
}

func TestHashPasswordUsesAFreshSixteenCharacterSalt(t *testing.T) {
	a, err := hashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := hashPassword("pw")
	parts := strings.Split(a, "$")
	if a == b || len(parts) != 4 || parts[1] != "6" || len(parts[2]) != 16 {
		t.Fatalf("hashes %q and %q; want distinct $6$ hashes with 16-character salts", a, b)
	}
}
