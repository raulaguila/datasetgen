// Package webhook POSTs webhook deliveries over HTTP.
package webhook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/raulaguila/dataset/internal/core/port/output"
)

// errPrivate refuses a connection inside the network: a hook must not reach
// the database, the metadata service or other internal hosts (SSRF).
var errPrivate = errors.New("webhook address is private")

// Sender delivers with a short timeout, no redirects and, unless AllowPrivate,
// only to public addresses, checked on the resolved IP at connect time so a
// name that later resolves inside the network is refused too.
type Sender struct {
	client *http.Client
}

var _ output.WebhookSender = (*Sender)(nil)

func New(allowPrivate bool) *Sender {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if !allowPrivate {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(host); ip == nil || !Public(ip) {
				return errPrivate
			}
			return nil
		}
	}
	transport := &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, Proxy: nil}
	return &Sender{client: &http.Client{
		Transport:     transport,
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Public is false for loopback, private, link-local, unspecified and multicast addresses.
func Public(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() ||
		// 100.64.0.0/10 (carrier-grade NAT) is not in IsPrivate.
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xC0 == 64))
}

func (s *Sender) Send(ctx context.Context, url string, body []byte, headers map[string]string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "dataset-webhook/1")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := s.client.Do(req)
	if err != nil {
		if errors.Is(err, errPrivate) {
			return 0, errPrivate
		}
		return 0, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64*1024))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return res.StatusCode, fmt.Errorf("answered %d", res.StatusCode)
	}
	return res.StatusCode, nil
}
