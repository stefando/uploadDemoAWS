# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

AWS Lambda-based multi-tenant file upload service written in Go. This is a pedagogical demo showcasing:
- Multi-tenant architecture with tenant isolation
- AWS Cognito authentication with JWT tokens
- Tag-based S3 access control using resource/session tag matching
- Modern Lambda deployment using `provided.al2023` runtime with compiled `bootstrap` binary

## Requirements Summary

### Core Architecture
- **AWS Lambda Function:** Single endpoint `/upload` (expandable with Chi router)
- **Multi-tenancy:** Two tenants (`tenant-a`, `tenant-b`) with separate S3 buckets
- **Authentication:** AWS Cognito User Pool producing JWT tokens with tenant claims
- **Storage:** S3 buckets (`store-tenant-a`, `store-tenant-b`) with file format `YYYY/MM/DD/<guid>.json`
- **Security:** Resource tag → session tag matching for S3 access control
- **Deployment:** CloudFormation/SAM with Route53 integration on `stefando.me`

### Security Model
- Cognito User Pool with V2_0 pre-token generation adds `tenant_id` claim to both ID and access tokens
- API Gateway uses Lambda authorizer with **access tokens** (not ID tokens)
- Lambda authorizer validates JWT tokens using OIDC and extracts tenant information
- **Current Implementation:** Basic IAM permissions allow access to all tenant buckets
- **Authorization Chain:** TOKEN type Lambda authorizer → OIDC JWT validation → tenant extraction → S3 access

### Deployment Strategy
- Use `provided.al2023` runtime with compiled Go binary named `bootstrap`
- CloudFormation template includes: Lambda, API Gateway, Cognito, S3 buckets, IAM roles
- Single command deployment/teardown via SAM CLI
- Custom domain integration with existing Route53 hosted zone

## Development Commands

### Task Automation (using Taskfile.dev)
- Deploy stack: `task deploy`
- Build Lambda: `task build`
- Test locally: `task test`
- Clean artifacts: `task clean`
- Delete stack: `task delete`

### AWS Operations
- Deploy with SAM: `sam deploy --profile personal`
- Build locally: `sam build --profile personal`
- Local testing: `sam local start-api --profile personal`

### Go Development
- Build Lambda binary: `GOOS=linux GOARCH=amd64 go build -o bootstrap main.go`
- Run tests: `go test ./...`
- Format code: `go fmt ./...`
- Static analysis: `go vet ./...`
- Tidy modules: `go mod tidy`

## Architecture Details

### Multi-tenant Flow
1. User authenticates with Cognito → receives ID token with `tenant_id` claim (via V2_0 pre-token generation)
2. API request includes ID token as Bearer token 
3. API Gateway Cognito User Pool authorizer validates ID token
4. Lambda extracts tenant information from API Gateway request context claims
5. **Current:** Lambda uses tenant-specific bucket based on extracted tenant_id
6. **TODO:** Implement session tag-based S3 access control for security isolation

### File Storage Pattern
- Path: `s3://store-{tenant-id}/YYYY/MM/DD/{guid}.json`
- Each tenant has isolated storage with date-based organization
- GUID ensures unique filenames within daily folders

### Security Layers
1. **API Gateway:** Cognito User Pool authorizer validates ID tokens 
2. **Lambda Function:** Extracts tenant claims from API Gateway request context
3. **Current IAM:** Basic permissions allow access to all tenant buckets
4. **TODO:** Implement tag-based IAM policies and S3 bucket policies for tenant isolation

## Cognito Configuration
- **User Pool:** Manages user accounts and authentication with V2_0 pre-token generation
- **User Pool Client:** Handles JWT token generation and validation
- **Custom Claims:** Tenant ID embedded in both ID and access token payloads via pre-token Lambda
- **Token Usage:** API Gateway authorizer uses **ID tokens** (not access tokens)
- **Hardcoded Users:** `user-tenant-a` and `user-tenant-b` for demo purposes

## AWS Resources Created
- Lambda Function with execution role
- API Gateway with custom domain
- Cognito User Pool and Client
- S3 buckets with tag-based policies
- IAM roles and policies for tag-based access
- Route53 record for API endpoint (using existing hosted zone)

## Development Notes
- Target audience: Senior engineers new to Go/AWS
- Comments focus on AWS concepts and Go Lambda patterns
- Modern idiomatic Go with proper error handling
- Chi router for HTTP routing (future expansion)

## Demo and Testing Instructions

### Deployment Process
1. Build the Lambda functions:
   ```bash
   task build
   ```

2. Package the application for deployment:
   ```bash
   task sam-package
   ```

3. Deploy the CloudFormation stack:
   ```bash
   task deploy
   ```

### Setting Up Test Users

After deployment, you need to create and configure Cognito users for testing:

1. Create user for tenant-a:
   ```bash
   aws cognito-idp admin-create-user \
     --user-pool-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolId'].OutputValue" --output text) \
     --username user-tenant-a \
     --message-action SUPPRESS \
     --temporary-password TempPass123! \
     --profile personal \
     --region eu-central-1
   ```

2. Set permanent password for tenant-a user:
   ```bash
   aws cognito-idp admin-set-user-password \
     --user-pool-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolId'].OutputValue" --output text) \
     --username user-tenant-a \
     --password TestPass123! \
     --permanent \
     --profile personal \
     --region eu-central-1
   ```

3. Create user for tenant-b:
   ```bash
   aws cognito-idp admin-create-user \
     --user-pool-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolId'].OutputValue" --output text) \
     --username user-tenant-b \
     --message-action SUPPRESS \
     --temporary-password TempPass123! \
     --profile personal \
     --region eu-central-1
   ```

4. Set permanent password for tenant-b user:
   ```bash
   aws cognito-idp admin-set-user-password \
     --user-pool-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolId'].OutputValue" --output text) \
     --username user-tenant-b \
     --password TestPass123! \
     --permanent \
     --profile personal \
     --region eu-central-1
   ```

### Authentication and API Testing

**IMPORTANT:** Use the direct API Gateway endpoint, not the custom domain, for testing authorization.

#### Step 1: Get Access Tokens

1. Get access token for tenant-a:
   ```bash
   aws cognito-idp initiate-auth \
     --auth-flow USER_PASSWORD_AUTH \
     --client-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolClientId'].OutputValue" --output text) \
     --auth-parameters USERNAME=user-tenant-a,PASSWORD=TestPass123! \
     --profile personal \
     --region eu-central-1 \
     --query "AuthenticationResult.AccessToken" --output text
   ```

2. Get access token for tenant-b:
   ```bash
   aws cognito-idp initiate-auth \
     --auth-flow USER_PASSWORD_AUTH \
     --client-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolClientId'].OutputValue" --output text) \
     --auth-parameters USERNAME=user-tenant-b,PASSWORD=TestPass123! \
     --profile personal \
     --region eu-central-1 \
     --query "AuthenticationResult.AccessToken" --output text
   ```

#### Step 2: Get API Gateway Direct Endpoint

Get the API Gateway ID and construct the direct endpoint:
```bash
# Get API Gateway ID
API_ID=$(aws apigateway get-rest-apis --profile personal --region eu-central-1 --query "items[?name=='upload-demo-stack-api'].id" --output text)

# Direct endpoint format: https://{api-id}.execute-api.{region}.amazonaws.com/{stage}
echo "Direct API endpoint: https://${API_ID}.execute-api.eu-central-1.amazonaws.com/prod"
```

#### Step 3: Test Upload with Correct curl Switches

Use verbose curl without progress meter:

1. Upload as tenant-a:
   ```bash
   curl -v -s -X POST \
     https://${API_ID}.execute-api.eu-central-1.amazonaws.com/prod/upload \
     -H "Authorization: Bearer [ACCESS_TOKEN_A]" \
     -H "Content-Type: application/json" \
     -d '{"test": "data from tenant-a"}'
   ```

2. Upload as tenant-b:
   ```bash
   curl -v -s -X POST \
     https://${API_ID}.execute-api.eu-central-1.amazonaws.com/prod/upload \
     -H "Authorization: Bearer [ACCESS_TOKEN_B]" \
     -H "Content-Type: application/json" \
     -d '{"test": "data from tenant-b"}'
   ```

**curl switches explained:**
- `-v`: Verbose output (shows headers and connection details)
- `-s`: Silent mode (no progress meter or timing information)
- `-X POST`: HTTP method (can be omitted as POST is inferred from data)

#### Expected Success Response:
```json
{"file_path":"2025/05/22/[guid].json","status":"success","tenant_id":"tenant-a"}
```

### Verifying Tenant Isolation

1. List files in tenant-a bucket:
   ```bash
   aws s3 ls s3://$(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='TenantABucket'].OutputValue" --output text)/$(date +"%Y/%m/%d/") --recursive --profile personal --region eu-central-1
   ```

2. List files in tenant-b bucket:
   ```bash
   aws s3 ls s3://$(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='TenantBBucket'].OutputValue" --output text)/$(date +"%Y/%m/%d/") --recursive --profile personal --region eu-central-1
   ```

### Cleanup

To delete the entire stack and resources:
```bash
aws cloudformation delete-stack --stack-name upload-demo-stack --profile personal --region eu-central-1
```

## Memory Notes
- **Authorization Chain Working:** TOKEN type Lambda authorizer validates access tokens using OIDC library
- **Custom Domain Issue:** Custom domain (upload-api.stefando.me) has authorization issues - use direct API Gateway endpoint
- **curl Commands:** Always use `curl -v -s` (verbose without progress meter) for debugging authorization
- **Access Tokens:** Use AccessToken (not IdToken) with Lambda authorizer for proper tenant_id extraction
- **Bearer Prefix:** Lambda authorizer properly strips "Bearer " prefix from authorization header (case insensitive)
- **Direct API Endpoint:** Use `https://{api-id}.execute-api.{region}.amazonaws.com/{stage}` format for testing