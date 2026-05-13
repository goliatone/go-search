package testkit

import (
	"os"
	"strings"
	"time"
)

const (
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
			ConnectionTimeout: 5 * time.Second,
		},
		Postgres: PostgresIntegrationConfig{
			DSN: strings.TrimSpace(os.Getenv(EnvPostgresDSN)),
		},
	}
}
