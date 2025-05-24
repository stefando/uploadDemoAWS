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
	cognitoClient *cognitoidentityprovider.Client
	stackName     string
}

// LoginRequest represents the login request payload
type LoginRequest struct {
	Tenant   string `json:"tenant"`
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
func NewLoginService(cfg aws.Config, stackName string) *LoginService {
	return &LoginService{
		cognitoClient: cognitoidentityprovider.NewFromConfig(cfg),
		stackName:     stackName,
	}
}

// Authenticate performs user authentication with Cognito
func (s *LoginService) Authenticate(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	// Validate input
	if req.Tenant == "" || req.Username == "" || req.Password == "" {
		return nil, fmt.Errorf("tenant, username, and password are required")
	}

	// Discover user pool and client by naming convention
	userPoolName := fmt.Sprintf("%s-%s-user-pool", s.stackName, req.Tenant)
	userPoolID, err := s.findUserPoolByName(ctx, userPoolName)
	if err != nil {
		return nil, fmt.Errorf("failed to find user pool for tenant %s: %w", req.Tenant, err)
	}

	// Get the user pool client
	clientID, err := s.findUserPoolClient(ctx, userPoolID, fmt.Sprintf("%s-%s-client", s.stackName, req.Tenant))
	if err != nil {
		return nil, fmt.Errorf("failed to find user pool client: %w", err)
	}

	// Prepare auth parameters
	authParams := map[string]string{
		"USERNAME": req.Username,
		"PASSWORD": req.Password,
	}

	// Call Cognito InitiateAuth
	input := &cognitoidentityprovider.InitiateAuthInput{
		AuthFlow:       types.AuthFlowTypeUserPasswordAuth,
		ClientId:       aws.String(clientID),
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
		ExpiresIn: result.AuthenticationResult.ExpiresIn,
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

// findUserPoolByName discovers a user pool by its name
func (s *LoginService) findUserPoolByName(ctx context.Context, poolName string) (string, error) {
	paginator := cognitoidentityprovider.NewListUserPoolsPaginator(s.cognitoClient, &cognitoidentityprovider.ListUserPoolsInput{
		MaxResults: aws.Int32(60),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to list user pools: %w", err)
		}

		for _, pool := range page.UserPools {
			if pool.Name != nil && *pool.Name == poolName {
				return *pool.Id, nil
			}
		}
	}

	return "", fmt.Errorf("user pool not found: %s", poolName)
}

// findUserPoolClient discovers a user pool client by name
func (s *LoginService) findUserPoolClient(ctx context.Context, userPoolID, clientName string) (string, error) {
	paginator := cognitoidentityprovider.NewListUserPoolClientsPaginator(s.cognitoClient, &cognitoidentityprovider.ListUserPoolClientsInput{
		UserPoolId: aws.String(userPoolID),
		MaxResults: aws.Int32(60),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to list user pool clients: %w", err)
		}

		for _, client := range page.UserPoolClients {
			// Get detailed client information to check the name
			describeOutput, err := s.cognitoClient.DescribeUserPoolClient(ctx, &cognitoidentityprovider.DescribeUserPoolClientInput{
				UserPoolId: aws.String(userPoolID),
				ClientId:   client.ClientId,
			})
			if err != nil {
				continue
			}

			if describeOutput.UserPoolClient != nil &&
				describeOutput.UserPoolClient.ClientName != nil &&
				*describeOutput.UserPoolClient.ClientName == clientName {
				return *client.ClientId, nil
			}
		}
	}

	return "", fmt.Errorf("user pool client not found: %s", clientName)
}
