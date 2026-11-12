package smtpmail

import (
	"bufio"
	"net"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/config"
	"github.com/raulaguila/dataset/pkg/smtpsecurity"
)

// fakeServer answers one SMTP session without offering STARTTLS and reports
// whether a message body arrived.
func fakeServer(t *testing.T) (host string, port int, gotData chan bool) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	gotData = make(chan bool, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		data := false
		defer func() { gotData <- data }()
		r := bufio.NewReader(conn)
		_, _ = conn.Write([]byte("220 fake\r\n"))
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "DATA"):
				data = true
				_, _ = conn.Write([]byte("354 go\r\n"))
				for line != ".\r\n" {
					if line, err = r.ReadString('\n'); err != nil {
						return
					}
				}
				_, _ = conn.Write([]byte("250 ok\r\n"))
			case strings.HasPrefix(cmd, "QUIT"):
				_, _ = conn.Write([]byte("221 bye\r\n"))
				return
			default:
				_, _ = conn.Write([]byte("250 ok\r\n"))
			}
		}
	}()
	h, p, _ := net.SplitHostPort(ln.Addr().String())
	port, _ = strconv.Atoi(p)
	return h, port, gotData
}

func TestStartTLSRequiredIsNotDowngraded(t *testing.T) {
	host, port, gotData := fakeServer(t)
	s := NewSender(config.SMTPConfig{Enabled: true, Host: host, Port: port, From: "a@acme.io", Security: smtpsecurity.ModeStartTLS})
	if err := s.Send("b@acme.io", "subject", "secret token"); err == nil {
		t.Fatal("expected refusal without STARTTLS")
	}
	if <-gotData {
		t.Fatal("message went out in the clear")
	}
}

func TestPlainModeDelivers(t *testing.T) {
	host, port, gotData := fakeServer(t)
	s := NewSender(config.SMTPConfig{Enabled: true, Host: host, Port: port, From: "a@acme.io", Security: smtpsecurity.ModeNone})
	if err := s.Send("b@acme.io", "subject", "body"); err != nil {
		t.Fatal(err)
	}
	if !<-gotData {
		t.Fatal("no message received")
	}
}

func TestMessageCarriesDateAndID(t *testing.T) {
	msg := string(buildMessage("Dataset <no-reply@acme.io>", "a@acme.io", "", "Hi", "body"))
	if !strings.Contains(msg, "\r\nDate: ") {
		t.Fatalf("no Date in %q", msg)
	}
	if !regexp.MustCompile(`\r\nMessage-ID: <[0-9a-f-]{36}@acme\.io>\r\n`).MatchString(msg) {
		t.Fatalf("no Message-ID in %q", msg)
	}
}
