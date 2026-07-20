package core

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/goliatone/go-search/examples/search-shell/internal/config"

	"github.com/goliatone/go-admin/pkg/admin"
	"github.com/goliatone/go-admin/quickstart"
	auth "github.com/goliatone/go-auth"
	"github.com/goliatone/go-router"
)

type DemoCredential struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
}

type DemoIdentity struct {
	id       string
	username string
	email    string
	role     string
}

func (i DemoIdentity) ID() string       { return i.id }
func (i DemoIdentity) Username() string { return i.username }
func (i DemoIdentity) Email() string    { return i.email }
func (i DemoIdentity) Role() string     { return i.role }

type demoIdentityProvider struct {
	credentials []DemoCredential
}

func (p *demoIdentityProvider) VerifyIdentity(_ context.Context, identifier, password string) (auth.Identity, error) {
	credential, err := p.findCredential(identifier)
	if err != nil {
		return nil, err
	}
	if err := auth.ComparePasswordAndHash(password, credential.PasswordHash); err != nil {
		return nil, auth.ErrMismatchedHashAndPassword
	}
	return identityFromCredential(credential), nil
}

func (p *demoIdentityProvider) FindIdentityByIdentifier(_ context.Context, identifier string) (auth.Identity, error) {
	credential, err := p.findCredential(identifier)
	if err != nil {
		return nil, err
	}
	return identityFromCredential(credential), nil
}

func (p *demoIdentityProvider) findCredential(identifier string) (DemoCredential, error) {
	if p == nil || len(p.credentials) == 0 {
		return DemoCredential{}, auth.ErrIdentityNotFound
	}
	target := strings.ToLower(strings.TrimSpace(identifier))
	if target == "" {
		return DemoCredential{}, auth.ErrIdentityNotFound
	}
	for _, credential := range p.credentials {
		if strings.EqualFold(target, credential.ID) ||
			strings.EqualFold(target, credential.Username) ||
			strings.EqualFold(target, credential.Email) {
			return credential, nil
		}
	}
	return DemoCredential{}, auth.ErrIdentityNotFound
}

type authRuntimeConfig struct {
	signingKey           string
	contextKey           string
	issuer               string
	audience             []string
	rejectedRouteDefault string
}

func (c authRuntimeConfig) GetSigningKey() string         { return c.signingKey }
func (c authRuntimeConfig) GetSigningMethod() string      { return "HS256" }
func (c authRuntimeConfig) GetContextKey() string         { return c.contextKey }
func (c authRuntimeConfig) GetTokenExpiration() int       { return 24 }
func (c authRuntimeConfig) GetExtendedTokenDuration() int { return 72 }
func (c authRuntimeConfig) GetTokenLookup() string {
	return fmt.Sprintf("header:Authorization,cookie:%s", c.GetContextKey())
}
func (c authRuntimeConfig) GetAuthScheme() string           { return "Bearer" }
func (c authRuntimeConfig) GetIssuer() string               { return c.issuer }
func (c authRuntimeConfig) GetAudience() []string           { return c.audience }
func (c authRuntimeConfig) GetRejectedRouteKey() string     { return "search_shell_reject" }
func (c authRuntimeConfig) GetRejectedRouteDefault() string { return c.rejectedRouteDefault }

func setupAuth(adm *admin.Admin, cfg *config.AppConfig) (*auth.Auther, *auth.RouteAuthenticator, *admin.GoAuthAuthenticator, DemoIdentity, string, authRuntimeConfig, error) {
	if adm == nil {
		return nil, nil, nil, DemoIdentity{}, "", authRuntimeConfig{}, fmt.Errorf("admin instance is required")
	}
	if cfg == nil {
		return nil, nil, nil, DemoIdentity{}, "", authRuntimeConfig{}, fmt.Errorf("config is required")
	}

	credentials, err := seedDemoCredentials(cfg)
	if err != nil {
		return nil, nil, nil, DemoIdentity{}, "", authRuntimeConfig{}, err
	}
	if len(credentials) == 0 {
		return nil, nil, nil, DemoIdentity{}, "", authRuntimeConfig{}, fmt.Errorf("no demo credentials configured")
	}
	primaryCredential := credentials[0]
	demoIdentity := identityFromCredential(primaryCredential)
	provider := &demoIdentityProvider{credentials: credentials}

	authCfg := authRuntimeConfig{
		signingKey:           strings.TrimSpace(cfg.Auth.SigningKey),
		contextKey:           "search_shell_user",
		issuer:               strings.TrimSpace(cfg.Name),
		audience:             []string{"go-admin"},
		rejectedRouteDefault: path.Join(cfg.Admin.BasePath, "login"),
	}
	if authCfg.issuer == "" {
		authCfg.issuer = "go-search-shell"
	}

	auther := auth.NewAuthenticator(provider, authCfg)
	routeAuth, err := auth.NewHTTPAuthenticator(auther, authCfg)
	if err != nil {
		return nil, nil, nil, DemoIdentity{}, "", authRuntimeConfig{}, err
	}

	loginPath := path.Join(cfg.Admin.BasePath, "login")
	goAuth, _ := quickstart.WithGoAuth(
		adm,
		routeAuth,
		authCfg,
		admin.GoAuthAuthorizerConfig{DefaultResource: "admin"},
		&admin.AuthConfig{
			LoginPath:    loginPath,
			LogoutPath:   path.Join(cfg.Admin.BasePath, "logout"),
			RedirectPath: cfg.Admin.BasePath,
		},
		admin.WithAuthErrorHandler(makeAuthErrorHandler(loginPath)),
	)

	return auther, routeAuth, goAuth, demoIdentity, authCfg.GetContextKey(), authCfg, nil
}

// DemoRouteProtection authenticates demo operations and adds origin/CSRF
// protection whenever a browser session cookie is used.
func (c *Core) DemoRouteProtection() (router.MiddlewareFunc, error) {
	if c == nil || c.RouteAuthenticator == nil || strings.TrimSpace(c.routeAuthConfig.signingKey) == "" {
		return nil, fmt.Errorf("demo route authentication is not configured")
	}
	return c.RouteAuthenticator.ProtectedBrowserRoute(
		c.routeAuthConfig,
		makeAuthErrorHandler(path.Join(c.Config.Admin.BasePath, "login")),
		auth.BrowserProtectionConfig{AuthCookieName: c.AuthCookieName},
	), nil
}

func makeAuthErrorHandler(loginPath string) func(router.Context, error) error {
	return func(c router.Context, _ error) error {
		if strings.Contains(c.Path(), "/api/") {
			return c.JSON(http.StatusUnauthorized, map[string]any{
				"error": "unauthorized",
			})
		}
		if strings.TrimSpace(loginPath) == "" {
			loginPath = "/login"
		}
		return c.Redirect(loginPath, http.StatusFound)
	}
}

func seedDemoCredentials(cfg *config.AppConfig) ([]DemoCredential, error) {
	username := "admin"
	email := "admin@example.com"
	password := ""
	if cfg != nil {
		if strings.TrimSpace(cfg.Auth.DemoUsername) != "" {
			username = strings.TrimSpace(cfg.Auth.DemoUsername)
		}
		if strings.TrimSpace(cfg.Auth.DemoEmail) != "" {
			email = strings.TrimSpace(cfg.Auth.DemoEmail)
		}
		if strings.TrimSpace(cfg.Auth.DemoPassword) != "" {
			password = strings.TrimSpace(cfg.Auth.DemoPassword)
		}
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash demo password: %w", err)
	}
	return []DemoCredential{
		{
			ID:           "demo-admin",
			Username:     username,
			Email:        email,
			PasswordHash: passwordHash,
			Role:         string(auth.RoleAdmin),
		},
	}, nil
}

func identityFromCredential(credential DemoCredential) DemoIdentity {
	id := strings.TrimSpace(credential.ID)
	if id == "" {
		id = "demo-" + strings.ToLower(strings.TrimSpace(credential.Username))
	}
	return DemoIdentity{
		id:       id,
		username: strings.TrimSpace(credential.Username),
		email:    strings.TrimSpace(credential.Email),
		role:     strings.TrimSpace(credential.Role),
	}
}
