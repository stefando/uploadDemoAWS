package main

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// TenantMapping defines which users belong to which tenants
// In a real application, this would come from a database
var TenantMapping = map[string]string{
	"user-tenant-a": "tenant-a",
	"user-tenant-b": "tenant-b",
}

// HandleRequest processes the Cognito Pre Token Generation event
// Using official AWS SDK event type for Cognito Pre Token Generation
func HandleRequest(ctx context.Context, event events.CognitoEventUserPoolsPreTokenGen) (events.CognitoEventUserPoolsPreTokenGen, error) {
	log.Printf("Received event for user: %s", event.UserName)
	
	// Look up tenant ID based on username
	tenantID, ok := TenantMapping[event.UserName]
	if !ok {
		log.Printf("Tenant mapping not found for user %s, skipping tenant claim", event.UserName)
		return event, nil
	}

	// Initialize the claims override if needed
	if event.Response.ClaimsOverrideDetails.ClaimsToAddOrOverride == nil {
		event.Response.ClaimsOverrideDetails.ClaimsToAddOrOverride = make(map[string]string)
	}

	// Add the tenant_id claim to the token
	event.Response.ClaimsOverrideDetails.ClaimsToAddOrOverride["tenant_id"] = tenantID
	
	log.Printf("Added tenant_id claim %s for user %s", tenantID, event.UserName)
	return event, nil
}

func main() {
	lambda.Start(HandleRequest)
}