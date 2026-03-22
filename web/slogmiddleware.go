package web

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder is a custom ResponseWriter that tracks the status code and bytes written.
type statusRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	size, err := r.ResponseWriter.Write(b)
	r.size += size
	return size, err
}

// slogMiddleware logs the start and end of each HTTP request using slog.
func (web *WebApp) slogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer with recorder, defaulting to 200 OK.
		rec := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		// Process the request.
		next.ServeHTTP(rec, r)

		// Calculate duration.
		duration := time.Since(start)

		// Skip logging static assets.
		//if strings.Prefix(r.URL.Path, "/static") {
		//	return
		//}

		// Log using key/value pairs
		web.log.Info("HTTP Request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Duration("duration", duration),
			slog.String("ip", r.RemoteAddr),
		)
	})
}
