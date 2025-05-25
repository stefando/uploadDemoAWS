#!/bin/bash
set -e

# Build main Lambda
echo "Building main Lambda function..."
mkdir -p .aws-sam/build/UploadFunction
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o .aws-sam/build/UploadFunction/bootstrap ./cmd/lambda

# Create zip file for main Lambda
echo "Creating zip for main Lambda..."
(cd .aws-sam/build/UploadFunction && zip -r function.zip bootstrap)

# Build pre-token Lambda
echo "Building pre-token Lambda function..."
mkdir -p .aws-sam/build/PreTokenGenerationLambda
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o .aws-sam/build/PreTokenGenerationLambda/bootstrap ./lambda/pre-token

# Create zip file for pre-token Lambda
echo "Creating zip for pre-token Lambda..."
(cd .aws-sam/build/PreTokenGenerationLambda && zip -r function.zip bootstrap)

# Build login Lambda
echo "Building login Lambda function..."
mkdir -p .aws-sam/build/LoginFunction
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o .aws-sam/build/LoginFunction/bootstrap ./cmd/login

# Create zip file for login Lambda
echo "Creating zip for login Lambda..."
(cd .aws-sam/build/LoginFunction && zip -r function.zip bootstrap)

# Build authorizer Lambda
echo "Building authorizer Lambda function..."
mkdir -p .aws-sam/build/TenantAuthorizerFunction
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o .aws-sam/build/TenantAuthorizerFunction/bootstrap ./lambda/authorizer

# Create zip file for authorizer Lambda
echo "Creating zip for authorizer Lambda..."
(cd .aws-sam/build/TenantAuthorizerFunction && zip -r function.zip bootstrap)

echo "Build complete!"