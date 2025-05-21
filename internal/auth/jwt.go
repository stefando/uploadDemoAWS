package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// JWT-related errors
var (
	ErrInvalidToken    = errors.New("invalid token format")
	ErrMissingTenantID = errors.New("tenant_id claim missing from token")
)

// TenantClaims extends the standard JWT claims with tenant information
type TenantClaims struct {
	jwt.RegisteredClaims
	TenantID string `json:"tenant_id"`
}

// ExtractTenantFromToken parses a JWT token and extracts the tenant ID
// This function doesn't validate the token signature (API Gateway does that)
// but extracts the tenant information from the payload
func ExtractTenantFromToken(tokenString string) (string, error) {
	// Remove "Bearer " prefix if present
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")
	
	// Parse the token without validating signature
	// In a production environment, we'd fully validate the token
	// But here we rely on API Gateway's JWT Authorizer to do that
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &TenantClaims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse token: %w", err)
	}
	
	// Extract tenant claims
	claims, ok := token.Claims.(*TenantClaims)
	if !ok {
		return "", ErrInvalidToken
	}
	
	// Ensure tenant ID exists
	if claims.TenantID == "" {
		return "", ErrMissingTenantID
	}
	
	return claims.TenantID, nil
}

// GetBucketNameForTenant returns the appropriate S3 bucket name based on tenant ID
func GetBucketNameForTenant(tenantID, bucketPrefix string) string {
	return fmt.Sprintf("%s-store-%s", bucketPrefix, tenantID)
}