package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/coreos/go-oidc/v3/oidc"
	"log"
	"os"
	"strings"
)

var (
	poolID   = mustEnv("COGNITO_POOL_ID")
	region   = mustEnv("COGNITO_REGION")
	clientID = mustEnv("COGNITO_CLIENT_ID")
)

func mustEnv(name string) string {
	v := os.Getenv(name)
	if v == "" {
		panic(fmt.Sprintf("missing env variable: %s", name))
	}
	return v
}

func ValidateToken(tokenStr string) (string, error) {
	issuer := fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s", region, poolID)

	// Connect to Cognito’s OIDC JWKS endpoint
	provider, err := oidc.NewProvider(context.Background(), issuer)
	if err != nil {
		return "", fmt.Errorf("oidc provider error: %w", err)
	}

	// For access tokens, skip audience check as they don't have 'aud' claim
	verifier := provider.Verifier(&oidc.Config{
		SkipClientIDCheck: true, // Access tokens don't have audience claim
	})

	// This will check sig, expiry, issuer, and aud for you
	idToken, err := verifier.Verify(context.Background(), tokenStr)
	if err != nil {
		return "", fmt.Errorf("token verification failed: %w", err)
	}

	// You can extract any JWT claim as a map
	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		return "", fmt.Errorf("decoding claims: %w", err)
	}

	// Pull out your custom tenant claim
	tenant, _ := claims["tenant_id"].(string)
	if tenant == "" {
		return "", fmt.Errorf("missing tenant_id claim")
	}

	return tenant, nil
}

func handler(ctx context.Context, event events.APIGatewayCustomAuthorizerRequestTypeRequest) (events.APIGatewayCustomAuthorizerResponse, error) {
	log.Printf("🚀 REQUEST AUTHORIZER INVOKED: Starting authorization for %s", event.MethodArn)
	log.Printf("📋 REQUEST INFO: %s %s", event.HTTPMethod, event.Path)
	log.Printf("🌐 Stage: %s, RequestID: %s", event.RequestContext.Stage, event.RequestContext.RequestID)

	// Log all available headers for debugging
	log.Printf("📋 All Headers: %+v", event.Headers)

	// Extract Authorization header from REQUEST event
	authHeader, exists := event.Headers["Authorization"]
	if !exists {
		authHeader, exists = event.Headers["authorization"] // Try lowercase
	}

	log.Printf("🎟️  Authorization Header Present: %v (looking for: Authorization or authorization)", exists)
	if !exists {
		log.Printf("❌ AUTHORIZATION FAILED: No Authorization header found")
		return events.APIGatewayCustomAuthorizerResponse{
			PrincipalID:    "unauthorized",
			PolicyDocument: generatePolicy("Deny", event.MethodArn),
		}, nil
	}

	token := authHeader
	log.Printf("🔍 Raw token received (length: %d): %s", len(token), token)

	// Handle case-insensitive "Bearer " prefix stripping
	if len(token) > 7 {
		prefix := strings.ToLower(token[:7])
		if prefix == "bearer " {
			token = token[7:] // Remove "Bearer " prefix (7 characters)
			log.Printf("🔍 Stripped 'Bearer ' prefix (case insensitive)")
		}
	}

	log.Printf("🔍 Token after stripping (length: %d)", len(token))
	if len(token) > 80 {
		log.Printf("🔍 First 80 chars: %s", token[:80])
	} else {
		log.Printf("🔍 Full token: %s", token)
	}

	tenant, err := ValidateToken(token)
	if err != nil {
		log.Printf("❌ AUTHORIZATION FAILED: %v", err)
		return events.APIGatewayCustomAuthorizerResponse{
			PrincipalID:    "unauthorized",
			PolicyDocument: generatePolicy("Deny", event.MethodArn),
		}, nil
	}

	log.Printf("✅ AUTHORIZATION SUCCESSFUL: tenant=%s", tenant)
	return events.APIGatewayCustomAuthorizerResponse{
		PrincipalID:    tenant,
		PolicyDocument: generatePolicy("Allow", event.MethodArn),
		Context: map[string]interface{}{
			"tenant_id": tenant,
		},
	}, nil
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
