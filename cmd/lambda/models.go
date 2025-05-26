package main

// InitiateUploadRequest represents the request to initiate a multipart upload
type InitiateUploadRequest struct {
	ContainerKey string `json:"containerKey"`
	Size         int64  `json:"size"`
	PartSize     int64  `json:"partSize"`
}

// InitiateUploadResponse contains presigned URLs and upload metadata
type InitiateUploadResponse struct {
	PresignedUrls map[int]string `json:"presignedUrls"`
	UploadID      string         `json:"uploadId"`
	ObjectKey     string         `json:"objectKey"`
}

// PartTag represents a completed part with its ETag
type PartTag struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"eTag"`
}

// CompleteUploadRequest represents the request to complete a multipart upload
type CompleteUploadRequest struct {
	UploadID  string    `json:"uploadId"`
	ObjectKey string    `json:"objectKey"`
	PartETags []PartTag `json:"partETags"`
}

// CompleteUploadResponse contains the final object location
type CompleteUploadResponse struct {
	ObjectKey string `json:"objectKey"`
	Location  string `json:"location"`
}

// AbortUploadRequest represents the request to abort a multipart upload
type AbortUploadRequest struct {
	UploadID  string `json:"uploadId"`
	ObjectKey string `json:"objectKey"`
}

// RefreshUploadRequest represents the request to refresh presigned URLs
type RefreshUploadRequest struct {
	UploadID    string `json:"uploadId"`
	ObjectKey   string `json:"objectKey"`
	PartNumbers []int  `json:"partNumbers"`
}

// RefreshUploadResponse contains refreshed presigned URLs
type RefreshUploadResponse struct {
	PresignedUrls map[int]string `json:"presignedUrls"`
}
