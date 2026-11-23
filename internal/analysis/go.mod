// Module analysis holds the measurements behind ANALYSIS-go-openapi.md.
//
// It is a separate module on purpose: it depends on go.yaml.in/yaml/v3 for comparison, and
// the library itself takes no runtime dependencies -- a property worth keeping.
module github.com/go-openapi/go-yaml/internal/analysis

go 1.26.0

require (
	github.com/go-openapi/go-yaml v0.0.0-00010101000000-000000000000
	github.com/go-openapi/testify/v2 v2.8.1
	github.com/goccy/go-yaml v1.19.2
	go.yaml.in/yaml/v3 v3.0.6
)

replace github.com/go-openapi/go-yaml => ../..
