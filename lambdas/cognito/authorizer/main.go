package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/coreos/go-oidc/v3/oidc"
	"log"
	"strings"
)

// No global variables needed

// TokenInfo contains the validated token information
type TokenInfo struct {
	TenantID   string
	Username   string
	Expiration int64 // Unix timestamp
}

// extractIssuerFromToken extracts the issuer claim from a JWT token without verification.
// This is safe because we immediately verify the token with the extracted issuer's keys.
// We need this because the OIDC library requires knowing the issuer URL to fetch the public keys,
// but the issuer is inside the token itself.
func extractIssuerFromToken(tokenStr string) (string, error) {
	// JWT format: header.payload.signature
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid token format: expected 3 parts, got %d", len(parts))
	}
	
	// Decode the payload (base64url without padding)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("failed to decode token payload: %w", err)
	}
	
	// Parse just enough to get the issuer
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("failed to parse token claims: %w", err)
	}
	
	issuer, ok := claims["iss"].(string)
	if !ok || issuer == "" {
		return "", fmt.Errorf("missing or invalid issuer claim")
	}
	
	return issuer, nil
}

func ValidateToken(ctx context.Context, tokenStr string) (*TokenInfo, error) {
	// Extract issuer from the token to know which Cognito User Pool to verify against
	issuer, err := extractIssuerFromToken(tokenStr)
	if err != nil {
		return nil, fmt.Errorf("failed to extract issuer: %w", err)
	}
	
	log.Printf("🔍 Token issuer: %s", issuer)
	
	// Connect to the issuer's OIDC endpoint to get the public keys
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider for issuer %s: %w", issuer, err)
	}

	// For access tokens, skip audience check as they don't have 'aud' claim
	verifier := provider.Verifier(&oidc.Config{
		SkipClientIDCheck: true, // Access tokens don't have audience claim
	})

	// Verify the token signature, expiry, and issuer
	idToken, err := verifier.Verify(ctx, tokenStr)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}

	// Extract claims from the verified token
	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to decode claims: %w", err)
	}

	// Extract tenant_id - this is our custom claim added by the pre-token Lambda
	tenant, _ := claims["tenant_id"].(string)
	if tenant == "" {
		return nil, fmt.Errorf("missing tenant_id claim")
	}

	// Extract username (Cognito uses the "username" claim in access tokens)
	username, _ := claims["username"].(string)
	
	// Extract the expiration (standard claim "exp")
	exp, _ := claims["exp"].(float64)
	expiration := int64(exp)

	log.Printf("✅ Token validated: tenant=%s, user=%s, exp=%d", 
		tenant, username, expiration)
	
	return &TokenInfo{
		TenantID:   tenant,
		Username:   username,
		Expiration: expiration,
	}, nil
}

// extractAuthorizationHeader retrieves the authorization header from the request
func extractAuthorizationHeader(headers map[string]string) (string, bool) {
	// Try standard capitalization first
	if authHeader, exists := headers["Authorization"]; exists {
		return authHeader, true
	}
	// Try lowercase as fallback
	if authHeader, exists := headers["authorization"]; exists {
		return authHeader, true
	}
	return "", false
}

// stripBearerPrefix removes the "Bearer " prefix from a token if present
func stripBearerPrefix(token string) string {
	if len(token) > 7 {
		prefix := strings.ToLower(token[:7])
		if prefix == "bearer " {
			log.Printf("🔍 Stripped 'Bearer ' prefix (case insensitive)")
			return token[7:] // Remove "Bearer " prefix (7 characters)
		}
	}
	return token
}

// createAuthorizerResponse creates a standardized authorizer response
func createAuthorizerResponse(principalID string, allow bool, methodArn string, context map[string]interface{}) events.APIGatewayCustomAuthorizerResponse {
	effect := "Allow"
	if !allow {
		effect = "Deny"
	}
	
	response := events.APIGatewayCustomAuthorizerResponse{
		PrincipalID:    principalID,
		PolicyDocument: generatePolicy(effect, methodArn),
	}
	
	if context != nil {
		response.Context = context
	}
	
	return response
}

func handler(ctx context.Context, event events.APIGatewayCustomAuthorizerRequestTypeRequest) (events.APIGatewayCustomAuthorizerResponse, error) {
	log.Printf("🚀 REQUEST AUTHORIZER INVOKED: Starting authorization for %s", event.MethodArn)
	log.Printf("📋 REQUEST INFO: %s %s", event.HTTPMethod, event.Path)
	log.Printf("🌐 Stage: %s, RequestID: %s", event.RequestContext.Stage, event.RequestContext.RequestID)

	// Log all available headers for debugging
	log.Printf("📋 All Headers: %+v", event.Headers)

	// Extract Authorization header from REQUEST event
	authHeader, exists := extractAuthorizationHeader(event.Headers)
	log.Printf("🎟️  Authorization Header Present: %v (looking for: Authorization or authorization)", exists)
	if !exists {
		log.Printf("❌ AUTHORIZATION FAILED: No Authorization header found")
		return createAuthorizerResponse("unauthorized", false, event.MethodArn, nil), nil
	}

	token := authHeader
	log.Printf("🔍 Raw token received (length: %d): %s", len(token), token)

	// Handle the case-insensitive stripping of the "Bearer " prefix
	token = stripBearerPrefix(token)

	log.Printf("🔍 Token after stripping (length: %d)", len(token))
	if len(token) > 80 {
		log.Printf("🔍 First 80 chars: %s", token[:80])
	} else {
		log.Printf("🔍 Full token: %s", token)
	}

	tokenInfo, err := ValidateToken(ctx, token)
	if err != nil {
		log.Printf("❌ AUTHORIZATION FAILED: %v", err)
		return createAuthorizerResponse("unauthorized", false, event.MethodArn, nil), nil
	}

	log.Printf("✅ AUTHORIZATION SUCCESSFUL: tenant=%s, user=%s, exp=%d", 
		tokenInfo.TenantID, tokenInfo.Username, tokenInfo.Expiration)
	
	// Pass token information to the Lambda via context
	authContext := map[string]interface{}{
		"tenant_id":        tokenInfo.TenantID,
		"username":         tokenInfo.Username,
		"token_expiration": fmt.Sprintf("%d", tokenInfo.Expiration), // Must be string in context
	}
	
	return createAuthorizerResponse(tokenInfo.TenantID, true, event.MethodArn, authContext), nil
}

func generatePolicy(effect, resource string) events.APIGatewayCustomAuthorizerPolicy {
	return events.APIGatewayCustomAuthorizerPolicy{
		Version: "2012-10-17",
		Statement: []events.IAMPolicyStatement{{
			Action:   []string{"execute-api:Invoke"},
			Effect:   effect,
			Resource: []string{resource},
		}},
	}
}

func main() {
	lambda.Start(handler)
}