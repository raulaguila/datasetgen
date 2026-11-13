package webhook

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublic(t *testing.T) {
	for ip, want := range map[string]bool{"8.8.8.8": true, "127.0.0.1": false, "10.1.2.3": false, "192.168.0.1": false, "169.254.169.254": false, "100.64.0.1": false, "::1": false, "2001:4860::1": true} {
		if got := Public(net.ParseIP(ip)); got != want {
			t.Errorf("Public(%s) = %v, want %v", ip, got, want)
		}
	}
}

func TestSendRefusesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	if _, err := New(false).Send(context.Background(), srv.URL, []byte(`{}`), nil); err == nil {
		t.Fatal("a loopback hook must be refused")
	}
	if status, err := New(true).Send(context.Background(), srv.URL, []byte(`{}`), nil); err != nil || status != 204 {
		t.Fatalf("allowed private: %d, %v", status, err)
	}
}
