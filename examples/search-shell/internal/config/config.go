package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type AppConfig struct {
	Name       string            `json:"name"`
	Env        string            `json:"env"`
	Server     ServerConfig      `json:"server"`
	Admin      AdminConfig       `json:"admin"`
	Auth       AuthConfig        `json:"auth"`
	Features   FeatureConfig     `json:"features"`
	SearchDemo SearchDemoConfig  `json:"search_demo"`
	ConfigPath string            `json:"-"`
	Metadata   map[string]string `json:"-"`
}

type ServerConfig struct {
	Address     string `json:"address"`
	PrintRoutes bool   `json:"print_routes"`
}

type AdminConfig struct {
	BasePath      string `json:"base_path"`
	Title         string `json:"title"`
	DefaultLocale string `json:"default_locale"`
}

type AuthConfig struct {
	SigningKey   string `json:"signing_key"`
	DemoUsername string `json:"demo_username"`
	DemoEmail    string `json:"demo_email"`
	DemoPassword string `json:"demo_password"`
}

type FeatureConfig struct {
	Dashboard bool `json:"dashboard"`
	CMS       bool `json:"cms"`
	Search    bool `json:"search"`
	Commands  bool `json:"commands"`
	Settings  bool `json:"settings"`
	Jobs      bool `json:"jobs"`
	Media     bool `json:"media"`
	Users     bool `json:"users"`
}

type SearchDemoConfig struct {
	Provider                  string `json:"provider"`
	CacheEnabled              bool   `json:"cache_enabled"`
	EditorialEnabled          bool   `json:"editorial_enabled"`
	SeedOnStart               bool   `json:"seed_on_start"`
	IndexName                 string `json:"index_name"`
	DefaultLocale             string `json:"default_locale"`
	CultureDataPath           string `json:"culture_data_path"`
	PostgresDSN               string `json:"postgres_dsn"`
	TypesenseServerURL        string `json:"typesense_server_url"`
	TypesenseAPIKey           string `json:"typesense_api_key"`
	TypesenseCollectionPrefix string `json:"typesense_collection_prefix"`
	ReindexBatchSize          int    `json:"reindex_batch_size"`
}

func Defaults() AppConfig {
	return AppConfig{
		Name: "go-search shell",
		Env:  "development",
		Server: ServerConfig{
			Address:     ":8484",
			PrintRoutes: true,
		},
		Admin: AdminConfig{
			BasePath:      "/admin",
			Title:         "Search Shell",
			DefaultLocale: "en",
		},
		Auth: AuthConfig{
			SigningKey:   "search-shell-dev-signing-key",
			DemoUsername: "admin",
			DemoEmail:    "admin@example.com",
			DemoPassword: "admin.pwd",
		},
		Features: FeatureConfig{
			Dashboard: true,
			CMS:       false,
			Search:    true,
			Commands:  false,
			Settings:  false,
			Jobs:      false,
			Media:     false,
			Users:     false,
		},
		SearchDemo: SearchDemoConfig{
			Provider:                  "memory",
			CacheEnabled:              false,
			EditorialEnabled:          true,
			SeedOnStart:               true,
			IndexName:                 "media_transcripts",
			DefaultLocale:             "en",
			CultureDataPath:           "",
			TypesenseCollectionPrefix: "search_shell_",
			ReindexBatchSize:          25,
		},
		Metadata: map[string]string{},
	}
}

func Load() (AppConfig, error) {
	cfg := Defaults()

	configPath := strings.TrimSpace(firstNonEmpty(
		os.Getenv("APP_CONFIG"),
		os.Getenv("APP_CONFIG_PATH"),
		defaultConfigPath(),
	))
	cfg.ConfigPath = configPath

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return AppConfig{}, fmt.Errorf("read config %q: %w", configPath, err)
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return AppConfig{}, fmt.Errorf("decode config %q: %w", configPath, err)
		}
		cfg.ConfigPath = configPath
	}

	applyEnv(&cfg)
	normalize(&cfg)
	return cfg, nil
}

func (c AppConfig) FeatureDefaults() map[string]bool {
	return map[string]bool{
		"dashboard":                   c.Features.Dashboard,
		"activity":                    false,
		"preview":                     false,
		"cms":                         c.Features.CMS,
		"commands":                    c.Features.Commands,
		"settings":                    c.Features.Settings,
		"search":                      c.Features.Search,
		"notifications":               false,
		"jobs":                        c.Features.Jobs,
		"media":                       c.Features.Media,
		"export":                      false,
		"bulk":                        false,
		"preferences":                 false,
		"profile":                     false,
		"users":                       c.Features.Users,
		"tenants":                     false,
		"organizations":               false,
		"translations.exchange":       false,
		"translations.queue":          false,
		"translations.qa.style":       false,
		"translations.qa.terminology": false,
	}
}

func defaultConfigPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return "internal/config/app.json"
	}
	return filepath.Join(wd, "internal", "config", "app.json")
}

func applyEnv(cfg *AppConfig) {
	if cfg == nil {
		return
	}

	cfg.Name = envString("APP_NAME", cfg.Name)
	cfg.Env = envString("APP_ENV", cfg.Env)
	cfg.Server.Address = envString("APP_SERVER__ADDRESS", cfg.Server.Address)
	cfg.Server.PrintRoutes = envBool("APP_SERVER__PRINT_ROUTES", cfg.Server.PrintRoutes)
	cfg.Admin.BasePath = envString("APP_ADMIN__BASE_PATH", cfg.Admin.BasePath)
	cfg.Admin.Title = envString("APP_ADMIN__TITLE", cfg.Admin.Title)
	cfg.Admin.DefaultLocale = envString("APP_ADMIN__DEFAULT_LOCALE", cfg.Admin.DefaultLocale)
	cfg.Auth.SigningKey = envString("APP_AUTH__SIGNING_KEY", cfg.Auth.SigningKey)
	cfg.Auth.DemoUsername = envString("APP_AUTH__DEMO_USERNAME", cfg.Auth.DemoUsername)
	cfg.Auth.DemoEmail = envString("APP_AUTH__DEMO_EMAIL", cfg.Auth.DemoEmail)
	cfg.Auth.DemoPassword = envString("APP_AUTH__DEMO_PASSWORD", cfg.Auth.DemoPassword)

	cfg.Features.Dashboard = envBool("APP_FEATURES__DASHBOARD", cfg.Features.Dashboard)
	cfg.Features.CMS = envBool("APP_FEATURES__CMS", cfg.Features.CMS)
	cfg.Features.Search = envBool("APP_FEATURES__SEARCH", cfg.Features.Search)
	cfg.Features.Commands = envBool("APP_FEATURES__COMMANDS", cfg.Features.Commands)
	cfg.Features.Settings = envBool("APP_FEATURES__SETTINGS", cfg.Features.Settings)
	cfg.Features.Jobs = envBool("APP_FEATURES__JOBS", cfg.Features.Jobs)
	cfg.Features.Media = envBool("APP_FEATURES__MEDIA", cfg.Features.Media)
	cfg.Features.Users = envBool("APP_FEATURES__USERS", cfg.Features.Users)

	cfg.SearchDemo.Provider = envString("APP_SEARCH_DEMO__PROVIDER", cfg.SearchDemo.Provider)
	cfg.SearchDemo.CacheEnabled = envBool("APP_SEARCH_DEMO__CACHE_ENABLED", cfg.SearchDemo.CacheEnabled)
	cfg.SearchDemo.EditorialEnabled = envBool("APP_SEARCH_DEMO__EDITORIAL_ENABLED", cfg.SearchDemo.EditorialEnabled)
	cfg.SearchDemo.SeedOnStart = envBool("APP_SEARCH_DEMO__SEED_ON_START", cfg.SearchDemo.SeedOnStart)
	cfg.SearchDemo.IndexName = envString("APP_SEARCH_DEMO__INDEX_NAME", cfg.SearchDemo.IndexName)
	cfg.SearchDemo.DefaultLocale = envString("APP_SEARCH_DEMO__DEFAULT_LOCALE", cfg.SearchDemo.DefaultLocale)
	cfg.SearchDemo.CultureDataPath = envString("APP_SEARCH_DEMO__CULTURE_DATA_PATH", cfg.SearchDemo.CultureDataPath)
	cfg.SearchDemo.PostgresDSN = envString("APP_SEARCH_DEMO__POSTGRES_DSN", cfg.SearchDemo.PostgresDSN)
	cfg.SearchDemo.TypesenseServerURL = envString("APP_SEARCH_DEMO__TYPESENSE_SERVER_URL", cfg.SearchDemo.TypesenseServerURL)
	cfg.SearchDemo.TypesenseAPIKey = envString("APP_SEARCH_DEMO__TYPESENSE_API_KEY", cfg.SearchDemo.TypesenseAPIKey)
	cfg.SearchDemo.TypesenseCollectionPrefix = envString("APP_SEARCH_DEMO__TYPESENSE_COLLECTION_PREFIX", cfg.SearchDemo.TypesenseCollectionPrefix)
	cfg.SearchDemo.ReindexBatchSize = envInt("APP_SEARCH_DEMO__REINDEX_BATCH_SIZE", cfg.SearchDemo.ReindexBatchSize)
}

func normalize(cfg *AppConfig) {
	if cfg == nil {
		return
	}
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.Env = strings.TrimSpace(cfg.Env)
	cfg.Server.Address = strings.TrimSpace(cfg.Server.Address)
	cfg.Admin.BasePath = firstNonEmpty(strings.TrimSpace(cfg.Admin.BasePath), "/admin")
	cfg.Admin.Title = firstNonEmpty(strings.TrimSpace(cfg.Admin.Title), "Search Shell")
	cfg.Admin.DefaultLocale = firstNonEmpty(strings.TrimSpace(cfg.Admin.DefaultLocale), "en")
	cfg.Auth.SigningKey = firstNonEmpty(strings.TrimSpace(cfg.Auth.SigningKey), "search-shell-dev-signing-key")
	cfg.Auth.DemoUsername = firstNonEmpty(strings.TrimSpace(cfg.Auth.DemoUsername), "admin")
	cfg.Auth.DemoEmail = firstNonEmpty(strings.TrimSpace(cfg.Auth.DemoEmail), "admin@example.com")
	cfg.Auth.DemoPassword = firstNonEmpty(strings.TrimSpace(cfg.Auth.DemoPassword), "admin.pwd")
	cfg.SearchDemo.Provider = firstNonEmpty(strings.ToLower(strings.TrimSpace(cfg.SearchDemo.Provider)), "memory")
	cfg.SearchDemo.IndexName = firstNonEmpty(strings.TrimSpace(cfg.SearchDemo.IndexName), "media_transcripts")
	cfg.SearchDemo.DefaultLocale = firstNonEmpty(strings.TrimSpace(cfg.SearchDemo.DefaultLocale), cfg.Admin.DefaultLocale)
	cfg.SearchDemo.CultureDataPath = strings.TrimSpace(cfg.SearchDemo.CultureDataPath)
	cfg.SearchDemo.PostgresDSN = strings.TrimSpace(cfg.SearchDemo.PostgresDSN)
	cfg.SearchDemo.TypesenseServerURL = strings.TrimSpace(cfg.SearchDemo.TypesenseServerURL)
	cfg.SearchDemo.TypesenseAPIKey = strings.TrimSpace(cfg.SearchDemo.TypesenseAPIKey)
	cfg.SearchDemo.TypesenseCollectionPrefix = firstNonEmpty(strings.TrimSpace(cfg.SearchDemo.TypesenseCollectionPrefix), "search_shell_")
	if cfg.SearchDemo.ReindexBatchSize <= 0 {
		cfg.SearchDemo.ReindexBatchSize = 25
	}
}

func envString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
