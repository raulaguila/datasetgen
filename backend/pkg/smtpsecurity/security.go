package smtpsecurity

import (
	"fmt"
	"strings"
)

type Mode string

const (
	ModeNone     Mode = "none"
	ModeTLS      Mode = "tls"
	ModeStartTLS Mode = "starttls"
)

func normalize(value string) Mode {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return ModeNone
	}
	return Mode(normalized)
}

func Parse(value string) (Mode, error) {
	mode := normalize(value)
	if !mode.Valid() {
		return "", fmt.Errorf("invalid SMTP_SECURITY %q: must be one of none, tls, starttls", value)
	}
	return mode, nil
}

func (m Mode) Valid() bool {
	switch m {
	case ModeNone, ModeTLS, ModeStartTLS:
		return true
	default:
		return false
	}
}

func (m Mode) UsesTLS() bool {
	switch m {
	case ModeTLS, ModeStartTLS:
		return true
	default:
		return false
	}
}
