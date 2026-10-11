package identity

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"magpie/internal/domain"
	"magpie/internal/platform/httpserver"
	"magpie/internal/platform/problem"
)

const (
	SessionCookie = "magpie_session"

	cookiePath           = "/"
	forwardedProtoHeader = "X-Forwarded-Proto"
	httpsScheme          = "https"

	apiPathPrefix  = httpserver.APIPrefix + "/"
	identityPrefix = httpserver.APIPrefix + "/identity"
	setupAdminPath = identityPrefix + setupAdminRoute
	signInPrefix   = identityPrefix + signInRoutePrefix
)

// Middleware gates the API on setup, resolves the session cookie into the request's principal and
// answers 401 for routes that need a session. Paths outside /api pass through untouched.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !strings.HasPrefix(path, apiPathPrefix) || path == httpserver.HealthPath {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()
		required, err := s.SetupRequired(ctx)
		if err != nil {
			problem.Write(w, r, err)
			return
		}
		if required && path != setupAdminPath {
			problem.Write(w, r, problem.New(TypeSetupRequired, http.StatusConflict, "create the local admin first"))
			return
		}

		if cookie, err := r.Cookie(SessionCookie); err == nil {
			active, err := s.Authenticate(ctx, cookie.Value)
			switch {
			case errors.Is(err, ErrNoSession):
				clearSessionCookie(w, r)
			case err != nil:
				problem.Write(w, r, err)
				return
			default:
				if active.Renewed {
					setSessionCookie(w, r, cookie.Value, active.ExpiresAt)
				}
				ctx = domain.WithPrincipal(ctx, active.Principal)
			}
		}

		if _, signedIn := domain.PrincipalFrom(ctx); !signedIn && !isPublic(path) {
			problem.Write(w, r, problem.New(problem.TypeUnauthorized, http.StatusUnauthorized, "sign in first"))
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isPublic lists the routes anyone may call: setup and the sign-in flows.
func isPublic(path string) bool {
	return path == setupAdminPath || strings.HasPrefix(path, signInPrefix)
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     cookiePath,
		Expires:  expires,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Path:     cookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get(forwardedProtoHeader), httpsScheme)
}
