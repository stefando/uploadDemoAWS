package main

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/lambda"
)

// TenantMapping defines which users belong to which tenants
// In a real application, this would come from a database
var TenantMapping = map[string]string{
	"user-tenant-a": "tenant-a",
	"user-tenant-b": "tenant-b",
}

// CognitoEventRequest represents the Lambda request from Cognito
type CognitoEventRequest struct {
	Version               string                 `json:"version"`
	TriggerSource         string                 `json:"triggerSource"`
	Region                string                 `json:"region"`
	UserPoolID            string                 `json:"userPoolId"`
	CallerContext         CallerContext          `json:"callerContext"`
	Request               TokenGenerationRequest `json:"request"`
	Response              TokenGenerationResponse `json:"response"`
}

// CallerContext provides information about the caller
type CallerContext struct {
	AWSSDKVersion string `json:"awsSdkVersion"`
	ClientID      string `json:"clientId"`
}

// TokenGenerationRequest contains the user attributes and token claims
type TokenGenerationRequest struct {
	UserAttributes map[string]string `json:"userAttributes"`
	ClientMetadata map[string]string `json:"clientMetadata"`
	GroupConfiguration GroupConfig `json:"groupConfiguration"`
}

// GroupConfig contains user group information
type GroupConfig struct {
	GroupsToOverride []string `json:"groupsToOverride"`
	IAMRolesToOverride []string `json:"iamRolesToOverride"`
	PreferredRole string `json:"preferredRole"`
}

// TokenGenerationResponse contains the modified token claims
type TokenGenerationResponse struct {
	ClaimsOverrideDetails ClaimsOverrideDetails `json:"claimsOverrideDetails"`
}

// ClaimsOverrideDetails allows modifying token claims
type ClaimsOverrideDetails struct {
	GroupOverrideDetails GroupOverrideDetails `json:"groupOverrideDetails"`
	ClaimsToAddOrOverride map[string]string `json:"claimsToAddOrOverride"`
	ClaimsToSuppress []string `json:"claimsToSuppress"`
}

// GroupOverrideDetails contains group membership overrides
type GroupOverrideDetails struct {
	GroupsToOverride []string `json:"groupsToOverride"`
	IAMRolesToOverride []string `json:"iamRolesToOverride"`
	PreferredRole string `json:"preferredRole"`
}

// HandleRequest processes the Cognito event and adds tenant claims
func HandleRequest(ctx context.Context, event CognitoEventRequest) (CognitoEventRequest, error) {
	// Extract the username from the request
	username, ok := event.Request.UserAttributes["cognito:username"]
	if !ok {
		log.Printf("Username not found in event, skipping tenant claim")
		return event, nil
	}

	// Look up tenant ID based on username
	tenantID, ok := TenantMapping[username]
	if !ok {
		log.Printf("Tenant mapping not found for user %s, skipping tenant claim", username)
		return event, nil
	}

	// Initialize the claims override if needed
	if event.Response.ClaimsOverrideDetails.ClaimsToAddOrOverride == nil {
		event.Response.ClaimsOverrideDetails.ClaimsToAddOrOverride = make(map[string]string)
	}

	// Add the tenant_id claim to the token
	event.Response.ClaimsOverrideDetails.ClaimsToAddOrOverride["tenant_id"] = tenantID
	
	log.Printf("Added tenant_id claim %s for user %s", tenantID, username)
	return event, nil
}

func main() {
	lambda.Start(HandleRequest)
}