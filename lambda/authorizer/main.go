package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/golang-jwt/jwt/v5"
)

// TenantClaims extends the standard JWT claims with tenant information
type TenantClaims struct {
	jwt.RegisteredClaims
	TenantID string `json:"tenant_id"`
}

// HandleRequest is the Lambda handler for API Gateway authorizer
func HandleRequest(ctx context.Context, event events.APIGatewayCustomAuthorizerRequest) (events.APIGatewayCustomAuthorizerResponse, error) {
	log.Printf("Processing authorization event: %s", event.MethodArn)
	
	// Extract the token from the Authorization header
	authHeader := event.AuthorizationToken
	if authHeader == "" {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("unauthorized: missing Authorization header")
	}
	
	// Remove "Bearer " prefix if present
	token := strings.TrimPrefix(authHeader, "Bearer ")
	
	// Decode the JWT token without validation (API Gateway's Cognito Authorizer already validated it)
	parsedToken, err := extractTokenClaims(token)
	if err != nil {
		log.Printf("Error parsing token: %v", err)
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("unauthorized: invalid token format")
	}
	
	// Verify that tenant_id claim exists
	if parsedToken.TenantID == "" {
		log.Printf("Missing tenant_id claim in token")
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("unauthorized: missing tenant_id claim")
	}
	
	log.Printf("Authorized user with tenant_id: %s", parsedToken.TenantID)
	
	// Generate an Allow policy for this user
	return generateIAMPolicy(parsedToken.TenantID, "Allow", event.MethodArn, parsedToken), nil
}

// extractTokenClaims parses the JWT token and extracts the custom claims
func extractTokenClaims(tokenString string) (*TenantClaims, error) {
	// Parse the token without validating the signature
	// Since API Gateway's Cognito Authorizer already validated it
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &TenantClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}
	
	// Extract tenant claims
	claims, ok := token.Claims.(*TenantClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}
	
	return claims, nil
}

// generateIAMPolicy generates an IAM policy document for API Gateway
func generateIAMPolicy(principalID, effect, resource string, claims *TenantClaims) events.APIGatewayCustomAuthorizerResponse {
	// Generate the IAM policy
	authResponse := events.APIGatewayCustomAuthorizerResponse{
		PrincipalID: principalID,
		PolicyDocument: events.APIGatewayCustomAuthorizerPolicy{
			Version: "2012-10-17",
			Statement: []events.IAMPolicyStatement{
				{
					Action:   []string{"execute-api:Invoke"},
					Effect:   effect,
					Resource: []string{resource},
				},
			},
		},
	}
	
	// Add context for downstream resources
	authResponse.Context = map[string]interface{}{
		"tenant_id": claims.TenantID,
		"sub":       claims.Subject,
	}
	
	return authResponse
}

func main() {
	lambda.Start(HandleRequest)
}