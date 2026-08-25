package httpapi

import (
	"context"
	"net/http"
	"net/url"

	"supabase-manager/apps/manager/internal/auth"
)

type identityContextKey struct{}

func (h authHandlers) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if origin == "" || !sameOrigin(origin, h.options.PublicOrigin) {
			writeError(response, http.StatusForbidden, "INVALID_ORIGIN", "Request origin is not allowed")
			return
		}
		next.ServeHTTP(response, request)
	})
}

func sameOrigin(left, right string) bool {
	leftURL, leftErr := url.Parse(left)
	rightURL, rightErr := url.Parse(right)
	return leftErr == nil && rightErr == nil && leftURL.Scheme == rightURL.Scheme && leftURL.Host == rightURL.Host
}

func RequireSession(service *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		token, ok := sessionToken(request)
		if !ok {
			writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required")
			return
		}
		identity, err := service.Authenticate(request.Context(), token)
		if err != nil {
			writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required")
			return
		}
		ctx := context.WithValue(request.Context(), identityContextKey{}, identity)
		next.ServeHTTP(response, request.WithContext(ctx))
	})
}

func ProtectAPI(options AuthOptions, next http.Handler) http.Handler {
	authenticated := RequireSession(options.Service, next)
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead && request.Method != http.MethodOptions {
			origin := request.Header.Get("Origin")
			if origin == "" || !sameOrigin(origin, options.PublicOrigin) {
				writeError(response, http.StatusForbidden, "INVALID_ORIGIN", "Request origin is not allowed")
				return
			}
			token, ok := sessionToken(request)
			if !ok || options.Service.ValidateCSRF(request.Context(), token, request.Header.Get("X-CSRF-Token")) != nil {
				writeError(response, http.StatusForbidden, "INVALID_CSRF", "CSRF validation failed")
				return
			}
		}
		authenticated.ServeHTTP(response, request)
	})
}

func IdentityFromContext(ctx context.Context) (auth.Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(auth.Identity)
	return identity, ok
}
