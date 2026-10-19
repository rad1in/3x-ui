package ocserv

import (
	"crypto/rand"
	"crypto/sha512"
	"math/big"
)

// ocserv's plain auth checks passwords with crypt(3); SHA-512 crypt is the
// one every libc understands.
const (
	cryptRounds  = 5000
	cryptAlpha   = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	cryptSaltLen = 16
)

// The byte order of the final digest in Drepper's encoding, three at a time.
var cryptOrder = [21][3]int{
	{0, 21, 42}, {22, 43, 1}, {44, 2, 23}, {3, 24, 45}, {25, 46, 4}, {47, 5, 26}, {6, 27, 48},
	{28, 49, 7}, {50, 8, 29}, {9, 30, 51}, {31, 52, 10}, {53, 11, 32}, {12, 33, 54}, {34, 55, 13},
	{56, 14, 35}, {15, 36, 57}, {37, 58, 16}, {59, 17, 38}, {18, 39, 60}, {40, 61, 19}, {62, 20, 41},
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, cryptSaltLen)
	for i := range salt {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(cryptAlpha))))
		if err != nil {
			return "", err
		}
		salt[i] = cryptAlpha[n.Int64()]
	}
	return sha512Crypt(password, string(salt)), nil
}

func repeatTo(src []byte, n int) []byte {
	out := make([]byte, 0, n)
	for len(out) < n {
		out = append(out, src[:min(len(src), n-len(out))]...)
	}
	return out
}

func sum512(parts ...[]byte) []byte {
	h := sha512.New()
	for _, p := range parts {
		h.Write(p)
	}
	return h.Sum(nil)
}

func sha512Crypt(password, salt string) string {
	pw := []byte(password)
	if len(salt) > cryptSaltLen {
		salt = salt[:cryptSaltLen]
	}
	s := []byte(salt)

	b := sum512(pw, s, pw)
	a := sha512.New()
	a.Write(pw)
	a.Write(s)
	a.Write(repeatTo(b, len(pw)))
	for n := len(pw); n > 0; n >>= 1 {
		if n&1 == 1 {
			a.Write(b)
		} else {
			a.Write(pw)
		}
	}
	digest := a.Sum(nil)

	dp := sha512.New()
	for range pw {
		dp.Write(pw)
	}
	p := repeatTo(dp.Sum(nil), len(pw))
	ds := sha512.New()
	for range 16 + int(digest[0]) {
		ds.Write(s)
	}
	salted := repeatTo(ds.Sum(nil), len(s))

	for i := range cryptRounds {
		h := sha512.New()
		if i&1 == 1 {
			h.Write(p)
		} else {
			h.Write(digest)
		}
		if i%3 != 0 {
			h.Write(salted)
		}
		if i%7 != 0 {
			h.Write(p)
		}
		if i&1 == 1 {
			h.Write(digest)
		} else {
			h.Write(p)
		}
		digest = h.Sum(nil)
	}

	out := make([]byte, 0, 86)
	put := func(v uint32, n int) {
		for range n {
			out = append(out, cryptAlpha[v&0x3f])
			v >>= 6
		}
	}
	for _, o := range cryptOrder {
		put(uint32(digest[o[0]])<<16|uint32(digest[o[1]])<<8|uint32(digest[o[2]]), 4)
	}
	put(uint32(digest[63]), 2)
	return "$6$" + string(s) + "$" + string(out)
}
