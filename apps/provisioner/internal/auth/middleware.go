package auth

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"supabase-manager/internal/contracts"
)

func RequireManagerToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if len(provided) != len(token) || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(response).Encode(contracts.ErrorEnvelope{Error: contracts.APIError{Code: "UNAUTHORIZED", Message: "Manager service authentication is required"}})
			return
		}
		next.ServeHTTP(response, request)
	})
}
