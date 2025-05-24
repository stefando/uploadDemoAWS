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

	// Get Cognito configuration from environment variables
	userPoolClientID := os.Getenv("USER_POOL_CLIENT_ID")
	if userPoolClientID == "" {
		log.Fatal("USER_POOL_CLIENT_ID environment variable not set")
	}
	userPoolID := os.Getenv("USER_POOL_ID")
	if userPoolID == "" {
		log.Fatal("USER_POOL_ID environment variable not set")
	}

	// Initialize login service
	loginService = auth.NewLoginService(cfg, userPoolID, userPoolClientID)
	log.Printf("Login service initialized for user pool: %s", userPoolID)
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
