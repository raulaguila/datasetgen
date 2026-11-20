package rest

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/raulaguila/dataset/config"
	apikeyhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/apikey"
	audithttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/audit"
	authhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/auth"
	automationhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/automation"
	collectionhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/collection"
	filehttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/file"
	healthhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/health"
	mcptoolhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/mcptool"
	notificationhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/notification"
	oauthhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/oauth"
	preferencehttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/preference"
	publichttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/public"
	recordhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/record"
	sessionhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/session"
	sharehttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/share"
	trashhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/trash"
	usershttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/user"
	viewhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/view"
	webhookhttp "github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/handler/webhook"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/mapper"
	"github.com/raulaguila/dataset/internal/adapter/inbound/http/rest/middleware"
	"github.com/raulaguila/dataset/internal/core/domain"
	"github.com/raulaguila/dataset/internal/core/port/input"
	"github.com/raulaguila/dataset/pkg/logger"
)

// maxBodyBytes bounds what a client makes the server buffer: the body is read
// before any middleware runs, rate limit included. The largest route is an
// attached file (domain.FileMaxBytes); larger uploads need their own listener
// or a streamed body, not a higher limit for every route.
const maxBodyBytes = domain.FileMaxBytes + 4*1024

func serverConfig(api config.APIConfig) fiber.Config {
	cfg := fiber.Config{
		AppName:            "dataset",
		ReadTimeout:        api.ReadTimeout,
		WriteTimeout:       api.WriteTimeout,
		IdleTimeout:        api.IdleTimeout,
		BodyLimit:          maxBodyBytes,
		ErrorHandler:       mapper.ErrorHandler,
		EnableIPValidation: true, // c.IP() goes into an inet column
	}
	if api.TrustedProxies != "" {
		cfg.ProxyHeader = api.ProxyHeader
		cfg.TrustProxy = true
		cfg.TrustProxyConfig.Proxies = splitList(api.TrustedProxies)
	}
	return cfg
}

func splitList(list string) []string {
	return strings.Split(strings.ReplaceAll(list, " ", ""), ",")
}

// New builds the Fiber app with middleware and routes.
func New(cfg *config.Environment, app *input.Application, hits middleware.Hits, log *logger.Logger) *fiber.App {
	fiberApp := fiber.New(serverConfig(cfg.API))

	fiberApp.Use(
		middleware.RequestID(),
		middleware.RequestLog(log, cfg.API.EnableLogger),
		recover.New(recover.Config{EnableStackTrace: true}),
		cors.New(cors.Config{
			AllowOrigins:  splitList(cfg.API.CorsOrigins),
			AllowMethods:  []string{fiber.MethodGet, fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete, fiber.MethodOptions},
			AllowHeaders:  []string{fiber.HeaderAuthorization, fiber.HeaderContentType, fiber.HeaderAccept, fiber.HeaderAcceptLanguage, fiber.HeaderOrigin},
			ExposeHeaders: []string{fiber.HeaderContentType, fiber.HeaderContentLength, fiber.HeaderXRequestID, fiber.HeaderRetryAfter},
			MaxAge:        3600,
		}),
		middleware.Timeout(cfg.API.RequestTimeout),
	)

	fiberApp.Get("/health", healthhttp.NewHandler(app.Health).Get) // before the limiter: probes must always answer
	if !cfg.Protected() {
		mountSwagger(fiberApp, cfg.Version) // never in production: it maps every route
	}

	fiberApp.Use(middleware.RateLimit("global", cfg.API.GlobalRateLimit, hits))
	bearer := middleware.Bearer(app.Auth)
	// Resource groups check the session inside Require, with the permission.
	token := middleware.Token(app.Auth)

	v1 := fiberApp.Group("/v1")

	passwordLimit := middleware.RateLimit("password", cfg.API.PasswordRateLimit, hits)
	authHandler := authhttp.New(app.Auth, app.Account)
	authHandler.Routes(v1.Group("/auth"), bearer, middleware.RateLimit("auth", cfg.API.AuthRateLimit, hits), passwordLimit)

	// Everything about the signed in user's own account.
	me := v1.Group("/me", bearer)
	authHandler.MeRoutes(me, passwordLimit)
	sessionhttp.New(app.Account).Routes(me.Group("/sessions"))
	preferencehttp.New(app.Account).Routes(me.Group("/preferences"))
	notificationhttp.New(app.Account).Routes(me.Group("/notifications"))
	apiKeys, mcpTools := apikeyhttp.New(app.Integrations), mcptoolhttp.New(app.Integrations)
	keys := me.Group("/keys")
	apiKeys.MeRoutes(keys)
	mcpTools.KeyRoutes(keys)
	// Assistants that cannot take a pasted key (the ChatGPT app) get one through OAuth.
	oauthhttp.New(app.Integrations).Routes(v1.Group("/oauth"), bearer, middleware.RateLimit("oauth", cfg.API.AuthRateLimit, hits))

	users := v1.Group("/users", token)
	usershttp.New(app.Users, app.Auth).Routes(users, app.Authorizer, passwordLimit)
	// Collections and records check the caller's role per collection, so they need the full session check.
	// Records and files also take an API key, capped to its collections (policy.CollectionRole).
	// A collection lives at /v1/collections/:owner/:slug, as its page does in the app.
	keyOrBearer := middleware.BearerOrKey(app.Auth, app.Integrations)
	at := middleware.CollectionPath(app.Collections)
	recordHandler := recordhttp.New(app.Records)
	// Registered before the collection group: its Bearer-only middleware would refuse a key.
	recordHandler.CollectionRoutes(v1.Group("/collections/:owner/:slug/records", keyOrBearer, at))
	collections := v1.Group("/collections", bearer)
	trash, files := trashhttp.New(app.Tools), filehttp.New(app.Tools)
	trash.Routes(collections) // before the :owner/:slug group, whose lookup would take "trash" for a username
	collectionAt := collections.Group("/:owner/:slug", at)
	collectionhttp.New(app.Collections, app.Tools).Routes(collections, collectionAt, app.Authorizer)
	trash.CollectionRoutes(collectionAt)
	files.CollectionRoutes(collectionAt)
	viewhttp.New(app.Tools).Routes(collectionAt)
	apiKeys.CollectionRoutes(collectionAt)
	sharehttp.New(app.Integrations).Routes(collectionAt)
	webhookhttp.New(app.Integrations).Routes(collectionAt)
	automationhttp.New(app.Integrations).Routes(collectionAt)
	mcpTools.Routes(collectionAt)
	recordHandler.Routes(v1.Group("/records", keyOrBearer))
	files.Routes(v1.Group("/files", keyOrBearer))
	audithttp.New(app.Audit).Routes(v1.Group("/audit", token), app.Authorizer)
	// Share links: no session; the token is the access. Form posts share the sign-in limit.
	publichttp.New(app.Integrations, app.Collections, app.Records).Routes(v1.Group("/public"), middleware.RateLimit("public", cfg.API.AuthRateLimit, hits))
	// AI assistants: MCP tools over the collections an API key reaches.
	mountMCP(fiberApp, app, cfg.Version)

	return fiberApp
}
