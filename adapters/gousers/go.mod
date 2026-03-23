module github.com/goliatone/go-search/adapters/gousers

go 1.26.1

require (
	github.com/goliatone/go-search v0.0.0
	github.com/goliatone/go-users v0.17.0
	github.com/google/uuid v1.6.0
)

require (
	github.com/goliatone/go-i18n v0.4.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/goliatone/go-search => ../..

replace github.com/goliatone/go-i18n => ../../../go-i18n

replace github.com/goliatone/go-users => ../../../go-users
