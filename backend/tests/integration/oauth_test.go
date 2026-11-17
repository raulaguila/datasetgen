package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
	"github.com/raulaguila/dataset/pkg/reqctx"
)

// An assistant registers, the member approves on the consent page, and the
// assistant trades the code (with its PKCE verifier) for a key that reaches MCP.
func TestOAuthGivesAnAssistantAKey(t *testing.T) {
	e := newEnv(t)
	collections, _ := collectionServices(e)
	keys, _ := toolServices(e, t)
	owner := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	c, err := collections.Create(owner, input.CreateCollectionCommand{Name: "Notas", Schema: json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	app := e.httpApp()
	defer func() { _ = app.Shutdown() }()
	send := func(method, path, ctype, body string, want int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", ctype)
		req.Header.Set("Accept", "application/json, text/event-stream")
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		data, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, res.StatusCode, want, data)
		}
		var out map[string]any
		_ = json.Unmarshal(data, &out)
		return out
	}

	const redirect = "https://chatgpt.com/connector_platform_oauth_redirect"
	if out := send("POST", "/v1/oauth/register", "application/json", `{"client_name":"x","redirect_uris":["http://evil.example/cb"]}`, 400); out["error"] != "invalid_redirect_uri" {
		t.Fatalf("plain http redirect: %v", out)
	}
	reg := send("POST", "/v1/oauth/register", "application/json", `{"client_name":"ChatGPT","redirect_uris":["`+redirect+`"]}`, 201)
	client, _ := reg["client_id"].(string)
	if client == "" || reg["token_endpoint_auth_method"] != "none" {
		t.Fatalf("register: %v", reg)
	}
	if name, err := keys.OAuthClient(owner, client, redirect); err != nil || name != "ChatGPT" {
		t.Fatalf("client: %q %v", name, err)
	}
	wantCode(t, ignore(keys.OAuthClient(owner, client, "https://other.example/cb")), apperror.CodeInvalidInput)

	verifier := strings.Repeat("v", 50)
	sum := sha256.Sum256([]byte(verifier))
	grant := input.OAuthGrant{
		ClientID: client, RedirectURI: redirect, Challenge: base64.RawURLEncoding.EncodeToString(sum[:]), ChallengeMethod: "S256",
		Name: "ChatGPT", Scopes: []domain.KeyScope{{CollectionID: c.ID}}, Days: 30,
	}
	plain := grant
	plain.ChallengeMethod = "plain"
	wantCode(t, ignore(keys.ApproveOAuth(owner, plain)), apperror.CodeInvalidInput)
	stranger := reqctx.WithActor(context.Background(), e.userID(t, e.activeUser(t)))
	if _, err := keys.ApproveOAuth(stranger, grant); err == nil {
		t.Fatal("a member approved a collection they cannot read")
	}
	code, err := keys.ApproveOAuth(owner, grant)
	if err != nil {
		t.Fatal(err)
	}

	token := func(verifier, redirect string, want int) map[string]any {
		form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {client}, "redirect_uri": {redirect}, "code_verifier": {verifier}}
		return send("POST", "/v1/oauth/token", "application/x-www-form-urlencoded", form.Encode(), want)
	}
	if out := token(strings.Repeat("w", 50), redirect, 400); out["error"] != "invalid_grant" {
		t.Fatalf("wrong verifier: %v", out)
	}
	token(verifier, "https://other.example/cb", 400)
	out := token(verifier, redirect, 200)
	secret, _ := out["access_token"].(string)
	if !strings.HasPrefix(secret, "dsk_") || out["token_type"] != "Bearer" || out["expires_in"].(float64) < 29*24*3600 {
		t.Fatalf("token: %v", out)
	}
	// The key shows among the member's keys and reaches the approved collection over MCP.
	mine, err := keys.MyKeys(owner)
	if err != nil || len(mine) != 1 || mine[0].Name != "ChatGPT" || mine[0].ExpiresAt == nil {
		t.Fatalf("keys: %+v %v", mine, err)
	}
	req := httptest.NewRequest("POST", "/v1/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"workspace_overview","arguments":{}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+secret)
	res, err := app.Test(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("mcp: %v %v", res, err)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), c.ID) {
		t.Fatalf("mcp list: %s", body)
	}
	if out := send("POST", "/v1/oauth/token", "application/x-www-form-urlencoded", "grant_type=password", 400); out["error"] != "unsupported_grant_type" {
		t.Fatalf("grant type: %v", out)
	}
}
