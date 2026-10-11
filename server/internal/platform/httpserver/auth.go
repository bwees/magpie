package httpserver

import (
	"context"
	"net/http"

	"magpie/internal/domain"
	"magpie/internal/platform/problem"
)

// RequireAdmin returns a problem unless the request's principal is an admin.
func RequireAdmin(ctx context.Context) error {
	principal, ok := domain.PrincipalFrom(ctx)
	if !ok {
		return problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "sign in first")
	}
	if !principal.Admin {
		return problem.New(problem.TypeForbidden, http.StatusForbidden, "admins only")
	}
	return nil
}
