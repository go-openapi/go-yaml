module github.com/go-openapi/go-yaml/internal/benchmarks

go 1.26.0

require (
	github.com/go-openapi/go-yaml v0.0.0-00010101000000-000000000000
	github.com/go-openapi/go-yaml/internal/analysis v0.0.0-00010101000000-000000000000
	github.com/go-openapi/spec v1.0.1
	github.com/go-openapi/testify/v2 v2.8.1
	github.com/goccy/go-yaml v1.19.2
	go.yaml.in/yaml/v3 v3.0.6
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/go-openapi/jsonpointer v1.0.0 // indirect
	github.com/go-openapi/jsonreference v1.0.1 // indirect
	github.com/go-openapi/swag/conv v0.29.1 // indirect
	github.com/go-openapi/swag/jsonutils v0.29.1 // indirect
	github.com/go-openapi/swag/loading v0.29.1 // indirect
	github.com/go-openapi/swag/pools v0.29.1 // indirect
	github.com/go-openapi/swag/stringutils v0.29.1 // indirect
	github.com/go-openapi/swag/typeutils v0.29.1 // indirect
	github.com/go-openapi/swag/yamlutils v0.29.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

replace github.com/go-openapi/go-yaml => ../..

replace github.com/go-openapi/go-yaml/internal/analysis => ../analysis
