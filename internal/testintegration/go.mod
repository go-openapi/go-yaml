module github.com/go-openapi/go-yaml/internal/testintegration

go 1.26.0

replace github.com/go-openapi/go-yaml => ../..

require (
	github.com/go-openapi/go-yaml v0.0.0-00010101000000-000000000000
	github.com/go-openapi/jsonpointer v1.0.2
	github.com/go-openapi/testify/v2 v2.8.1
	github.com/go-playground/validator/v10 v10.30.3
	github.com/google/go-cmp v0.7.0
	go.yaml.in/yaml/v3 v3.0.6
	pgregory.net/rapid v1.3.0
)

require (
	github.com/gabriel-vasile/mimetype v1.4.15 // indirect
	github.com/go-openapi/conformance-suites v0.0.0
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/leodido/go-urn v1.5.0 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

replace github.com/go-openapi/conformance-suites => ../../../conformance-suites
