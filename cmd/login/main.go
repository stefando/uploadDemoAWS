package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stefando/uploadDemoAWS/internal/auth"
)

var (
	loginService *auth.LoginService
)

func init() {
	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	// Get stack name from environment variables
	stackName := os.Getenv("STACK_NAME")
	if stackName == "" {
		log.Fatal("STACK_NAME environment variable not set")
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		log.Fatal("AWS_REGION environment variable not set")
	}

	// Initialize login service
	loginService = auth.NewLoginService(cfg, stackName, region)
	log.Printf("Login service initialized for stack: %s in region: %s", stackName, region)
}

// handleLogin processes the Lambda event directly without Chi router
func handleLogin(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Only accept POST method
	if request.HTTPMethod != http.MethodPost {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusMethodNotAllowed,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error":"Method not allowed"}`,
		}, nil
	}

	// Parse request body
	var loginReq auth.LoginRequest
	if err := json.Unmarshal([]byte(request.Body), &loginReq); err != nil {
		log.Printf("Failed to parse request body: %v", err)
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error":"Invalid request body"}`,
		}, nil
	}

	// Authenticate user
	resp, err := loginService.Authenticate(ctx, &loginReq)
	if err != nil {
		log.Printf("Authentication failed: %v", err)
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusUnauthorized,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error":"Authentication failed"}`,
		}, nil
	}

	// Marshal response
	responseBody, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"error":"Internal server error"}`,
		}, nil
	}

	// Return success response
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(responseBody),
	}, nil
}

func main() {
	lambda.Start(handleLogin)
}
