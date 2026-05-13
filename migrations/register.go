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

const (
	SourceKeyProviderPostgres = "go-search-provider-postgres"
	SourceKeyGeneration       = "go-search-generation"
	SourceKeyEditorial        = "go-search-editorial"
	SourceKeyDispatch         = "go-search-dispatch"
)

const (
	SourceOrderProviderPostgres = 700
	SourceOrderGeneration       = 710
	SourceOrderEditorial        = 720
	SourceOrderDispatch         = 730
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

type sourceDefinition struct {
	name      string
	sourceKey string
	order     int
	resolveFS func() (fs.FS, error)
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
	definitions, targets, err := selectedSourceDefinitions(opts...)
	if err != nil {
		return nil, err
	}
	sources := make([]persistence.OrderedMigrationSource, 0, len(definitions))
	for _, definition := range definitions {
		source, err := buildStableSource(definition, targets)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, nil
}

// LegacyOrderedSources returns the pre-source-stable positional search source
// descriptors for compatibility backfills. Hosts repairing a shared database
// must include their own historical runtime sources before these descriptors.
func LegacyOrderedSources(opts ...Option) ([]persistence.OrderedMigrationSource, error) {
	definitions, targets, err := selectedSourceDefinitions(opts...)
	if err != nil {
		return nil, err
	}
	sources := make([]persistence.OrderedMigrationSource, 0, len(definitions))
	for _, definition := range definitions {
		source, err := buildLegacySource(definition, targets)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func selectedSourceDefinitions(opts ...Option) ([]sourceDefinition, []string, error) {
	options := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	providerEnabled, generationEnabled, editorialEnabled, dispatchEnabled, err := resolveProfile(options.profile)
	if err != nil {
		return nil, nil, err
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

	definitions := make([]sourceDefinition, 0, 4)
	if providerEnabled {
		definitions = append(definitions, sourceDefinition{
			name:      SourceNameProviderPostgres,
			sourceKey: SourceKeyProviderPostgres,
			order:     SourceOrderProviderPostgres,
			resolveFS: searchpostgres.GetMigrationsFS,
		})
	}
	if generationEnabled {
		definitions = append(definitions, sourceDefinition{
			name:      SourceNameGeneration,
			sourceKey: SourceKeyGeneration,
			order:     SourceOrderGeneration,
			resolveFS: generationbunstore.GetMigrationsFS,
		})
	}
	if editorialEnabled {
		definitions = append(definitions, sourceDefinition{
			name:      SourceNameEditorial,
			sourceKey: SourceKeyEditorial,
			order:     SourceOrderEditorial,
			resolveFS: editorialbunstore.GetMigrationsFS,
		})
	}
	if dispatchEnabled {
		definitions = append(definitions, sourceDefinition{
			name:      SourceNameDispatch,
			sourceKey: SourceKeyDispatch,
			order:     SourceOrderDispatch,
			resolveFS: dispatchbunstore.GetMigrationsFS,
		})
	}
	return definitions, options.validationTargets, nil
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

func buildStableSource(definition sourceDefinition, targets []string) (persistence.OrderedMigrationSource, error) {
	root, err := definition.resolveFS()
	if err != nil {
		return persistence.OrderedMigrationSource{}, err
	}
	return persistence.NewStableOrderedMigrationSource(
		definition.name,
		root,
		definition.sourceKey,
		definition.order,
		persistence.WithOrderedMigrationDialectOptions(dialectOptions(definition.name, targets)...),
	), nil
}

func buildLegacySource(definition sourceDefinition, targets []string) (persistence.OrderedMigrationSource, error) {
	root, err := definition.resolveFS()
	if err != nil {
		return persistence.OrderedMigrationSource{}, err
	}
	return persistence.OrderedMigrationSource{
		Name:    definition.name,
		Root:    root,
		Options: dialectOptions(definition.name, targets),
	}, nil
}

func dialectOptions(name string, targets []string) []persistence.DialectMigrationOption {
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
	return options
}

type unsupportedProfileError struct {
	profile Profile
}

func (e *unsupportedProfileError) Error() string {
	return "go-search/migrations: unsupported profile " + `"` + string(e.profile) + `"`
}
