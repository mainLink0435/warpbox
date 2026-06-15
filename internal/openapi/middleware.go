package openapi

import "net/http"

// annotatedHandler wraps an http.Handler with OpenAPI operation metadata.
// chi.Walk() discovers these wrappers on route tree nodes because they
// are registered directly as the endpoint handler (not via middleware).
type annotatedHandler struct {
	handler http.Handler
	op      Operation
}

func (h *annotatedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}

// Annotated wraps a handler with OpenAPI operation metadata and returns
// an http.Handler suitable for direct registration with chi route methods.
//
// Usage:
//
//	r.Get("/healthz", openapi.Annotated(s.handleHealthz, openapi.Operation{
//	    Summary: "Health check",
//	    Responses: map[string]openapi.Response{...},
//	}))
func Annotated(handler http.Handler, op Operation) http.Handler {
	return &annotatedHandler{handler: handler, op: op}
}

// unwrapAnnotated extracts the Operation from an annotated handler.
func unwrapAnnotated(h http.Handler) (Operation, http.Handler, bool) {
	ah, ok := h.(*annotatedHandler)
	if !ok {
		return Operation{}, h, false
	}
	return ah.op, ah.handler, true
}
