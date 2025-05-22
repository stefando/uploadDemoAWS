package upload

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

// UploadService handles file uploads to S3 with tenant isolation
type UploadService struct {
	s3Client    *s3.Client
	bucketNames map[string]string // Maps tenant IDs to bucket names
}

// NewUploadService creates a new upload service with the provided S3 client
func NewUploadService(s3Client *s3.Client, tenantBuckets map[string]string) *UploadService {
	return &UploadService{
		s3Client:    s3Client,
		bucketNames: tenantBuckets,
	}
}

// UploadFile uploads a file to the tenant's S3 bucket with proper path formatting
func (s *UploadService) UploadFile(ctx context.Context, tenantID string, content []byte) (string, error) {
	// Get the bucket name for this tenant
	bucketName, ok := s.bucketNames[tenantID]
	if !ok {
		return "", fmt.Errorf("unknown tenant ID: %s", tenantID)
	}

	// Generate timestamp-based path (YYYY/MM/DD)
	now := time.Now().UTC()
	datePath := fmt.Sprintf("%d/%02d/%02d", now.Year(), now.Month(), now.Day())
	
	// Generate a unique filename using UUID
	fileID := uuid.New().String()
	key := fmt.Sprintf("%s/%s.json", datePath, fileID)
	
	// Create the S3 PutObject input
	input := &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
		Body:   strings.NewReader(string(content)),
		// Add content type for JSON
		ContentType: aws.String("application/json"),
	}
	
	// Upload the file to S3
	// The tenant context is automatically used by the AWS SDK
	_, err := s.s3Client.PutObject(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to upload file: %w", err)
	}
	
	// Return the file path/key
	return key, nil
}