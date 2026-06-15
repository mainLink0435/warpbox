package openapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestAnnotatedWrapsHandler(t *testing.T) {
	var called bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	wrapped := Annotated(h, Operation{Summary: "test"})

	op, inner, ok := unwrapAnnotated(wrapped)
	if !ok {
		t.Fatal("expected annotated handler")
	}
	if op.Summary != "test" {
		t.Errorf("expected summary 'test', got %q", op.Summary)
	}

	inner.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if !called {
		t.Error("inner handler was not called")
	}
}

func TestUnannotatedHandler(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	_, _, ok := unwrapAnnotated(h)
	if ok {
		t.Error("expected unannotated handler to return false")
	}
}

func TestConvertPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/healthz", "/healthz"},
		{"/items/{id}", "/items/{id}"},
		{"/items/{id:uuid}", "/items/{id}"},
		{"/items/{id:[0-9]+}", "/items/{id}"},
		{"/users/{userId}/posts/{postId}", "/users/{userId}/posts/{postId}"},
		{"/http/*", "/http/*"},
		{"/prefix/*/suffix", "/prefix/suffix"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := convertPath(tt.input)
			if got != tt.expected {
				t.Errorf("convertPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExtractPathParams(t *testing.T) {
	tests := []struct {
		pattern  string
		expected int // number of params
	}{
		{"/healthz", 0},
		{"/items/{id}", 1},
		{"/items/{id}/sub/{subId}", 2},
		{"/items/{id:regex}", 1},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			params := extractPathParams(tt.pattern)
			if len(params) != tt.expected {
				t.Errorf("extractPathParams(%q) = %d params, want %d", tt.pattern, len(params), tt.expected)
			}
			for _, p := range params {
				if p.In != "path" {
					t.Errorf("param %q: expected 'path', got %q", p.Name, p.In)
				}
				if !p.Required {
					t.Errorf("param %q: expected required=true", p.Name)
				}
			}
		})
	}
}

func TestBuildFromRouter(t *testing.T) {
	r := chi.NewRouter()

	b := NewBuilder(Info{
		Title:   "Test API",
		Version: "1.0.0",
	})

	// Annotated route
	healthzHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	r.Method("GET", "/healthz", Annotated(healthzHandler, Operation{
		Summary: "Health check",
		Tags:    []string{"System"},
		Responses: map[string]Response{
			"200": {Description: "OK", Content: JSONContent(Schema{Type: "object"})},
		},
	}))

	// Another annotated route
	itemHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	r.Method("GET", "/items/{id}", Annotated(itemHandler, Operation{
		Summary: "Get item",
		Tags:    []string{"Items"},
		Responses: map[string]Response{
			"200": {Description: "Item found"},
			"404": {Description: "Not found"},
		},
	}))

	// Unannotated route — should not appear in spec
	r.Get("/secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	b.BuildFromRouter(r)

	if b.spec == nil {
		t.Fatal("spec not built")
	}
	if b.spec.OpenAPI != "3.0.3" {
		t.Errorf("expected OpenAPI version 3.0.3, got %q", b.spec.OpenAPI)
	}
	if b.spec.Info.Title != "Test API" {
		t.Errorf("expected title 'Test API', got %q", b.spec.Info.Title)
	}

	// Check annotated routes exist
	if _, ok := b.spec.Paths["/healthz"]; !ok {
		t.Error("expected /healthz in spec paths")
	}
	if _, ok := b.spec.Paths["/items/{id}"]; !ok {
		t.Error("expected /items/{id} in spec paths")
	}
	// Check unannotated route is excluded
	if _, ok := b.spec.Paths["/secret"]; ok {
		t.Error("unannotated route /secret should not appear in spec")
	}

	// Check method and metadata
	healthzPath := b.spec.Paths["/healthz"]
	op, ok := healthzPath["get"]
	if !ok {
		t.Fatal("expected GET method on /healthz")
	}
	if op.Summary != "Health check" {
		t.Errorf("expected summary 'Health check', got %q", op.Summary)
	}
	if len(op.Tags) != 1 || op.Tags[0] != "System" {
		t.Errorf("expected tags [System], got %v", op.Tags)
	}
	if _, ok := op.Responses["200"]; !ok {
		t.Error("expected 200 response")
	}

	// Check item route has path parameter
	itemPath := b.spec.Paths["/items/{id}"]
	itemOp := itemPath["get"]
	if len(itemOp.Parameters) != 1 {
		t.Fatalf("expected 1 path parameter, got %d", len(itemOp.Parameters))
	}
	if itemOp.Parameters[0].Name != "id" {
		t.Errorf("expected parameter name 'id', got %q", itemOp.Parameters[0].Name)
	}
}

func TestHandlerServesSpec(t *testing.T) {
	r := chi.NewRouter()
	b := NewBuilder(Info{Title: "Test", Version: "1.0.0"})

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	r.Method("GET", "/test", Annotated(testHandler, Operation{
		Summary: "test",
		Responses: map[string]Response{
			"200": {Description: "OK"},
		},
	}))

	b.BuildFromRouter(r)

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	b.Handler()(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var spec Spec
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		t.Fatalf("failed to decode spec: %v", err)
	}
	if spec.Info.Title != "Test" {
		t.Errorf("expected title 'Test', got %q", spec.Info.Title)
	}
}

func TestHandlerWithoutBuildReturnsError(t *testing.T) {
	b := NewBuilder(Info{Title: "Test", Version: "1.0.0"})

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	b.Handler()(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != 500 {
		t.Errorf("expected 500 when spec not built, got %d", resp.StatusCode)
	}
}

func TestJSONContent(t *testing.T) {
	c := JSONContent(Schema{Type: "string"})
	if _, ok := c["application/json"]; !ok {
		t.Error("expected application/json key")
	}
	if c["application/json"].Schema.Type != "string" {
		t.Errorf("expected schema type 'string', got %q", c["application/json"].Schema.Type)
	}
}

func TestFormContent(t *testing.T) {
	c := FormContent(Schema{Type: "object"})
	if _, ok := c["application/x-www-form-urlencoded"]; !ok {
		t.Error("expected application/x-www-form-urlencoded key")
	}
}

func TestSpecJSONOutput(t *testing.T) {
	r := chi.NewRouter()
	b := NewBuilder(Info{Title: "Test", Version: "1.0.0"})

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	r.Method("GET", "/test", Annotated(testHandler, Operation{
		Summary: "test",
		Responses: map[string]Response{
			"200": {Description: "OK"},
		},
	}))

	b.BuildFromRouter(r)

	raw, err := json.Marshal(b.spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("failed to unmarshal spec: %v", err)
	}

	// Check required top-level keys
	for _, key := range []string{"openapi", "info", "paths"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("missing top-level key %q", key)
		}
	}

	// Check paths is an object, not null
	paths, ok := parsed["paths"].(map[string]any)
	if !ok {
		t.Fatal("paths should be a JSON object")
	}
	if len(paths) == 0 {
		t.Error("paths should not be empty")
	}
}
