#!/bin/bash
set -e

# Build main Lambda
echo "Building main Lambda function..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o bootstrap ./cmd/lambda

# Create zip file for main Lambda
echo "Creating zip for main Lambda..."
mkdir -p .aws-sam/build/UploadFunction
cp bootstrap .aws-sam/build/UploadFunction/
(cd .aws-sam/build/UploadFunction && zip -r function.zip bootstrap)

# Build pre-token Lambda
echo "Building pre-token Lambda function..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o lambda/pre-token/bootstrap ./lambda/pre-token

# Create zip file for pre-token Lambda
echo "Creating zip for pre-token Lambda..."
mkdir -p .aws-sam/build/PreTokenGenerationLambda
cp lambda/pre-token/bootstrap .aws-sam/build/PreTokenGenerationLambda/
(cd .aws-sam/build/PreTokenGenerationLambda && zip -r function.zip bootstrap)

# Build authorizer Lambda
echo "Building authorizer Lambda function..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o lambda/authorizer/bootstrap ./lambda/authorizer

# Create zip file for authorizer Lambda
echo "Creating zip for authorizer Lambda..."
mkdir -p .aws-sam/build/TenantAuthorizerFunction
cp lambda/authorizer/bootstrap .aws-sam/build/TenantAuthorizerFunction/
(cd .aws-sam/build/TenantAuthorizerFunction && zip -r function.zip bootstrap)

echo "Build complete!"