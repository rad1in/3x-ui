package s3lite

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// The three worked examples from AWS's "Authenticating Requests: Using the
// Authorization Header" page; a signer that matches them talks to real S3.
func TestSignMatchesAWSDocumentedExamples(t *testing.T) {
	const (
		accessKey = "AKIAIOSFODNN7EXAMPLE"
		secretKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
		emptySHA  = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	)
	at := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name, method, url, payloadHash string
		headers                        map[string]string
		wantSig                        string
	}{
		{"GET object", http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", emptySHA,
			map[string]string{"Range": "bytes=0-9"},
			"f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"},
		{"PUT object", http.MethodPut, "https://examplebucket.s3.amazonaws.com/test$file.text",
			"44ce7dd67c959e0d3524ffac1771dfbba87d2b6b4b4e99e42034a8b803f8b072",
			map[string]string{"Date": "Fri, 24 May 2013 00:00:00 GMT", "X-Amz-Storage-Class": "REDUCED_REDUNDANCY"},
			"98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd"},
		{"GET bucket", http.MethodGet, "https://examplebucket.s3.amazonaws.com/?max-keys=2&prefix=J", emptySHA, nil,
			"34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := http.NewRequest(c.method, c.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range c.headers {
				req.Header.Set(k, v)
			}
			sign(req, c.payloadHash, accessKey, secretKey, "us-east-1", at)
			auth := req.Header.Get("Authorization")
			if !strings.HasSuffix(auth, "Signature="+c.wantSig) {
				t.Fatalf("Authorization = %q, want signature %s", auth, c.wantSig)
			}
			if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request,") {
				t.Fatalf("credential scope wrong: %q", auth)
			}
		})
	}
}
