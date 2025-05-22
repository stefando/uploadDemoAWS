package upload

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/google/uuid"
)

// UploadService handles file uploads to S3 with tenant isolation
type UploadService struct {
	stsClient  *sts.Client
	bucketName string     // Single shared bucket for all tenants
	roleArn    string     // ARN of the role to assume for tenant access
	awsConfig  aws.Config // Base AWS config for creating new clients
}

// NewUploadService creates a new upload service
func NewUploadService(cfg aws.Config, bucketName string) *UploadService {
	stsClient := sts.NewFromConfig(cfg)
	roleArn := os.Getenv("TENANT_ACCESS_ROLE_ARN")
	if roleArn == "" {
		// This will be set in the CloudFormation template
		panic("TENANT_ACCESS_ROLE_ARN environment variable not set")
	}
	
	return &UploadService{
		stsClient:  stsClient,
		bucketName: bucketName,
		roleArn:    roleArn,
		awsConfig:  cfg,
	}
}

// UploadFile uploads a file to the shared S3 bucket with tenant-prefixed path
func (s *UploadService) UploadFile(ctx context.Context, tenantID string, content []byte) (string, error) {
	// Validate tenant ID
	if tenantID == "" {
		return "", fmt.Errorf("tenant ID cannot be empty")
	}

	// Generate timestamp-based path (YYYY/MM/DD)
	now := time.Now().UTC()
	datePath := fmt.Sprintf("%d/%02d/%02d", now.Year(), now.Month(), now.Day())
	
	// Generate a unique filename using UUID
	fileID := uuid.New().String()
	// Include tenant ID as prefix in the path: //<tenant>/YYYY/MM/DD/<guid>.json
	key := fmt.Sprintf("%s/%s/%s.json", tenantID, datePath, fileID)
	
	// Assume role with tenant session tag
	assumeRoleInput := &sts.AssumeRoleInput{
		RoleArn:         aws.String(s.roleArn),
		RoleSessionName: aws.String(fmt.Sprintf("tenant-%s-session-%d", tenantID, time.Now().Unix())),
		Tags: []types.Tag{
			{
				Key:   aws.String("tenant_id"),
				Value: aws.String(tenantID),
			},
		},
		DurationSeconds: aws.Int32(3600), // 1 hour
	}
	
	assumeRoleOutput, err := s.stsClient.AssumeRole(ctx, assumeRoleInput)
	if err != nil {
		return "", fmt.Errorf("failed to assume role for tenant %s: %w", tenantID, err)
	}
	
	// Create new S3 client with the assumed role credentials
	tenantS3Client := s3.NewFromConfig(s.awsConfig, func(o *s3.Options) {
		o.Credentials = aws.NewCredentialsCache(
			aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				return aws.Credentials{
					AccessKeyID:     *assumeRoleOutput.Credentials.AccessKeyId,
					SecretAccessKey: *assumeRoleOutput.Credentials.SecretAccessKey,
					SessionToken:    *assumeRoleOutput.Credentials.SessionToken,
					Source:          "AssumeRoleProvider",
					CanExpire:       true,
					Expires:         *assumeRoleOutput.Credentials.Expiration,
				}, nil
			}),
		)
	})
	
	// Create the S3 PutObject input
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
		Body:   strings.NewReader(string(content)),
		// Add content type for JSON
		ContentType: aws.String("application/json"),
	}
	
	// Upload the file to S3 using tenant-scoped credentials
	_, err = tenantS3Client.PutObject(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}
	
	// Return the file path/key
	return key, nil
}