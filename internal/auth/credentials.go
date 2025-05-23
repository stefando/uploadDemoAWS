package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
)

// TenantInfo is a key type for storing tenant information in context
type TenantInfo string

// ContextTenantKey is the key used to store tenant information in context
const ContextTenantKey TenantInfo = "tenant_id"

// TenantTaggedCredentialsProvider adds tenant tags to AWS credentials
// This is a custom implementation to modify the session token with tenant information
// without using STS AssumeRole operations
type TenantTaggedCredentialsProvider struct {
	Source   aws.CredentialsProvider
	TenantID string
}

// Retrieve implements the aws.CredentialsProvider interface
// It gets credentials from the underlying provider and adds tenant information
func (t *TenantTaggedCredentialsProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	// Get credentials from the source provider
	creds, err := t.Source.Retrieve(ctx)
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("failed to retrieve base credentials: %w", err)
	}

	// Add tenant tag to the credentials
	// In a real implementation, this would use a more sophisticated approach to modify
	// the session token, possibly with JWT or similar
	creds.SessionToken = fmt.Sprintf("%s;tenantId=%s", creds.SessionToken, t.TenantID)

	return creds, nil
}

// WithTenantID adds tenant ID to the context
// This function should be called when processing requests to ensure the tenant context
// is properly propagated to AWS API calls
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, ContextTenantKey, tenantID)
}

// GetTenantID retrieves tenant ID from context
func GetTenantID(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(ContextTenantKey).(string)
	return val, ok
}

// AssumeRoleForTenant assumes an IAM role with tenant-specific session tags
// This enables fine-grained access control based on tenant identity
func AssumeRoleForTenant(ctx context.Context, stsClient *sts.Client, roleArn, tenantID string) (aws.Credentials, error) {
	if tenantID == "" {
		return aws.Credentials{}, fmt.Errorf("tenant ID cannot be empty")
	}

	if roleArn == "" {
		return aws.Credentials{}, fmt.Errorf("role ARN cannot be empty")
	}

	// Create session name with tenant ID and timestamp for uniqueness
	sessionName := fmt.Sprintf("tenant-%s-session-%d", tenantID, time.Now().Unix())

	// Prepare assume role input with tenant session tag
	assumeRoleInput := &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleArn),
		RoleSessionName: aws.String(sessionName),
		Tags: []types.Tag{
			{
				Key:   aws.String("tenant_id"),
				Value: aws.String(tenantID),
			},
		},
		DurationSeconds: aws.Int32(10800), // 3 hours (to ensure 2+ hours of validity)
	}

	// Assume the role
	assumeRoleOutput, err := stsClient.AssumeRole(ctx, assumeRoleInput)
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("failed to assume role for tenant %s: %w", tenantID, err)
	}

	// Convert STS credentials to AWS SDK credentials
	return aws.Credentials{
		AccessKeyID:     *assumeRoleOutput.Credentials.AccessKeyId,
		SecretAccessKey: *assumeRoleOutput.Credentials.SecretAccessKey,
		SessionToken:    *assumeRoleOutput.Credentials.SessionToken,
		Source:          "AssumeRoleProvider",
		CanExpire:       true,
		Expires:         *assumeRoleOutput.Credentials.Expiration,
	}, nil
}
