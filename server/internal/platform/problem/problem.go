// Package problem turns errors into RFC 9457 problem JSON with a stable type.
package problem

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-fuego/fuego"
)

const (
	ContentType       = "application/problem+json"
	contentTypeHeader = "Content-Type"
)

// Type identifies a kind of problem. Clients branch on it, so a value never changes once shipped.
type Type string

const (
	TypeBadRequest   Type = "urn:magpie:problem:bad-request"
	TypeUnauthorized Type = "urn:magpie:problem:unauthorized"
	TypeForbidden    Type = "urn:magpie:problem:forbidden"
	TypeNotFound     Type = "urn:magpie:problem:not-found"
	TypeConflict     Type = "urn:magpie:problem:conflict"
	TypeInternal     Type = "urn:magpie:problem:internal"
)

var typesByStatus = map[int]Type{
	http.StatusBadRequest:   TypeBadRequest,
	http.StatusUnauthorized: TypeUnauthorized,
	http.StatusForbidden:    TypeForbidden,
	http.StatusNotFound:     TypeNotFound,
	http.StatusConflict:     TypeConflict,
}

// New builds a problem that fuego serialises as application/problem+json.
func New(problemType Type, status int, detail string) fuego.HTTPError {
	return fuego.HTTPError{
		Type:   string(problemType),
		Status: status,
		Title:  http.StatusText(status),
		Detail: detail,
	}
}

// Handle is the fuego error handler. Known errors keep their status and get a type;
// anything else becomes a generic internal problem so internals never leak to clients.
func Handle(ctx context.Context, err error) error {
	var withStatus fuego.ErrorWithStatus
	if !errors.As(err, &withStatus) {
		slog.ErrorContext(ctx, "unhandled error", "error", err)
		internal := New(TypeInternal, http.StatusInternalServerError, "")
		internal.Err = err
		return internal
	}

	httpErr := fuego.HTTPError{Err: err}
	errors.As(err, &httpErr)
	httpErr.Status = withStatus.StatusCode()
	if httpErr.Title == "" {
		httpErr.Title = http.StatusText(httpErr.Status)
	}
	if httpErr.Type == "" {
		httpErr.Type = string(typeForStatus(httpErr.Status))
	}

	level := slog.LevelDebug
	if httpErr.Status >= http.StatusInternalServerError {
		level = slog.LevelError
	}
	slog.Log(ctx, level, "request failed", "type", httpErr.Type, "status", httpErr.Status, "detail", httpErr.Detail, "error", err)
	return httpErr
}

// Send is the fuego error serialiser. It always answers with problem JSON, whatever the Accept header says.
func Send(w http.ResponseWriter, r *http.Request, err error) {
	var httpErr fuego.HTTPError
	if !errors.As(err, &httpErr) {
		httpErr = New(TypeInternal, http.StatusInternalServerError, "")
	}
	httpErr.Status = httpErr.StatusCode()

	w.Header().Set(contentTypeHeader, ContentType)
	w.WriteHeader(httpErr.Status)
	if encodeErr := json.NewEncoder(w).Encode(httpErr); encodeErr != nil {
		slog.ErrorContext(r.Context(), "writing problem response", "error", encodeErr)
	}
}

// Write sends err as problem JSON from a plain net/http handler.
func Write(w http.ResponseWriter, r *http.Request, err error) {
	Send(w, r, Handle(r.Context(), err))
}

func typeForStatus(status int) Type {
	if problemType, ok := typesByStatus[status]; ok {
		return problemType
	}
	return TypeInternal
}
