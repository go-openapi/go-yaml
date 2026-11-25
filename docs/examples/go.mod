// Module github.com/go-openapi/go-yaml/docs/examples holds the recipes on the
// documentation site. It is a module of its own so that nothing it imports
// reaches a go-yaml consumer.
module github.com/go-openapi/go-yaml/docs/examples

go 1.26.0

replace github.com/go-openapi/go-yaml => ../..

require github.com/go-openapi/go-yaml v0.0.0-00010101000000-000000000000

require (
	github.com/go-openapi/core/json v0.0.3 // indirect
	github.com/go-openapi/swag/conv v0.28.0 // indirect
	github.com/go-openapi/swag/pools v0.28.0 // indirect
)
