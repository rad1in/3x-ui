// Package s3lite is the small slice of the S3 API the backup job needs: put,
// list and delete objects on any S3-compatible store, signed with SigV4.
package s3lite

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Client addresses objects path-style (endpoint/bucket/key), which AWS, R2,
// MinIO, Wasabi and Backblaze all accept.
type Client struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	HTTP      *http.Client
}

// Object is one listed key.
type Object struct {
	Key  string
	Size int64
}

func (c *Client) region() string {
	if c.Region == "" {
		return "us-east-1"
	}
	return c.Region
}

func (c *Client) do(ctx context.Context, method, key string, query url.Values, body []byte) ([]byte, error) {
	u, err := url.Parse(strings.TrimRight(c.Endpoint, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("s3: endpoint %q is not an http(s) URL", c.Endpoint)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + c.Bucket + "/" + key
	u.RawPath = uriEncode(u.Path, false)
	u.RawQuery = canonicalQuery(query)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	sign(req, hex.EncodeToString(sum[:]), c.AccessKey, c.SecretKey, c.region(), time.Now())
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s3: %s %s: %w", method, key, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		}
		_ = xml.Unmarshal(raw, &e)
		return nil, fmt.Errorf("s3: %s %s: HTTP %d %s %s", method, key, resp.StatusCode, e.Code, e.Message)
	}
	return raw, nil
}

func (c *Client) Put(ctx context.Context, key string, body []byte) error {
	_, err := c.do(ctx, http.MethodPut, key, nil, body)
	return err
}

func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.do(ctx, http.MethodDelete, key, nil, nil)
	return err
}

// List returns every key under prefix, following continuation tokens.
func (c *Client) List(ctx context.Context, prefix string) ([]Object, error) {
	var out []Object
	token := ""
	for {
		q := url.Values{"list-type": {"2"}, "prefix": {prefix}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		raw, err := c.do(ctx, http.MethodGet, "", q, nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			Contents []struct {
				Key  string `xml:"Key"`
				Size int64  `xml:"Size"`
			} `xml:"Contents"`
			IsTruncated           bool   `xml:"IsTruncated"`
			NextContinuationToken string `xml:"NextContinuationToken"`
		}
		if err := xml.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("s3: list %q: %w", prefix, err)
		}
		for _, o := range page.Contents {
			out = append(out, Object{Key: o.Key, Size: o.Size})
		}
		if !page.IsTruncated || page.NextContinuationToken == "" {
			return out, nil
		}
		token = page.NextContinuationToken
	}
}

// uriEncode is SigV4's encoding: every byte but A-Z a-z 0-9 - _ . ~ as %XX,
// and "/" too unless it separates path segments.
func uriEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case 'A' <= ch && ch <= 'Z', 'a' <= ch && ch <= 'z', '0' <= ch && ch <= '9', ch == '-', ch == '_', ch == '.', ch == '~':
			b.WriteByte(ch)
		case ch == '/' && !encodeSlash:
			b.WriteByte(ch)
		default:
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}
	return b.String()
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, uriEncode(k, true)+"="+uriEncode(v, true))
		}
	}
	return strings.Join(parts, "&")
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

// sign adds SigV4 headers, signing host and every header already on req.
func sign(req *http.Request, payloadHash, accessKey, secretKey, region string, at time.Time) {
	amzDate := at.UTC().Format("20060102T150405Z")
	day := amzDate[:8]
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	values := map[string]string{"host": req.URL.Host}
	for k, vs := range req.Header {
		lk := strings.ToLower(k)
		if lk == "authorization" || lk == "user-agent" {
			continue
		}
		trimmed := make([]string, len(vs))
		for i, v := range vs {
			trimmed[i] = strings.Join(strings.Fields(v), " ")
		}
		values[lk] = strings.Join(trimmed, ",")
	}
	names := make([]string, 0, len(values))
	for k := range values {
		names = append(names, k)
	}
	sort.Strings(names)
	var headers strings.Builder
	for _, k := range names {
		headers.WriteString(k + ":" + values[k] + "\n")
	}
	signed := strings.Join(names, ";")

	path := req.URL.Path
	if path == "" {
		path = "/"
	}
	canonical := strings.Join([]string{
		req.Method, uriEncode(path, false), canonicalQuery(req.URL.Query()), headers.String(), signed, payloadHash,
	}, "\n")
	scope := day + "/" + region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])

	key := hmacSHA256([]byte("AWS4"+secretKey), day)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+
		", SignedHeaders="+signed+", Signature="+signature)
}
