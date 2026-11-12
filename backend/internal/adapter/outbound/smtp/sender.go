package smtpmail

import (
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/raulaguila/dataset/config"
	"github.com/raulaguila/dataset/pkg/smtpsecurity"
)

const (
	dialTimeout = 15 * time.Second
	sendTimeout = 30 * time.Second
)

// Sender sends plain text email via SMTP.
type Sender struct {
	cfg config.SMTPConfig
}

func NewSender(cfg config.SMTPConfig) *Sender {
	return &Sender{cfg: cfg}
}

func (s *Sender) Enabled() bool {
	return s.cfg.Enabled
}

func (s *Sender) Send(to, subject, body string) error {
	if s.cfg.Host == "" || s.cfg.From == "" {
		return fmt.Errorf("smtp_misconfigured")
	}

	conn, err := s.dial(net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port)))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	// One stalled server must not freeze the whole queue.
	if err := conn.SetDeadline(time.Now().Add(sendTimeout)); err != nil {
		return err
	}

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if s.cfg.Security == smtpsecurity.ModeStartTLS {
		// No fallback: a network attacker can hide the STARTTLS offer.
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("smtp server does not offer STARTTLS")
		}
		if err := client.StartTLS(s.tlsConfig()); err != nil {
			return err
		}
	}
	if s.cfg.User != "" {
		if err := client.Auth(smtp.PlainAuth("", s.cfg.User, s.cfg.Password, s.cfg.Host)); err != nil {
			return err
		}
	}
	if err := client.Mail(s.cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(buildMessage(s.cfg.From, to, s.cfg.ReplyTo, subject, body)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (s *Sender) dial(addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: dialTimeout}
	if s.cfg.Security == smtpsecurity.ModeTLS {
		return tls.DialWithDialer(dialer, "tcp", addr, s.tlsConfig())
	}
	return dialer.Dial("tcp", addr)
}

func (s *Sender) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
}

func buildMessage(from, to, replyTo, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	if replyTo != "" {
		b.WriteString("Reply-To: " + replyTo + "\r\n")
	}
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n") // headers are ASCII; plain ASCII passes unchanged
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	domain := strings.TrimRight(from[strings.LastIndex(from, "@")+1:], ">")
	b.WriteString("Message-ID: <" + uuid.NewString() + "@" + domain + ">\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}
