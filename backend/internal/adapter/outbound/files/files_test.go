package files

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/raulaguila/dataset/pkg/apperror"
)

func TestLocalRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := &Local{Dir: t.TempDir()}
	if err := store.Put(ctx, "avatars/u1", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "avatars/u1", []byte("two")); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(ctx, "avatars/u1"); err != nil || string(got) != "two" {
		t.Fatalf("get %q %v", got, err)
	}
	if err := store.Delete(ctx, "avatars/u1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "avatars/u1"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := store.Get(ctx, "avatars/u1"); !apperror.IsCode(err, apperror.CodeNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	for _, key := range []string{"../etc/passwd", "avatars/../../x", "/abs", "", "a//b"} {
		if err := store.Put(ctx, key, nil); err == nil {
			t.Errorf("%q accepted", key)
		}
	}
}

// The GET Object example from the AWS Signature Version 4 documentation.
func TestS3SignatureMatchesTheAWSExample(t *testing.T) {
	s := &S3{Region: "us-east-1", AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}
	req, _ := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	s.sign(req, nil, time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	auth := req.Header.Get("Authorization")
	want := "Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if !strings.HasSuffix(auth, want) {
		t.Fatalf("got %s", auth)
	}
}

// Runs against a real S3-compatible server when TEST_S3_ENDPOINT is set
// (with TEST_S3_ACCESS_KEY and TEST_S3_SECRET_KEY); it creates its own bucket.
func TestS3AgainstServer(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT not set")
	}
	ctx := context.Background()
	s := &S3{Endpoint: endpoint, Region: "us-east-1", Bucket: "files-test", AccessKey: os.Getenv("TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("TEST_S3_SECRET_KEY")}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, endpoint+"/files-test", http.NoBody)
	s.sign(req, nil, time.Now().UTC())
	if res, err := http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	} else {
		_ = res.Body.Close() // 200, or 409 when it already exists
	}
	if err := s.Put(ctx, "avatars/u1", []byte("image")); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(ctx, "avatars/u1"); err != nil || string(got) != "image" {
		t.Fatalf("get %q %v", got, err)
	}
	if err := s.Delete(ctx, "avatars/u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "avatars/u1"); !apperror.IsCode(err, apperror.CodeNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	wrong := *s
	wrong.SecretKey = "wrong"
	if err := wrong.Put(ctx, "avatars/u2", []byte("x")); err == nil {
		t.Fatal("a wrong secret was accepted")
	}
}
