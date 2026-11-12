// Package policy contains identity rules shared by application entry points.
package policy

import (
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/raulaguila/dataset/pkg/apperror"
)

const (
	maxEmailBytes    = 254
	minPasswordBytes = 8
	maxPasswordBytes = 128
)

// ValidateIdentity returns the name and email a new account stores.
func ValidateIdentity(rawName, rawEmail string) (name, email string, err error) {
	if name, err = ValidateName(rawName); err != nil {
		return "", "", err
	}
	email = NormalizeEmail(rawEmail)
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || email != strings.ToLower(strings.TrimSpace(rawEmail)) {
		return "", "", apperror.New(apperror.CodeInvalidInput, "invalid email").WithField("email")
	}
	return name, email, nil
}

// ValidateName returns the name an account stores.
func ValidateName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if n := utf8.RuneCountInString(name); n < 2 || n > 80 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", apperror.New(apperror.CodeInvalidInput, "name must be 2-80 characters").WithField("name")
	}
	return name, nil
}

// NormalizeEmail makes an address safe to compare and to store: Postgres refuses
// NUL, and the email index refuses very long values, so a stored attempt never fails.
func NormalizeEmail(raw string) string {
	email := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(raw, "\x00", "")))
	if len(email) > maxEmailBytes {
		email = strings.ToValidUTF8(email[:maxEmailBytes], "")
	}
	return email
}

// ValidatePassword applies the password policy.
func ValidatePassword(plain string) error {
	if len(plain) < minPasswordBytes {
		return apperror.New(apperror.CodeInvalidInput, "password too short").WithField("password")
	}
	if len(plain) > maxPasswordBytes {
		return apperror.New(apperror.CodeInvalidInput, "password too long").WithField("password")
	}
	lower := strings.ToLower(plain)
	first, _ := utf8.DecodeRuneInString(lower)
	if commonPasswords[lower] || strings.TrimLeft(lower, string(first)) == "" {
		return apperror.New(apperror.CodeInvalidInput, "password too common, choose another").WithField("password")
	}
	return nil
}

// ValidatePasswordFor also refuses the account's own name or email as password.
func ValidatePasswordFor(plain, name, email string) error {
	if err := ValidatePassword(plain); err != nil {
		return err
	}
	lower := strings.ToLower(plain)
	local, _, _ := strings.Cut(strings.ToLower(email), "@")
	if lower == strings.ToLower(email) || lower == local || lower == strings.ToLower(name) {
		return apperror.New(apperror.CodeInvalidInput, "password cannot be your name or email").WithField("password")
	}
	return nil
}

// commonPasswords are the first guesses of any attack that fit the length rule.
// shortcut: a short fixed list, swap for a breached-password check if accounts get guessed.
var commonPasswords = map[string]bool{
	"password": true, "password1": true, "password123": true, "passw0rd": true, "p@ssw0rd": true,
	"12345678": true, "123456789": true, "1234567890": true, "87654321": true, "12341234": true, "123123123": true,
	"qwertyui": true, "qwertyuiop": true, "qwerty123": true, "asdfghjk": true, "1q2w3e4r": true, "1qaz2wsx": true, "zaq12wsx": true,
	"abc12345": true, "abcd1234": true, "iloveyou": true, "trustno1": true, "letmein1": true, "welcome1": true, "changeme": true,
	"admin123": true, "administrator": true, "master123": true, "superman": true, "football": true, "baseball": true,
	"sunshine": true, "princess": true, "starwars": true, "computer": true, "internet": true,
	"senha123": true, "senha1234": true, "mudar123": true, "brasil123": true,
}
