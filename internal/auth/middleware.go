package auth

import (
	"log"
	"net/http"
	"strings"
)

// TenantMiddleware extracts tenant information from JWT and adds it to the request context
func TenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			// Skip tenant extraction if no valid Authorization header
			next.ServeHTTP(w, r)
			return
		}

		// Extract tenant ID from JWT
		tenantID, err := ExtractTenantFromToken(authHeader)
		if err != nil {
			log.Printf("Warning: Failed to extract tenant ID from token: %v", err)
			// Continue without tenant ID if extraction fails
			next.ServeHTTP(w, r)
			return
		}

		// Add tenant ID to request context
		ctx := WithTenantID(r.Context(), tenantID)
		
		// Continue with the enhanced context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}