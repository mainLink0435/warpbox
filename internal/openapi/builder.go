package openapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Builder constructs an OpenAPI 3.0 spec from annotated chi routes.
type Builder struct {
	info Info
	spec *Spec // cached after BuildFromRouter
}

// NewBuilder returns a new Builder with the given API info.
func NewBuilder(info Info) *Builder {
	return &Builder{info: info}
}

// BuildFromRouter walks chi's route tree and builds the OpenAPI spec from
// routes annotated with openapi.Annotated(). Unannotated routes are skipped.
//
// After calling this, use Handler() to serve the spec.
func (b *Builder) BuildFromRouter(r chi.Routes) {
	paths := make(map[string]PathItem)

	chi.Walk(r, func(method, rawRoute string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		op, _, ok := unwrapAnnotated(handler)
		if !ok {
			return nil // skip unannotated routes
		}

		routePath := convertPath(rawRoute)
		methodLower := strings.ToLower(method)

		op.Parameters = append(op.Parameters, extractPathParams(rawRoute)...)

		if _, exists := paths[routePath]; !exists {
			paths[routePath] = make(PathItem)
		}
		paths[routePath][methodLower] = op
		return nil
	})

	b.spec = &Spec{
		OpenAPI: "3.0.3",
		Info:    b.info,
		Paths:   paths,
	}
}

// Handler returns an http.HandlerFunc that serves the OpenAPI spec as JSON.
// Must be called after BuildFromRouter.
func (b *Builder) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if b.spec == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "spec not built"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(b.spec)
	}
}

// pathParamRegex matches chi path parameters: {name} and {name:regex}
var pathParamRegex = regexp.MustCompile(`\{(\w+)(?::[^}]+)?\}`)

// convertPath converts a chi route pattern to an OpenAPI 3.0 path pattern.
//   - {name:regex} → {name}
//   - {name} → {name}
//   - clean up /*/ artifacts
func convertPath(chiPattern string) string {
	// Remove regex constraints: {param:regex} → {param}
	result := pathParamRegex.ReplaceAllString(chiPattern, "{$1}")
	// Collapse /*/ to /
	result = strings.ReplaceAll(result, "/*/", "/")
	return result
}

// extractPathParams extracts OpenAPI path parameter definitions from a chi pattern.
func extractPathParams(pattern string) []Parameter {
	matches := pathParamRegex.FindAllStringSubmatch(pattern, -1)
	if len(matches) == 0 {
		return nil
	}
	params := make([]Parameter, 0, len(matches))
	for _, m := range matches {
		name := m[1]
		params = append(params, Parameter{
			Name:     name,
			In:       "path",
			Required: true,
			Schema:   Schema{Type: "string"},
		})
	}
	return params
}
