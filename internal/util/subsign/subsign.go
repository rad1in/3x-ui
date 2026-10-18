// Package subsign signs subscription links: an HMAC over the subscription id
// and an optional expiry, so a link can be shared without exposing the id alone.
package subsign

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strconv"
	"time"
)

var (
	ErrBadSignature = errors.New("subscription link signature is invalid")
	ErrExpired      = errors.New("subscription link has expired")
)

func mac(key []byte, subID string, exp int64) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte("sub|" + subID + "|" + strconv.FormatInt(exp, 10)))
	return h.Sum(nil)
}

// Sign returns the signature for a subscription id; exp is unix seconds, 0 for no expiry.
func Sign(key []byte, subID string, exp int64) string {
	return base64.RawURLEncoding.EncodeToString(mac(key, subID, exp))
}

// Verify checks sig against the id and expiry, then the expiry against now.
func Verify(key []byte, subID string, exp int64, sig string, now time.Time) error {
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || len(key) == 0 || subtle.ConstantTimeCompare(got, mac(key, subID, exp)) != 1 {
		return ErrBadSignature
	}
	if exp > 0 && now.Unix() > exp {
		return ErrExpired
	}
	return nil
}
