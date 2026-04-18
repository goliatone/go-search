package migrations

import (
	"io/fs"
	"strings"

	persistence "github.com/goliatone/go-persistence-bun"
	"github.com/goliatone/go-search/internal/migrationutil"
	searchpostgres "github.com/goliatone/go-search/providers/postgres"
	dispatchbunstore "github.com/goliatone/go-search/stores/dispatch/bun"
	editorialbunstore "github.com/goliatone/go-search/stores/editorial/bun"
	generationbunstore "github.com/goliatone/go-search/stores/generation/bun"
)

type Profile string

const (
	ProfilePostgresProvider Profile = "postgres-provider"
	ProfileExternalProvider Profile = "external-provider"
)

const (
	SourceNameProviderPostgres = "go-search-provider-postgres"
	SourceNameGeneration       = "go-search-generation"
	SourceNameEditorial        = "go-search-editorial"
	SourceNameDispatch         = "go-search-dispatch"
)

type Option func(*options)

type options struct {
	profile           Profile
	providerEnabled   *bool
	generationEnabled *bool
	editorialEnabled  *bool
	dispatchEnabled   *bool
	validationTargets []string
}

func defaultOptions() options {
	return options{
		profile:           ProfilePostgresProvider,
		validationTargets: []string{"postgres"},
	}
}

func WithProfile(profile Profile) Option {
	return func(opts *options) {
		if opts != nil {
			opts.profile = profile
		}
	}
}

func WithProviderEnabled(enabled bool) Option {
	return func(opts *options) {
		if opts != nil {
			value := enabled
			opts.providerEnabled = &value
		}
	}
}

func WithGenerationEnabled(enabled bool) Option {
	return func(opts *options) {
		if opts != nil {
			value := enabled
			opts.generationEnabled = &value
		}
	}
}

func WithEditorialEnabled(enabled bool) Option {
	return func(opts *options) {
		if opts != nil {
			value := enabled
			opts.editorialEnabled = &value
		}
	}
}

func WithDispatchEnabled(enabled bool) Option {
	return func(opts *options) {
		if opts != nil {
			value := enabled
			opts.dispatchEnabled = &value
		}
	}
}

func WithValidationTargets(targets ...string) Option {
	return func(opts *options) {
		if opts != nil {
			opts.validationTargets = append([]string(nil), targets...)
		}
	}
}

func Register(client *persistence.Client, opts ...Option) error {
	if client == nil {
		return nil
	}
	return RegisterManager(client.GetMigrations(), opts...)
}

func RegisterManager(manager *persistence.Migrations, opts ...Option) error {
	if manager == nil {
		return nil
	}
	sources, err := OrderedSources(opts...)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return nil
	}
	return manager.RegisterOrderedMigrationSources(sources...)
}

func OrderedSources(opts ...Option) ([]persistence.OrderedMigrationSource, error) {
	options := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	providerEnabled, generationEnabled, editorialEnabled, dispatchEnabled, err := resolveProfile(options.profile)
	if err != nil {
		return nil, err
	}
	if options.providerEnabled != nil {
		providerEnabled = *options.providerEnabled
	}
	if options.generationEnabled != nil {
		generationEnabled = *options.generationEnabled
	}
	if options.editorialEnabled != nil {
		editorialEnabled = *options.editorialEnabled
	}
	if options.dispatchEnabled != nil {
		dispatchEnabled = *options.dispatchEnabled
	}

	sources := make([]persistence.OrderedMigrationSource, 0, 4)
	if providerEnabled {
		source, err := buildSource(SourceNameProviderPostgres, searchpostgres.GetMigrationsFS, options.validationTargets)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	if generationEnabled {
		source, err := buildSource(SourceNameGeneration, generationbunstore.GetMigrationsFS, options.validationTargets)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	if editorialEnabled {
		source, err := buildSource(SourceNameEditorial, editorialbunstore.GetMigrationsFS, options.validationTargets)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	if dispatchEnabled {
		source, err := buildSource(SourceNameDispatch, dispatchbunstore.GetMigrationsFS, options.validationTargets)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func resolveProfile(profile Profile) (bool, bool, bool, bool, error) {
	switch Profile(strings.TrimSpace(strings.ToLower(string(profile)))) {
	case "", ProfilePostgresProvider:
		return true, true, true, false, nil
	case ProfileExternalProvider:
		return false, true, true, false, nil
	default:
		return false, false, false, false, &unsupportedProfileError{profile: profile}
	}
}

func buildSource(name string, resolveFS func() (fs.FS, error), targets []string) (persistence.OrderedMigrationSource, error) {
	root, err := resolveFS()
	if err != nil {
		return persistence.OrderedMigrationSource{}, err
	}
	options := []persistence.DialectMigrationOption{
		persistence.WithDialectSourceLabel(name),
		persistence.WithDialectResolver(migrationutil.RequirePostgresDialect),
		persistence.WithValidationTargets(),
		persistence.WithValidateOnMigrate(true),
	}
	if len(targets) > 0 {
		options = append(options, persistence.WithDialectValidationContract(persistence.DialectValidationContract{
			MandatoryTargets: append([]string(nil), targets...),
		}))
	}
	return persistence.OrderedMigrationSource{
		Name:    name,
		Root:    root,
		Options: options,
	}, nil
}

type unsupportedProfileError struct {
	profile Profile
}

func (e *unsupportedProfileError) Error() string {
	return "go-search/migrations: unsupported profile " + `"` + string(e.profile) + `"`
}
