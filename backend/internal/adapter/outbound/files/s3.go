package files

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/raulaguila/dataset/internal/core/port/output"
)

// S3 talks to an S3-compatible bucket with path-style URLs and AWS Signature
// Version 4, which AWS S3, MinIO and Cloudflare R2 all accept. Three calls do
// not justify the AWS SDK.
type S3 struct {
	Endpoint  string // scheme and host, without the bucket
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	Client    *http.Client // nil uses a client with a 30s timeout
	now       func() time.Time
}

var _ output.FileStore = (*S3)(nil)

func (s *S3) Put(ctx context.Context, key string, data []byte) error {
	res, err := s.do(ctx, http.MethodPut, key, data)
	if err != nil {
		return err
	}
	return s.check(res, http.StatusOK)
}

func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	res, err := s.do(ctx, http.MethodGet, key, nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusNotFound {
		_ = res.Body.Close()
		return nil, notFound()
	}
	if err := s.check(res, http.StatusOK); err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	return io.ReadAll(res.Body)
}

// Delete answers 204 whether or not the key existed.
func (s *S3) Delete(ctx context.Context, key string) error {
	res, err := s.do(ctx, http.MethodDelete, key, nil)
	if err != nil {
		return err
	}
	return s.check(res, http.StatusNoContent)
}

// check closes the body unless the status is ok and the caller reads it.
func (s *S3) check(res *http.Response, ok int) error {
	if res.StatusCode == ok {
		if res.Request.Method != http.MethodGet {
			_ = res.Body.Close()
		}
		return nil
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 300))
	return fmt.Errorf("s3 %s: %s %s", res.Request.Method, res.Status, strings.TrimSpace(string(body)))
}

func (s *S3) do(ctx context.Context, method, key string, body []byte) (*http.Response, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimRight(s.Endpoint, "/") + "/" + s.Bucket + "/" + key)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body == nil {
		req.Body, req.ContentLength = http.NoBody, 0
	}
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	s.sign(req, body, now().UTC())
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return client.Do(req)
}

// sign adds the SigV4 Authorization header for the s3 service.
func (s *S3) sign(req *http.Request, body []byte, at time.Time) {
	payloadHash := sha256Hex(body)
	amzDate := at.Format("20060102T150405Z")
	day := at.Format("20060102")
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	headers := map[string]string{"host": req.URL.Host}
	for name, values := range req.Header {
		headers[strings.ToLower(name)] = strings.TrimSpace(strings.Join(values, ","))
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, name := range names {
		canonHeaders.WriteString(name + ":" + headers[name] + "\n")
	}
	signed := strings.Join(names, ";")
	canonical := strings.Join([]string{
		req.Method, req.URL.EscapedPath(), req.URL.Query().Encode(), canonHeaders.String(), signed, payloadHash,
	}, "\n")
	scope := day + "/" + s.Region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex([]byte(canonical))
	key := hmacSHA256([]byte("AWS4"+s.SecretKey), day)
	key = hmacSHA256(key, s.Region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.AccessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+signature)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}
