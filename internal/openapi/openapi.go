// Package openapi provides a lightweight, chi-driven OpenAPI 3.0 spec generator.
//
// Use openapi.Annotated() middleware to attach metadata to chi routes at
// registration time. At server start, Builder.BuildFromRouter() calls
// chi.Walk() to enumerate all routes, extract metadata from annotated ones,
// and produce a complete OpenAPI 3.0 spec that is served at /openapi.json.
//
// Non-annotated routes (WebDAV, static assets, etc.) are silently excluded.
package openapi

import "encoding/json"

// Info provides metadata about the API.
type Info struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

// Spec is the root OpenAPI 3.0 document.
type Spec struct {
	OpenAPI string              `json:"openapi"`
	Info    Info                `json:"info"`
	Paths   map[string]PathItem `json:"paths"`
}

// PathItem describes the available operations on a single path.
type PathItem map[string]Operation // key = lowercase HTTP method (get, post, …)

// Operation describes a single API endpoint.
type Operation struct {
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Parameters  []Parameter           `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]Response   `json:"responses"`
}

// Parameter describes a single operation parameter.
type Parameter struct {
	Name        string `json:"name"`
	In          string `json:"in"` // path, query, header, cookie
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
	Schema      Schema `json:"schema"`
}

// RequestBody describes the request body for an operation.
type RequestBody struct {
	Required bool                  `json:"required,omitempty"`
	Content  map[string]MediaType  `json:"content"`
}

// Response describes a single response from an API operation.
type Response struct {
	Description string                `json:"description"`
	Content     map[string]MediaType  `json:"content,omitempty"`
}

// MediaType provides the schema for a request/response content type.
type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

// Schema is a JSON Schema document used within OAS 3.0.
type Schema struct {
	Type                 string             `json:"type,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Enum                 []string           `json:"enum,omitempty"`
	Format               string             `json:"format,omitempty"`
	Example              any                `json:"example,omitempty"`
}

// JSONContent returns a content map for application/json with the given schema.
func JSONContent(schema Schema) map[string]MediaType {
	return map[string]MediaType{
		"application/json": {Schema: &schema},
	}
}

// FormContent returns a content map for application/x-www-form-urlencoded
// with the given schema.
func FormContent(schema Schema) map[string]MediaType {
	return map[string]MediaType{
		"application/x-www-form-urlencoded": {Schema: &schema},
	}
}

// MarshalJSON for Spec ensures proper serialization.
// Custom handling to serialize empty Paths as {}, not null.
func (s *Spec) MarshalJSON() ([]byte, error) {
	type spec Spec
	if s.Paths == nil {
		s.Paths = make(map[string]PathItem)
	}
	return json.Marshal((*spec)(s))
}
