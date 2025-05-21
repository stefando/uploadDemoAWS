package auth

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
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