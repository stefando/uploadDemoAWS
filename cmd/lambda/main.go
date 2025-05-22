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
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/stefando/uploadDemoAWS/internal/auth"
	"github.com/stefando/uploadDemoAWS/internal/upload"
)

// Global variables to hold initialized services
var (
	s3Client      *s3.Client
	uploadService *upload.UploadService
)

// Init initializes the AWS clients and services
func init() {
	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	// Initialize S3 client
	s3Client = s3.NewFromConfig(cfg)

	// Get the shared bucket name from environment variable
	sharedBucket := os.Getenv("SHARED_BUCKET")
	if sharedBucket == "" {
		log.Fatal("SHARED_BUCKET environment variable not set")
	}

	// Initialize upload service with the shared bucket
	uploadService = upload.NewUploadService(s3Client, sharedBucket)

	log.Printf("Services initialized with shared bucket: %s", sharedBucket)
}

// setupRouter creates and configures the Chi router
func setupRouter() *chi.Mux {
	r := chi.NewRouter()

	// Middleware for all routes
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(auth.TenantMiddleware) // Add tenant information to all requests

	// API routes
	r.Route("/upload", func(r chi.Router) {
		r.Post("/", handleUpload)
		// Future endpoints would go here as we expand the API
	})

	// Health check endpoint
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	return r
}

// handleUpload processes file upload requests
func handleUpload(w http.ResponseWriter, r *http.Request) {
	// Get tenant ID from the context (set by TenantMiddleware)
	tenantID, ok := auth.GetTenantID(r.Context())
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

	// Use the context that already has tenant information (from middleware)
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
		"status":   "success",
		"file_path": filePath,
		"tenant_id": tenantID,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
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

	// Extract tenant ID from API Gateway REQUEST authorizer context
	if req.RequestContext.Authorizer != nil {
		// For REQUEST authorizers, context is directly in Authorizer map
		if tenantID, exists := req.RequestContext.Authorizer["tenant_id"].(string); exists && tenantID != "" {
			// Add tenant ID directly to request context
			ctx = auth.WithTenantID(httpReq.Context(), tenantID)
			httpReq = httpReq.WithContext(ctx)
			log.Printf("Tenant ID from REQUEST authorizer context: %s", tenantID)
		} else {
			log.Printf("No tenant_id found in authorizer context: %+v", req.RequestContext.Authorizer)
		}
	}

	// Create a response recorder to capture Chi's response
	respRecorder := newResponseRecorder()

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
		body = io.NopCloser(io.Reader(io.Reader(strings.NewReader(req.Body))))
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

// newResponseRecorder creates a new response recorder
func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		headers:    make(map[string]string),
		statusCode: http.StatusOK, // Default status
	}
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