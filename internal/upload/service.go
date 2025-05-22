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
	s3Client   *s3.Client
	bucketName string // Single shared bucket for all tenants
}

// NewUploadService creates a new upload service with the provided S3 client
func NewUploadService(s3Client *s3.Client, bucketName string) *UploadService {
	return &UploadService{
		s3Client:   s3Client,
		bucketName: bucketName,
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
	
	// Create the S3 PutObject input
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucketName),
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