package web

import (
	"log"
	"net/http"
)

// preventCSRF is Alex Edwards's examplar implementation of Go's 1.25 CSRF middleware.
func preventCSRF(next http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("CSRF check failed"))
	}))
	return cop.Handler(next)
}

// enforceCSRF wraps preventCSRF and ensure that any browser or agent that does not
// support the CSRF protection headers is rejected.
func enforceCSRF(next http.Handler) http.Handler {

	standardCSRF := preventCSRF(next)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Ignore non-data changing methods.
		if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" || r.Method == "TRACE" {
			next.ServeHTTP(w, r)
			return
		}

		// Reject if browser/agent does not support Sec-Fetch-Site or Origin.
		if r.Header.Get("Sec-Fetch-Site") == "" && r.Header.Get("Origin") == "" {
			log.Printf("Rejected request from %s: missing Sec-Fetch-Site and/or Origin headers", r.RemoteAddr)
			http.Error(w, "Agent or browser not supported.", http.StatusForbidden)
			return
		}

		// Continue to execute the standard CSRF handler.
		standardCSRF.ServeHTTP(w, r)
	})
}
