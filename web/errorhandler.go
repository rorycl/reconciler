package web

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/rorycl/reconciler/domain"
)

// ErrorChecker wraps appHandler (handlers that return an error). This allows error
// reporting to be centralised, and the appHandlers to be simplified by avoiding error
// handling boilerplate.
func (web *WebApp) ErrorChecker(h appHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err != nil {
			if e, isErr := errors.AsType[domain.ErrSystem](err); isErr {
				web.log.Error(err.Error(), "detail", e.Detail, "method", r.Method, "uri", r.URL.RequestURI())
				http.Error(w, e.Msg, http.StatusInternalServerError)
				return
			}
			if e, isErr := errors.AsType[errInternal](err); isErr {
				web.log.Error(err.Error(), "method", r.Method, "uri", r.URL.RequestURI())
				http.Error(w, e.msg, http.StatusInternalServerError)
				return
			}
			if e, isErr := errors.AsType[domain.ErrUsage](err); isErr {
				web.log.Info(err.Error(), "detail", e.Detail, "method", r.Method, "uri", r.URL.RequestURI())
				http.Error(w, e.Msg, 404)
				return
			}
			if e, isErr := errors.AsType[errUsage](err); isErr {
				web.log.Info(err.Error(), "method", r.Method, "uri", r.URL.RequestURI())
				http.Error(w, e.msg, e.status)
				return
			}
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
			web.log.Error(err.Error(), "method", r.Method, "uri", r.URL.RequestURI())
			http.Error(w, e.msg, http.StatusInternalServerError)
			return

		}
		return
	})
}
