package upload

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

	"github.com/stefando/uploadDemoAWS/internal/auth"
	"github.com/stefando/uploadDemoAWS/internal/models"
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
	// Generate timestamp-based path (YYYY/MM/DD)
	now := time.Now().UTC()
	datePath := fmt.Sprintf("%d/%02d/%02d", now.Year(), now.Month(), now.Day())

	// Generate a unique filename using UUID
	fileID := uuid.New().String()

	// Include tenant ID as prefix in the path: <tenant>/YYYY/MM/DD/<guid>.json
	return fmt.Sprintf("%s/%s/%s.json", tenantID, datePath, fileID)
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

	// Generate the S3 key
	key := generateS3Key(tenantID)

	// Get tenant-scoped credentials
	tenantCreds, err := auth.AssumeRoleForTenant(ctx, s.stsClient, s.roleArn, tenantID)
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

// InitiateMultipartUpload starts a new multipart upload and returns presigned URLs
func (s *UploadService) InitiateMultipartUpload(ctx context.Context, tenantID string, req *models.InitiateUploadRequest) (*models.InitiateUploadResponse, error) {
	// Validate inputs
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID cannot be empty")
	}
	if req.Size <= 0 {
		return nil, fmt.Errorf("size must be greater than zero")
	}
	if req.PartSize <= 0 {
		return nil, fmt.Errorf("part size must be greater than zero")
	}

	// Generate S3 key with container key prefix
	objectKey := fmt.Sprintf("%s/%s/%s", tenantID, req.ContainerKey, uuid.New().String())

	// Get tenant-scoped credentials
	tenantCreds, err := auth.AssumeRoleForTenant(ctx, s.stsClient, s.roleArn, tenantID)
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

	// Calculate number of parts
	numParts := int((req.Size + req.PartSize - 1) / req.PartSize)
	presignedUrls := make(map[int]string)

	// Generate presigned URLs for each part
	for i := 1; i <= numParts; i++ {
		uploadPartReq := &s3.UploadPartInput{
			Bucket:     aws.String(s.bucketName),
			Key:        aws.String(objectKey),
			PartNumber: aws.Int32(int32(i)),
			UploadId:   createResp.UploadId,
		}

		presignReq, err := presignClient.PresignUploadPart(ctx, uploadPartReq, func(opts *s3.PresignOptions) {
			opts.Expires = time.Duration(2 * time.Hour) // 2 hours validity
		})
		if err != nil {
			// Clean up the multipart upload
			tenantS3Client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
				Bucket:   aws.String(s.bucketName),
				Key:      aws.String(objectKey),
				UploadId: createResp.UploadId,
			})
			return nil, fmt.Errorf("failed to generate presigned URL for part %d: %w", i, err)
		}

		presignedUrls[i] = presignReq.URL
	}

	return &models.InitiateUploadResponse{
		PresignedUrls: presignedUrls,
		UploadID:      *createResp.UploadId,
		ObjectKey:     objectKey,
	}, nil
}

// CompleteMultipartUpload completes a multipart upload
func (s *UploadService) CompleteMultipartUpload(ctx context.Context, tenantID string, req *models.CompleteUploadRequest) (*models.CompleteUploadResponse, error) {
	// Validate inputs
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID cannot be empty")
	}
	if req.UploadID == "" {
		return nil, fmt.Errorf("upload ID cannot be empty")
	}
	if len(req.PartETags) == 0 {
		return nil, fmt.Errorf("part ETags cannot be empty")
	}

	// Extract object key from upload ID (in real implementation, you'd store this mapping)
	// For demo, we'll need to pass the object key in the request or store it in a database
	// For now, we'll extract it from the first part's presigned URL or require it in the request

	// Get tenant-scoped credentials
	tenantCreds, err := auth.AssumeRoleForTenant(ctx, s.stsClient, s.roleArn, tenantID)
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

	// Convert part ETags to AWS SDK format
	completedParts := make([]types.CompletedPart, len(req.PartETags))
	for i, part := range req.PartETags {
		completedParts[i] = types.CompletedPart{
			ETag:       aws.String(part.ETag),
			PartNumber: aws.Int32(int32(part.PartNumber)),
		}
	}

	// Use object key from request
	objectKey := req.ObjectKey
	if objectKey == "" {
		return nil, fmt.Errorf("object key cannot be empty")
	}

	// Complete the multipart upload
	completeResp, err := tenantS3Client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(s.bucketName),
		Key:      aws.String(objectKey),
		UploadId: aws.String(req.UploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	return &models.CompleteUploadResponse{
		ObjectKey: objectKey,
		Location:  *completeResp.Location,
	}, nil
}

// AbortMultipartUpload cancels an in-progress multipart upload
func (s *UploadService) AbortMultipartUpload(ctx context.Context, tenantID string, req *models.AbortUploadRequest) error {
	// Validate inputs
	if tenantID == "" {
		return fmt.Errorf("tenant ID cannot be empty")
	}
	if req.UploadID == "" {
		return fmt.Errorf("upload ID cannot be empty")
	}

	// Get tenant-scoped credentials
	tenantCreds, err := auth.AssumeRoleForTenant(ctx, s.stsClient, s.roleArn, tenantID)
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

// RefreshPresignedUrls refreshes presigned URLs for specified parts
func (s *UploadService) RefreshPresignedUrls(ctx context.Context, tenantID string, req *models.RefreshUploadRequest) (*models.RefreshUploadResponse, error) {
	// Validate inputs
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID cannot be empty")
	}
	if req.UploadID == "" {
		return nil, fmt.Errorf("upload ID cannot be empty")
	}
	if len(req.PartNumbers) == 0 {
		return nil, fmt.Errorf("part numbers cannot be empty")
	}

	// Get tenant-scoped credentials
	tenantCreds, err := auth.AssumeRoleForTenant(ctx, s.stsClient, s.roleArn, tenantID)
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

	// Use object key from request
	objectKey := req.ObjectKey
	if objectKey == "" {
		return nil, fmt.Errorf("object key cannot be empty")
	}

	// Generate new presigned URLs for requested parts
	presignedUrls := make(map[int]string)
	for _, partNum := range req.PartNumbers {
		uploadPartReq := &s3.UploadPartInput{
			Bucket:     aws.String(s.bucketName),
			Key:        aws.String(objectKey),
			PartNumber: aws.Int32(int32(partNum)),
			UploadId:   aws.String(req.UploadID),
		}

		presignReq, err := presignClient.PresignUploadPart(ctx, uploadPartReq, func(opts *s3.PresignOptions) {
			opts.Expires = time.Duration(2 * time.Hour) // 2 hours validity
		})
		if err != nil {
			return nil, fmt.Errorf("failed to refresh presigned URL for part %d: %w", partNum, err)
		}

		presignedUrls[partNum] = presignReq.URL
	}

	return &models.RefreshUploadResponse{
		PresignedUrls: presignedUrls,
	}, nil
}
