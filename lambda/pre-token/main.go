package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// HandleRequest processes the Cognito Pre Token Generation V2_0 event
// Using official AWS SDK event type for Cognito Pre Token Generation V2_0
func HandleRequest(ctx context.Context, event events.CognitoEventUserPoolsPreTokenGenV2_0) (events.CognitoEventUserPoolsPreTokenGenV2_0, error) {
	log.Printf("Received event for user: %s", event.UserName)

	// Get tenant ID from environment variable - this Lambda is tenant-specific
	tenantID := os.Getenv("TENANT_ID")
	if tenantID == "" {
		log.Printf("TENANT_ID environment variable not set, skipping tenant claim")
		return event, nil
	}

	// Add tenant_id claim to ID tokens
	if event.Response.ClaimsAndScopeOverrideDetails.IDTokenGeneration.ClaimsToAddOrOverride == nil {
		event.Response.ClaimsAndScopeOverrideDetails.IDTokenGeneration.ClaimsToAddOrOverride = make(map[string]interface{})
	}
	event.Response.ClaimsAndScopeOverrideDetails.IDTokenGeneration.ClaimsToAddOrOverride["tenant_id"] = tenantID

	// Add tenant_id to access tokens (KEY for API Gateway authorization!)
	if event.Response.ClaimsAndScopeOverrideDetails.AccessTokenGeneration.ClaimsToAddOrOverride == nil {
		event.Response.ClaimsAndScopeOverrideDetails.AccessTokenGeneration.ClaimsToAddOrOverride = make(map[string]interface{})
	}
	event.Response.ClaimsAndScopeOverrideDetails.AccessTokenGeneration.ClaimsToAddOrOverride["tenant_id"] = tenantID

	log.Printf("Added tenant_id claim %s to both ID and access tokens for user %s", tenantID, event.UserName)
	return event, nil
}

func main() {
	lambda.Start(HandleRequest)
}
