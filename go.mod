module github.com/goliatone/go-search

go 1.26.1

require github.com/goliatone/go-errors v0.10.0

require (
	github.com/go-ozzo/ozzo-validation/v4 v4.3.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
)

replace github.com/goliatone/go-command => ../go-command

replace github.com/goliatone/go-errors => ../go-errors

replace github.com/goliatone/go-repository-bun => ../go-repository-bun

replace github.com/goliatone/go-persistence-bun => ../go-persistence-bun
