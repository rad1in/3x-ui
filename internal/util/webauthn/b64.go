package webauthn

import "encoding/base64"

// b64Decode reads unpadded base64url, the encoding WebAuthn JSON uses.
func b64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
