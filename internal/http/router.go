package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/saifullinartem/saifullin_golang/api"
	"github.com/saifullinartem/saifullin_golang/internal/http/domainerror"
	"github.com/saifullinartem/saifullin_golang/internal/http/routing"
)

func NewRouter(handler api.ServerInterface) chi.Router {
	router := chi.NewRouter()
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		domainerror.Write(w, r, domainerror.ErrRouteNotFound)
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", strings.Join(routing.AllowedMethods(router, r.URL.Path), ", "))
		domainerror.Write(w, r, domainerror.ErrMethodNotAllowed)
	})
	api.HandlerWithOptions(handler, api.ChiServerOptions{BaseRouter: router, ErrorHandlerFunc: ParameterError})
	return router
}
