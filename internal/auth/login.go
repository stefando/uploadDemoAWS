package auth

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
)

// LoginService handles authentication with AWS Cognito
type LoginService struct {
	cognitoClient    *cognitoidentityprovider.Client
	userPoolID       string
	userPoolClientID string
}

// LoginRequest represents the login request payload
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents the login response with tokens
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int32  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// NewLoginService creates a new login service instance
func NewLoginService(cfg aws.Config, userPoolID, userPoolClientID string) *LoginService {
	return &LoginService{
		cognitoClient:    cognitoidentityprovider.NewFromConfig(cfg),
		userPoolID:       userPoolID,
		userPoolClientID: userPoolClientID,
	}
}

// Authenticate performs user authentication with Cognito
func (s *LoginService) Authenticate(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	// Validate input
	if req.Username == "" || req.Password == "" {
		return nil, fmt.Errorf("username and password are required")
	}

	// Prepare auth parameters
	authParams := map[string]string{
		"USERNAME": req.Username,
		"PASSWORD": req.Password,
	}

	// Call Cognito InitiateAuth
	input := &cognitoidentityprovider.InitiateAuthInput{
		AuthFlow:       types.AuthFlowTypeUserPasswordAuth,
		ClientId:       aws.String(s.userPoolClientID),
		AuthParameters: authParams,
	}

	result, err := s.cognitoClient.InitiateAuth(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Check if we got authentication result
	if result.AuthenticationResult == nil {
		return nil, fmt.Errorf("unexpected authentication response")
	}

	// Build response
	response := &LoginResponse{
		TokenType: "Bearer",
		ExpiresIn: aws.ToInt32(result.AuthenticationResult.ExpiresIn),
	}

	// Include tokens if present
	if result.AuthenticationResult.AccessToken != nil {
		response.AccessToken = *result.AuthenticationResult.AccessToken
	}
	if result.AuthenticationResult.IdToken != nil {
		response.IDToken = *result.AuthenticationResult.IdToken
	}
	if result.AuthenticationResult.RefreshToken != nil {
		response.RefreshToken = *result.AuthenticationResult.RefreshToken
	}

	return response, nil
}
