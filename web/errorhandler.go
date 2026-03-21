package web

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/rorycl/reconciler/domain"
	"github.com/rorycl/reconciler/internal/token"
)

// ErrorChecker wraps appHandler (handlers that return an error). This allows error
// reporting to be centralised, and the appHandlers to be simplified by avoiding error
// handling boilerplate.
func (web *WebApp) ErrorChecker(h appHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err != nil {
			// OAuth2 web flow error.
			if e, isErr := errors.AsType[token.ErrTokenWebClient](err); isErr {
				web.log.Info(err.Error(), "context", e.Context, "method", r.Method, "uri", r.URL.RequestURI())
				http.Error(w, e.Msg, http.StatusBadRequest) // not sure about best error type
				return
			}
			// Domain system error.
			if e, isErr := errors.AsType[domain.ErrSystem](err); isErr {
				web.log.Error(err.Error(), "detail", e.Detail, "method", r.Method, "uri", r.URL.RequestURI())
				http.Error(w, e.Msg, http.StatusInternalServerError)
				return
			}
			// Domain usage error.
			if e, isErr := errors.AsType[domain.ErrUsage](err); isErr {
				web.log.Info(err.Error(), "detail", e.Detail, "method", r.Method, "uri", r.URL.RequestURI())
				http.Error(w, e.Msg, http.StatusBadRequest)
				return
			}
			// Web internal error.
			if e, isErr := errors.AsType[errInternal](err); isErr {
				web.log.Error(err.Error(), "method", r.Method, "uri", r.URL.RequestURI())
				http.Error(w, e.msg, http.StatusInternalServerError)
				return
			}
			// Web usage error.
			if e, isErr := errors.AsType[errUsage](err); isErr {
				web.log.Info(err.Error(), "method", r.Method, "uri", r.URL.RequestURI())
				http.Error(w, e.msg, e.status)
				return
			}
			// Web htmx client error.
			if e, isErr := errors.AsType[errHTMX](err); isErr {
				web.log.Warn(err.Error(), "method", r.Method, "uri", r.URL.RequestURI())
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				errorString := fmt.Sprintf(
					`<div class="text-sm text-red px-4 pb-2">%s</div>`,
					e.msg,
				)
				_, _ = w.Write([]byte(errorString))
				return
			}
			// Fall through error.
			web.log.Error(err.Error(), "method", r.Method, "uri", r.URL.RequestURI())
			http.Error(w, "an unknown error occurred", http.StatusInternalServerError)
			return

		}
	})
}
