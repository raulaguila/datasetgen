// Package oauth lets an assistant that cannot take a pasted key (the ChatGPT app) get one:
// it registers, the member approves, and the code becomes a key.
package oauth

import (
	"errors"
	"time"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/apikey"

	"github.com/gofiber/fiber/v3"

	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/apperror"
)

type Handler struct {
	svc input.IntegrationService
}

func New(svc input.IntegrationService) *Handler {
	return &Handler{svc: svc}
}

func fail(c fiber.Ctx, err error) error { return mapper.WriteError(c, err) }

func badBody(c fiber.Ctx) error {
	return mapper.WriteError(c, apperror.New(apperror.CodeInvalidInput, "invalid body"))
}

// Routes mounts on /v1/oauth. register and token are public (the
// assistant calls them, rate limited); client and approve are the member's
// consent page, behind the session.
func (h *Handler) Routes(r fiber.Router, session, limit fiber.Handler) {
	r.Post("/register", limit, h.OAuthRegister)
	r.Post("/token", limit, h.OAuthToken)
	r.Get("/client", session, h.OAuthClient)
	r.Post("/approve", session, h.OAuthApprove)
}

type oauthRegisterDTO struct {
	ClientName   string   `json:"client_name" example:"ChatGPT"`
	RedirectURIs []string `json:"redirect_uris"`
} // @name OAuthRegisterInput

type oauthClientDTO struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
} // @name OAuthClient

// oauthError answers in the OAuth form (RFC 6749 5.2), which assistants read.
func oauthError(c fiber.Ctx, code string, err error) error {
	var app *apperror.Error
	msg := "request refused"
	if errors.As(err, &app) {
		msg = app.Message
	} else if err != nil {
		return fail(c, err)
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": code, "error_description": msg})
}

// @Summary Register an OAuth client (RFC 7591)
// @Description Public. Nothing is stored: client_id carries the client's name and redirect URIs, sealed. A redirect URI is https, or http on localhost.
// @Tags OAuth
// @Accept json
// @Produce json
// @Param body body oauthRegisterDTO true "Client"
// @Success 201 {object} oauthClientDTO
// @Failure 400 {object} object{error=string,error_description=string}
// @Failure 429 {object} mapper.errorBody
// @ID oauthRegister
// @Router /v1/oauth/register [post]
func (h *Handler) OAuthRegister(c fiber.Ctx) error {
	var body oauthRegisterDTO
	if err := c.Bind().Body(&body); err != nil {
		return oauthError(c, "invalid_client_metadata", nil)
	}
	id, err := h.svc.RegisterOAuthClient(c.Context(), body.ClientName, body.RedirectURIs)
	if err != nil {
		code := "invalid_client_metadata"
		if apperror.IsCode(err, apperror.CodeInvalidInput) && len(body.RedirectURIs) > 0 {
			code = "invalid_redirect_uri"
		}
		return oauthError(c, code, err)
	}
	name, _ := h.svc.OAuthClient(c.Context(), id, body.RedirectURIs[0])
	return c.Status(fiber.StatusCreated).JSON(oauthClientDTO{
		ClientID: id, ClientName: name, RedirectURIs: body.RedirectURIs, GrantTypes: []string{"authorization_code"},
		ResponseTypes: []string{"code"}, TokenEndpointAuthMethod: "none",
	})
}

// @Summary The client a consent page asks for
// @Tags OAuth
// @Produce json
// @Param client_id query string true "Client ID"
// @Param redirect_uri query string true "Redirect URI the client registered"
// @Security BearerAuth
// @Success 200 {object} object{client_name=string}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID oauthClient
// @Router /v1/oauth/client [get]
func (h *Handler) OAuthClient(c fiber.Ctx) error {
	name, err := h.svc.OAuthClient(c.Context(), c.Query("client_id"), c.Query("redirect_uri"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(fiber.Map{"client_name": name})
}

type oauthApproveDTO struct {
	ClientID            string            `json:"client_id"`
	RedirectURI         string            `json:"redirect_uri"`
	CodeChallenge       string            `json:"code_challenge"`
	CodeChallengeMethod string            `json:"code_challenge_method" example:"S256"`
	Name                string            `json:"name" example:"ChatGPT"`
	Scopes              []apikey.ScopeDTO `json:"scopes"`
	ExpiresInDays       int               `json:"expires_in_days" example:"90"`
} // @name OAuthApproveInput

// @Summary Approve an assistant's access; returns the code for its redirect
// @Description The key is made only when the assistant trades the code (POST /v1/oauth/token), with these collections and validity. The code lasts 5 minutes.
// @Tags OAuth
// @Accept json
// @Produce json
// @Param body body oauthApproveDTO true "Approval"
// @Security BearerAuth
// @Success 200 {object} object{code=string}
// @Failure 400 {object} mapper.errorBody
// @Failure 401 {object} mapper.errorBody
// @Failure 403 {object} mapper.errorBody
// @Failure 429 {object} mapper.errorBody
// @ID oauthApprove
// @Router /v1/oauth/approve [post]
func (h *Handler) OAuthApprove(c fiber.Ctx) error {
	var body oauthApproveDTO
	if err := c.Bind().Body(&body); err != nil {
		return badBody(c)
	}
	code, err := h.svc.ApproveOAuth(c.Context(), input.OAuthGrant{
		ClientID: body.ClientID, RedirectURI: body.RedirectURI, Challenge: body.CodeChallenge, ChallengeMethod: body.CodeChallengeMethod,
		Name: body.Name, Scopes: apikey.KeyScopes(body.Scopes), Days: body.ExpiresInDays,
	})
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(fiber.Map{"code": code})
}

// @Summary Trade an approved code for an access token (an API key)
// @Description Public, form encoded: grant_type=authorization_code, code, redirect_uri, client_id, code_verifier (PKCE S256). The token is a dsk_ API key; it shows in the member's keys and ends when the member picked.
// @Tags OAuth
// @Accept x-www-form-urlencoded
// @Produce json
// @Param grant_type formData string true "authorization_code"
// @Param code formData string true "Code from the redirect"
// @Param redirect_uri formData string true "Same redirect URI"
// @Param client_id formData string true "Client ID"
// @Param code_verifier formData string true "PKCE verifier"
// @Success 200 {object} object{access_token=string,token_type=string,expires_in=int}
// @Failure 400 {object} object{error=string,error_description=string}
// @Failure 429 {object} mapper.errorBody
// @ID oauthToken
// @Router /v1/oauth/token [post]
func (h *Handler) OAuthToken(c fiber.Ctx) error {
	if c.FormValue("grant_type") != "authorization_code" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unsupported_grant_type", "error_description": "only authorization_code"})
	}
	k, err := h.svc.ExchangeOAuthCode(c.Context(), c.FormValue("code"), c.FormValue("client_id"), c.FormValue("redirect_uri"), c.FormValue("code_verifier"))
	if err != nil {
		return oauthError(c, "invalid_grant", err)
	}
	out := fiber.Map{"access_token": k.Secret, "token_type": "Bearer"}
	if k.Key.ExpiresAt != nil {
		out["expires_in"] = int(time.Until(*k.Key.ExpiresAt).Seconds())
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(out)
}
