package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Global variables to hold initialized services
var (
	uploadService *UploadService
)

// Init initializes the AWS clients and services
func init() {
	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	// Get the shared bucket name from environment variable
	sharedBucket := os.Getenv("SHARED_BUCKET")
	if sharedBucket == "" {
		log.Fatal("SHARED_BUCKET environment variable not set")
	}

	// Initialize upload service with AWS config and bucket name
	uploadService = NewUploadService(cfg, sharedBucket)

	log.Printf("Services initialized with shared bucket: %s", sharedBucket)
}

// setupRouter creates and configures the Chi router
func setupRouter() *chi.Mux {
	r := chi.NewRouter()

	// Middleware for all routes
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// API routes
	r.Route("/upload", func(r chi.Router) {
		r.Post("/", handleUpload)
		r.Post("/initiate", handleInitiateUpload)
		r.Post("/complete", handleCompleteUpload)
		r.Post("/abort", handleAbortUpload)
		r.Post("/refresh", handleRefreshUpload)
	})

	// Health check endpoint
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	return r
}

// handleUpload processes file upload requests
func handleUpload(w http.ResponseWriter, r *http.Request) {
	// Get tenant ID from the context (set by Lambda authorizer)
	tenantID, ok := GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Tenant ID not found in request context", http.StatusUnauthorized)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Validate JSON format
	var jsonData interface{}
	if err := json.Unmarshal(body, &jsonData); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// Use the context that already has tenant information
	ctx := r.Context()

	// Upload the file to S3
	filePath, err := uploadService.UploadFile(ctx, tenantID, body)
	if err != nil {
		log.Printf("Upload error: %v", err)
		http.Error(w, "Failed to upload file", http.StatusInternalServerError)
		return
	}

	// Return success response with file path
	response := map[string]string{
		"status":    "success",
		"file_path": filePath,
		"tenant_id": tenantID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}

// handleInitiateUpload handles multipart upload initiation
func handleInitiateUpload(w http.ResponseWriter, r *http.Request) {
	// Get tenant ID from the context
	tenantID, ok := GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Tenant ID not found in request context", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var req InitiateUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Initiate multipart upload
	resp, err := uploadService.InitiateMultipartUpload(r.Context(), tenantID, &req)
	if err != nil {
		log.Printf("Initiate upload error: %v", err)
		http.Error(w, "Failed to initiate upload", http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// handleCompleteUpload handles multipart upload completion
func handleCompleteUpload(w http.ResponseWriter, r *http.Request) {
	// Get tenant ID from the context
	tenantID, ok := GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Tenant ID not found in request context", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var req CompleteUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Complete multipart upload
	resp, err := uploadService.CompleteMultipartUpload(r.Context(), tenantID, &req)
	if err != nil {
		log.Printf("Complete upload error: %v", err)
		http.Error(w, "Failed to complete upload", http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// handleAbortUpload handles multipart upload abort
func handleAbortUpload(w http.ResponseWriter, r *http.Request) {
	// Get tenant ID from the context
	tenantID, ok := GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Tenant ID not found in request context", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var req AbortUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Abort multipart upload
	if err := uploadService.AbortMultipartUpload(r.Context(), tenantID, &req); err != nil {
		log.Printf("Abort upload error: %v", err)
		http.Error(w, "Failed to abort upload", http.StatusInternalServerError)
		return
	}

	// Return success response
	w.WriteHeader(http.StatusNoContent)
}

// handleRefreshUpload handles refreshing presigned URLs for multipart upload
func handleRefreshUpload(w http.ResponseWriter, r *http.Request) {
	// Get tenant ID from the context
	tenantID, ok := GetTenantID(r.Context())
	if !ok {
		http.Error(w, "Tenant ID not found in request context", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var req RefreshUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Refresh presigned URLs
	resp, err := uploadService.RefreshPresignedUrls(r.Context(), tenantID, &req)
	if err != nil {
		log.Printf("Refresh upload error: %v", err)
		http.Error(w, "Failed to refresh presigned URLs", http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// lambdaHandler is the main Lambda handler function that adapts API Gateway events
// to the Chi router
func lambdaHandler(ctx context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Create a new http.Request from the API Gateway event
	httpReq, err := createHTTPRequest(ctx, req)
	if err != nil {
		log.Printf("Error creating HTTP request: %v", err)
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "Internal server error",
		}, nil
	}

	// Extract the tenant ID and token expiration from API Gateway REQUEST authorizer context
	if req.RequestContext.Authorizer != nil {
		// For REQUEST authorizers, context is directly in Authorizer map
		ctx = httpReq.Context()
		
		if tenantID, exists := req.RequestContext.Authorizer["tenant_id"].(string); exists && tenantID != "" {
			// Add tenant ID to request context
			ctx = WithTenantID(ctx, tenantID)
			log.Printf("Tenant ID from REQUEST authorizer context: %s", tenantID)
		} else {
			log.Printf("No tenant_id found in authorizer context: %+v", req.RequestContext.Authorizer)
		}
		
		// Extract token expiration
		if tokenExp, exists := req.RequestContext.Authorizer["token_expiration"].(float64); exists {
			// Convert float64 to int64 (API Gateway converts numbers to float64)
			ctx = WithTokenExpiration(ctx, int64(tokenExp))
			log.Printf("Token expiration from REQUEST authorizer context: %d", int64(tokenExp))
		}
		
		httpReq = httpReq.WithContext(ctx)
	}

	// Create a response recorder to capture Chi's response
	respRecorder := &responseRecorder{
		headers:    make(map[string]string),
		statusCode: http.StatusOK, // Default status
	}

	// Process the request through the Chi router
	router := setupRouter()
	router.ServeHTTP(respRecorder, httpReq)

	// Convert the captured response to an API Gateway response
	return events.APIGatewayProxyResponse{
		StatusCode: respRecorder.statusCode,
		Headers:    respRecorder.headers,
		Body:       string(respRecorder.body),
	}, nil
}

// createHTTPRequest creates an http.Request from an API Gateway event
func createHTTPRequest(ctx context.Context, req events.APIGatewayProxyRequest) (*http.Request, error) {
	// Create a new HTTP request
	var body io.Reader
	if req.Body != "" {
		body = io.NopCloser(strings.NewReader(req.Body))
	}

	// Determine the full request path
	path := req.Path
	if req.PathParameters != nil {
		for param, value := range req.PathParameters {
			path = strings.Replace(path, "{"+param+"}", value, -1)
		}
	}

	// Create the HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, req.HTTPMethod, path, body)
	if err != nil {
		return nil, err
	}

	// Add query parameters
	if req.QueryStringParameters != nil {
		query := httpReq.URL.Query()
		for param, value := range req.QueryStringParameters {
			query.Add(param, value)
		}
		httpReq.URL.RawQuery = query.Encode()
	}

	// Add headers
	for key, value := range req.Headers {
		httpReq.Header.Add(key, value)
	}

	return httpReq, nil
}

// responseRecorder captures Chi's HTTP response
type responseRecorder struct {
	headers    map[string]string
	body       []byte
	statusCode int
}


// Header implements the http.ResponseWriter interface
func (r *responseRecorder) Header() http.Header {
	httpHeader := http.Header{}
	for key, value := range r.headers {
		httpHeader.Add(key, value)
	}
	return httpHeader
}

// Write implements the http.ResponseWriter interface
func (r *responseRecorder) Write(body []byte) (int, error) {
	r.body = append(r.body, body...)
	return len(body), nil
}

// WriteHeader implements the http.ResponseWriter interface
func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}

func main() {
	lambda.Start(lambdaHandler)
}
