package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/coreos/go-oidc/v3/oidc"
	"os"
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

	// Enforce audience = your App (Client) ID
	verifier := provider.Verifier(&oidc.Config{
		ClientID: clientID,
		// SkipClientIDCheck: true // (do NOT set if you want aud check)
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

func handler(ctx context.Context, event events.APIGatewayCustomAuthorizerRequest) (events.APIGatewayCustomAuthorizerResponse, error) {
	log.Printf("🚀 AUTHORIZER INVOKED: Starting authorization for %s", event.MethodArn)
	log.Printf("🎟️  Authorization Token Present: %v", event.AuthorizationToken != "")
	
	// Remove "Bearer " prefix if present
	token := strings.TrimPrefix(event.AuthorizationToken, "Bearer ")
	log.Printf("🔍 Token extracted (length: %d)", len(token))
	
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
