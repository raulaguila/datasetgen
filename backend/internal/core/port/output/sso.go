package output

import "context"

// SingleSignOn is an OpenID Connect provider that vouches for an email.
type SingleSignOn interface {
	// Name is the provider label for the login button.
	Name() string
	// AuthURL is where the browser signs in; verifier is the PKCE secret.
	AuthURL(ctx context.Context, state, nonce, verifier string) (string, error)
	// Email trades the code for a checked ID token and returns its verified email.
	Email(ctx context.Context, code, verifier, nonce string) (string, error)
}
