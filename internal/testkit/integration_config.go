package testkit

import "time"

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

// Integration holds explicit test configuration. By default it is empty and
// integration tests will skip. Local setups can override it from another file
// in the same package without relying on environment variables.
var Integration = IntegrationConfig{
	Typesense: TypesenseIntegrationConfig{
		ConnectionTimeout: 5 * time.Second,
	},
}
