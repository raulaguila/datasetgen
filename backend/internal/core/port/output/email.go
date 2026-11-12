package output

// EmailToken is the delivery intent; the codec protects Token at rest.
type EmailToken struct {
	Email string
	Token string
	Lang  string // the user's language for the email text; empty means the default
}

// EmailTokenCodec owns the durable wire format and secret protection.
type EmailTokenCodec interface {
	Encode(EmailToken) ([]byte, error)
	Decode([]byte) (EmailToken, error)
}
