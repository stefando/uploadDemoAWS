package main

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

// TokenExpiration is a key type for storing token expiration in context
type TokenExpiration string

// ContextTenantKey is the key used to store tenant information in context
const ContextTenantKey TenantInfo = "tenant_id"

// ContextTokenExpirationKey is the key used to store token expiration in context
const ContextTokenExpirationKey TokenExpiration = "token_expiration"

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

// WithTokenExpiration adds token expiration to the context
func WithTokenExpiration(ctx context.Context, expiration int64) context.Context {
	return context.WithValue(ctx, ContextTokenExpirationKey, expiration)
}

// GetTokenExpiration retrieves token expiration from context
func GetTokenExpiration(ctx context.Context) (int64, bool) {
	val, ok := ctx.Value(ContextTokenExpirationKey).(int64)
	return val, ok
}

// AssumeRoleForTenant assumes an IAM role with tenant-specific session tags
// This enables fine-grained access control based on tenant identity
// durationSeconds controls how long the credentials are valid (max 10800 for our role)
func AssumeRoleForTenant(ctx context.Context, stsClient *sts.Client, roleArn, tenantID string, durationSeconds int32) (aws.Credentials, error) {
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
		DurationSeconds: aws.Int32(durationSeconds),
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
