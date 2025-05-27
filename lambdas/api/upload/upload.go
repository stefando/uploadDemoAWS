package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/google/uuid"

)

const (
	// MinSessionDuration is the minimum duration for AWS STS AssumeRole (15 minutes)
	MinSessionDuration = 900 // seconds
	
	// LongSessionDuration is the duration for operations requiring presigned URLs (3 hours)
	LongSessionDuration = 10800 // seconds
	
	// PresignedURLBuffer is the time buffer before token expiration (5 minutes)
	PresignedURLBuffer = 5 * time.Minute
	
	// MinPresignedURLDuration is the minimum duration for presigned URLs
	MinPresignedURLDuration = 5 * time.Minute
	
	// DefaultPresignedURLDuration is the default duration for presigned URLs when no token expiration
	DefaultPresignedURLDuration = 2 * time.Hour
)

// UploadService handles file uploads to S3 with tenant isolation
type UploadService struct {
	stsClient  *sts.Client
	bucketName string     // Single shared bucket for all tenants
	roleArn    string     // ARN of the role to assume for tenant access
	awsConfig  aws.Config // Base AWS config for creating new clients
}

// generateS3Key creates a unique S3 key with tenant prefix and date-based organization
func generateS3Key(tenantID string) string {
	// Generate a timestamp-based path (YYYY/MM/DD)
	now := time.Now().UTC()
	datePath := fmt.Sprintf("%d/%02d/%02d", now.Year(), now.Month(), now.Day())

	// Generate a unique filename using UUID
	fileID := uuid.New().String()

	// Include tenant ID as prefix in the path: <tenant>/YYYY/MM/DD/<guid>.json
	return fmt.Sprintf("%s/%s/%s.json", tenantID, datePath, fileID)
}

// generateS3KeyForMultipart creates a unique S3 key for multipart uploads with .raw extension
func generateS3KeyForMultipart(tenantID string) string {
	// Generate a timestamp-based path (YYYY/MM/DD)
	now := time.Now().UTC()
	datePath := fmt.Sprintf("%d/%02d/%02d", now.Year(), now.Month(), now.Day())

	// Generate a unique filename using UUID
	fileID := uuid.New().String()

	// Include tenant ID as prefix in the path: <tenant>/YYYY/MM/DD/<guid>.raw
	return fmt.Sprintf("%s/%s/%s.raw", tenantID, datePath, fileID)
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

	// Check if token has enough time left for minimum session duration
	if tokenExp, ok := GetTokenExpiration(ctx); ok {
		timeUntilExpiry := time.Unix(tokenExp, 0).Sub(time.Now())
		minDurationRequired := time.Duration(MinSessionDuration) * time.Second
		if timeUntilExpiry < minDurationRequired {
			return "", fmt.Errorf("token expires too soon for upload operation (needs at least %v, has %v)", minDurationRequired, timeUntilExpiry)
		}
	}

	// Generate the S3 key
	key := generateS3Key(tenantID)

	// Get tenant-scoped credentials
	tenantCreds, err := AssumeRoleForTenant(ctx, s.stsClient, s.roleArn, tenantID, MinSessionDuration)
	if err != nil {
		return "", err
	}

	// Create a new S3 client with the assumed role credentials
	tenantS3Client := s3.NewFromConfig(s.awsConfig, func(o *s3.Options) {
		o.Credentials = aws.NewCredentialsCache(
			aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				return tenantCreds, nil
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

// validateInitiateRequest validates the initiate multipart upload request
func validateInitiateRequest(tenantID string, req *InitiateUploadRequest) error {
	if tenantID == "" {
		return fmt.Errorf("tenant ID cannot be empty")
	}
	if req.Size <= 0 {
		return fmt.Errorf("size must be greater than zero")
	}
	if req.PartSize <= 0 {
		return fmt.Errorf("part size must be greater than zero")
	}
	return nil
}

// calculatePresignExpiration determines the expiration time for presigned URLs based on token expiration
func calculatePresignExpiration(ctx context.Context) time.Duration {
	if tokenExp, ok := GetTokenExpiration(ctx); ok {
		// Token expiration is Unix timestamp in seconds
		timeUntilExpiry := time.Unix(tokenExp, 0).Sub(time.Now())
		if timeUntilExpiry > 0 {
			// Use token expiration minus a small buffer (5 minutes)
			presignExpiration := timeUntilExpiry - PresignedURLBuffer
			if presignExpiration < MinPresignedURLDuration {
				// Minimum 5 minutes
				return MinPresignedURLDuration
			}
			return presignExpiration
		}
		// Token already expired, use minimal duration
		return MinPresignedURLDuration
	}
	// No token expiration in context, default to 2 hours
	return DefaultPresignedURLDuration
}

// generatePresignedUrls creates presigned URLs for all parts of a multipart upload
func (s *UploadService) generatePresignedUrls(ctx context.Context, presignClient *s3.PresignClient, bucketName, objectKey, uploadID string, numParts int, expiration time.Duration) (map[int]string, error) {
	presignedUrls := make(map[int]string)
	
	for i := 1; i <= numParts; i++ {
		uploadPartReq := &s3.UploadPartInput{
			Bucket:     aws.String(bucketName),
			Key:        aws.String(objectKey),
			PartNumber: aws.Int32(int32(i)),
			UploadId:   aws.String(uploadID),
		}

		presignReq, err := presignClient.PresignUploadPart(ctx, uploadPartReq, func(opts *s3.PresignOptions) {
			opts.Expires = expiration
		})
		if err != nil {
			return nil, fmt.Errorf("failed to generate presigned URL for part %d: %w", i, err)
		}

		presignedUrls[i] = presignReq.URL
	}
	
	return presignedUrls, nil
}

// InitiateMultipartUpload starts a new multipart upload and returns presigned URLs
func (s *UploadService) InitiateMultipartUpload(ctx context.Context, tenantID string, req *InitiateUploadRequest) (*InitiateUploadResponse, error) {
	// Validate inputs
	if err := validateInitiateRequest(tenantID, req); err != nil {
		return nil, err
	}

	// Generate an S3 key with date-based organization and .raw extension
	objectKey := generateS3KeyForMultipart(tenantID)

	// Get tenant-scoped credentials
	tenantCreds, err := AssumeRoleForTenant(ctx, s.stsClient, s.roleArn, tenantID, LongSessionDuration)
	if err != nil {
		return nil, err
	}

	// Create a new S3 client with the assumed role credentials
	tenantS3Client := s3.NewFromConfig(s.awsConfig, func(o *s3.Options) {
		o.Credentials = aws.NewCredentialsCache(
			aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				return tenantCreds, nil
			}),
		)
	})

	// Create presigned client
	presignClient := s3.NewPresignClient(tenantS3Client)

	// Initiate multipart upload
	createResp, err := tenantS3Client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s.bucketName),
		Key:         aws.String(objectKey),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart upload: %w", err)
	}

	// Calculate the number of parts
	numParts := int((req.Size + req.PartSize - 1) / req.PartSize)

	// Calculate presigned URL expiration based on token expiration
	presignExpiration := calculatePresignExpiration(ctx)

	// Generate presigned URLs for each part
	presignedUrls, err := s.generatePresignedUrls(ctx, presignClient, s.bucketName, objectKey, *createResp.UploadId, numParts, presignExpiration)
	if err != nil {
		// DEMOWARE DECISION: Abort on presigned URL failure
		// In production, consider returning partial success (UploadID + ObjectKey)
		// and letting client retry via /upload/refresh endpoint
		_, _ = tenantS3Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(s.bucketName),
			Key:      aws.String(objectKey),
			UploadId: createResp.UploadId,
		})
		return nil, fmt.Errorf("failed to generate presigned URLs: %w", err)
	}

	return &InitiateUploadResponse{
		PresignedUrls: presignedUrls,
		UploadID:      *createResp.UploadId,
		ObjectKey:     objectKey,
	}, nil
}

// validateCompleteRequest validates the complete multipart upload request
func validateCompleteRequest(tenantID string, req *CompleteUploadRequest) error {
	if tenantID == "" {
		return fmt.Errorf("tenant ID cannot be empty")
	}
	if req.UploadID == "" {
		return fmt.Errorf("upload ID cannot be empty")
	}
	if len(req.PartETags) == 0 {
		return fmt.Errorf("part ETags cannot be empty")
	}
	if req.ObjectKey == "" {
		return fmt.Errorf("object key cannot be empty")
	}
	return nil
}

// convertPartETags converts part ETags to AWS SDK format
func convertPartETags(partETags []PartTag) []types.CompletedPart {
	completedParts := make([]types.CompletedPart, len(partETags))
	for i, part := range partETags {
		completedParts[i] = types.CompletedPart{
			ETag:       aws.String(part.ETag),
			PartNumber: aws.Int32(int32(part.PartNumber)),
		}
	}
	return completedParts
}

// CompleteMultipartUpload completes a multipart upload
func (s *UploadService) CompleteMultipartUpload(ctx context.Context, tenantID string, req *CompleteUploadRequest) (*CompleteUploadResponse, error) {
	// Validate inputs
	if err := validateCompleteRequest(tenantID, req); err != nil {
		return nil, err
	}

	// Extract object key from upload ID (in real implementation, you'd store this mapping)
	// For demo, we'll need to pass the object key in the request or store it in a database
	// For now, we'll extract it from the first part's presigned URL or require it in the request

	// Get tenant-scoped credentials
	tenantCreds, err := AssumeRoleForTenant(ctx, s.stsClient, s.roleArn, tenantID, MinSessionDuration)
	if err != nil {
		return nil, err
	}

	// Create a new S3 client with the assumed role credentials
	tenantS3Client := s3.NewFromConfig(s.awsConfig, func(o *s3.Options) {
		o.Credentials = aws.NewCredentialsCache(
			aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				return tenantCreds, nil
			}),
		)
	})

	// Convert part ETags to the AWS SDK format
	completedParts := convertPartETags(req.PartETags)

	// Complete the multipart upload
	completeResp, err := tenantS3Client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucketName),
		Key:      aws.String(req.ObjectKey),
		UploadId: aws.String(req.UploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	return &CompleteUploadResponse{
		ObjectKey: req.ObjectKey,
		Location:  *completeResp.Location,
	}, nil
}

// AbortMultipartUpload cancels an in-progress multipart upload
func (s *UploadService) AbortMultipartUpload(ctx context.Context, tenantID string, req *AbortUploadRequest) error {
	// Validate inputs
	if tenantID == "" {
		return fmt.Errorf("tenant ID cannot be empty")
	}
	if req.UploadID == "" {
		return fmt.Errorf("upload ID cannot be empty")
	}

	// Get tenant-scoped credentials
	tenantCreds, err := AssumeRoleForTenant(ctx, s.stsClient, s.roleArn, tenantID, MinSessionDuration)
	if err != nil {
		return err
	}

	// Create a new S3 client with the assumed role credentials
	tenantS3Client := s3.NewFromConfig(s.awsConfig, func(o *s3.Options) {
		o.Credentials = aws.NewCredentialsCache(
			aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				return tenantCreds, nil
			}),
		)
	})

	// Use object key from request
	objectKey := req.ObjectKey
	if objectKey == "" {
		return fmt.Errorf("object key cannot be empty")
	}

	// Abort the multipart upload
	_, err = tenantS3Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(s.bucketName),
		Key:      aws.String(objectKey),
		UploadId: aws.String(req.UploadID),
	})
	if err != nil {
		return fmt.Errorf("failed to abort multipart upload: %w", err)
	}

	return nil
}

// validateRefreshRequest validates the refresh presigned URLs request
func validateRefreshRequest(tenantID string, req *RefreshUploadRequest) error {
	if tenantID == "" {
		return fmt.Errorf("tenant ID cannot be empty")
	}
	if req.UploadID == "" {
		return fmt.Errorf("upload ID cannot be empty")
	}
	if len(req.PartNumbers) == 0 {
		return fmt.Errorf("part numbers cannot be empty")
	}
	if req.ObjectKey == "" {
		return fmt.Errorf("object key cannot be empty")
	}
	return nil
}

// RefreshPresignedUrls refreshes presigned URLs for specified parts
func (s *UploadService) RefreshPresignedUrls(ctx context.Context, tenantID string, req *RefreshUploadRequest) (*RefreshUploadResponse, error) {
	// Validate inputs
	if err := validateRefreshRequest(tenantID, req); err != nil {
		return nil, err
	}

	// Get tenant-scoped credentials
	tenantCreds, err := AssumeRoleForTenant(ctx, s.stsClient, s.roleArn, tenantID, LongSessionDuration)
	if err != nil {
		return nil, err
	}

	// Create a new S3 client with the assumed role credentials
	tenantS3Client := s3.NewFromConfig(s.awsConfig, func(o *s3.Options) {
		o.Credentials = aws.NewCredentialsCache(
			aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
				return tenantCreds, nil
			}),
		)
	})

	// Create presigned client
	presignClient := s3.NewPresignClient(tenantS3Client)

	// Calculate presigned URL expiration based on token expiration
	presignExpiration := calculatePresignExpiration(ctx)

	// Generate refreshed presigned URLs for requested parts
	presignedUrls := make(map[int]string)
	for _, partNum := range req.PartNumbers {
		uploadPartReq := &s3.UploadPartInput{
			Bucket:     aws.String(s.bucketName),
			Key:        aws.String(req.ObjectKey),
			PartNumber: aws.Int32(int32(partNum)),
			UploadId:   aws.String(req.UploadID),
		}

		presignReq, err := presignClient.PresignUploadPart(ctx, uploadPartReq, func(opts *s3.PresignOptions) {
			opts.Expires = presignExpiration
		})
		if err != nil {
			return nil, fmt.Errorf("failed to refresh presigned URL for part %d: %w", partNum, err)
		}

		presignedUrls[partNum] = presignReq.URL
	}

	return &RefreshUploadResponse{
		PresignedUrls: presignedUrls,
	}, nil
}
