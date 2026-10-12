package httpserver

import "net/http"

// fuego writes a route's default status before its serializer sets Content-Type, so the status is
// held back until the body is written.
func holdStatus(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := &statusHoldingWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		writer.release()
	})
}

type statusHoldingWriter struct {
	http.ResponseWriter
	held int
	sent bool
}

// Informational statuses, such as a WebSocket's 101, go out at once.
func (w *statusHoldingWriter) WriteHeader(status int) {
	if w.sent || status < http.StatusOK {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.held == 0 {
		w.held = status
	}
}

func (w *statusHoldingWriter) Write(b []byte) (int, error) {
	w.release()
	return w.ResponseWriter.Write(b)
}

func (w *statusHoldingWriter) Flush() {
	w.release()
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *statusHoldingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *statusHoldingWriter) release() {
	if w.sent {
		return
	}
	w.sent = true
	if w.held != 0 {
		w.ResponseWriter.WriteHeader(w.held)
	}
}
