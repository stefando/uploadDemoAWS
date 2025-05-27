package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var (
	dynamoClient *dynamodb.Client
	tableName    string
)

func init() {
	// Initialize the DynamoDB client
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	
	dynamoClient = dynamodb.NewFromConfig(cfg)
	tableName = os.Getenv("TABLE_NAME")
	if tableName == "" {
		log.Fatal("TABLE_NAME environment variable not set")
	}
}

// HandleRequest processes the Cognito Pre Token Generation V2_0 event
func HandleRequest(ctx context.Context, event events.CognitoEventUserPoolsPreTokenGenV2_0) (events.CognitoEventUserPoolsPreTokenGenV2_0, error) {
	log.Printf("Received event for user: %s in pool: %s", event.UserName, event.UserPoolID)

	// Look up the tenant ID from DynamoDB using the pool ID
	result, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &tableName,
		Key: map[string]types.AttributeValue{
			"pool_id": &types.AttributeValueMemberS{Value: event.UserPoolID},
		},
	})
	
	if err != nil {
		log.Printf("Failed to look up tenant for pool %s: %v", event.UserPoolID, err)
		return event, nil
	}
	
	if result.Item == nil {
		log.Printf("No tenant mapping found for pool %s", event.UserPoolID)
		return event, nil
	}
	
	// Extract the tenant ID from the result
	tenantAttr, ok := result.Item["tenant_id"]
	if !ok {
		log.Printf("No tenant_id attribute in mapping for pool %s", event.UserPoolID)
		return event, nil
	}
	
	tenantIDValue, ok := tenantAttr.(*types.AttributeValueMemberS)
	if !ok || tenantIDValue.Value == "" {
		log.Printf("Invalid tenant_id value for pool %s", event.UserPoolID)
		return event, nil
	}
	
	tenantID := tenantIDValue.Value
	log.Printf("Found tenant ID: %s for pool: %s", tenantID, event.UserPoolID)

	// Add the tenant_id claim to ID tokens
	if event.Response.ClaimsAndScopeOverrideDetails.IDTokenGeneration.ClaimsToAddOrOverride == nil {
		event.Response.ClaimsAndScopeOverrideDetails.IDTokenGeneration.ClaimsToAddOrOverride = make(map[string]interface{})
	}
	event.Response.ClaimsAndScopeOverrideDetails.IDTokenGeneration.ClaimsToAddOrOverride["tenant_id"] = tenantID

	// Add tenant_id to the access tokens (KEY for API Gateway authorization!)
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
