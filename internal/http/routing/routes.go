package routing

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

var methods = [...]string{
	http.MethodConnect, http.MethodDelete, http.MethodGet,
	http.MethodHead, http.MethodOptions, http.MethodPatch,
	http.MethodPost, http.MethodPut, http.MethodTrace,
}

func AllowedMethods(router chi.Router, path string) []string {
	allowed := make([]string, 0, len(methods))
	for _, method := range methods {
		if router.Match(chi.NewRouteContext(), method, path) {
			allowed = append(allowed, method)
		}
	}
	return allowed
}

func Pattern(router chi.Router, r *http.Request) string {
	ctx := chi.NewRouteContext()
	if router.Match(ctx, r.Method, r.URL.Path) {
		return ctx.RoutePattern()
	}
	for _, method := range methods {
		ctx = chi.NewRouteContext()
		if router.Match(ctx, method, r.URL.Path) {
			return ctx.RoutePattern()
		}
	}
	return "unmatched"
}
