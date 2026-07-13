package testkit

import (
	"os"
	"strings"
	"time"
)

const (
	EnvTypesenseURL    = "GO_SEARCH_TEST_TYPESENSE_URL"
	EnvTypesenseAPIKey = "GO_SEARCH_TEST_TYPESENSE_API_KEY"
	// EnvPostgresDSN enables Postgres integration tests without requiring a
	// repo-local override file.
	EnvPostgresDSN = "GO_SEARCH_TEST_POSTGRES_DSN"
)

type IntegrationConfig struct {
	Typesense TypesenseIntegrationConfig
	Postgres  PostgresIntegrationConfig
}

type TypesenseIntegrationConfig struct {
	ServerURL         string
	APIKey            string
	ConnectionTimeout time.Duration
}

type PostgresIntegrationConfig struct {
	DSN string
}

// Integration holds explicit test configuration. By default integration tests
// skip unless configured through environment variables or a package-local test
// override file.
var Integration = integrationConfigFromEnv()

func integrationConfigFromEnv() IntegrationConfig {
	return IntegrationConfig{
		Typesense: TypesenseIntegrationConfig{
			ServerURL:         strings.TrimSpace(os.Getenv(EnvTypesenseURL)),
			APIKey:            strings.TrimSpace(os.Getenv(EnvTypesenseAPIKey)),
			ConnectionTimeout: 5 * time.Second,
		},
		Postgres: PostgresIntegrationConfig{
			DSN: strings.TrimSpace(os.Getenv(EnvPostgresDSN)),
		},
	}
}
