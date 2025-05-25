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
- **AWS Lambda Functions:** Separate functions for different concerns:
  - **Upload Lambda** (`cmd/lambda`): Handles all file operations (`/upload/*` endpoints)
  - **Login Lambda** (`cmd/login`): Handles authentication (`/login` endpoint)
  - **Authorizer Lambda** (`lambda/authorizer`): Validates JWT tokens for protected endpoints
  - **Pre-token Lambdas** (`lambda/pre-token`): Per-tenant Lambdas that add tenant claims to Cognito tokens
- **API Endpoints:**
  - `/login` - Authenticate with tenant parameter and receive JWT tokens (no auth required)
  - `/upload` - Direct JSON upload (requires auth)
  - `/upload/initiate` - Start multipart upload (requires auth)
  - `/upload/complete` - Complete multipart upload (requires auth)
  - `/upload/abort` - Cancel multipart upload (requires auth)
  - `/upload/refresh` - Refresh presigned URLs (requires auth)
  - `/health` - Health check (no auth required)
- **Multi-tenancy:** Separate Cognito User Pools per tenant with naming convention discovery
- **Authentication:** Multiple Cognito User Pools (one per tenant) producing JWT tokens with tenant claims
- **Storage:** Single shared S3 bucket (`store-shared`) with tenant-prefixed paths
- **Security:** Session tag-based S3 access control via AssumeRole with tenant tags
- **Deployment:** CloudFormation/SAM with Route53 integration on `stefando.me`

### Security Model
- **Multi-tenant Cognito:** Separate User Pools per tenant, discovered by naming convention (`{stack-name}-{tenant-id}-user-pool`)
- **Pre-token Generation:** Each tenant has dedicated Lambda with V2_0 trigger that adds `tenant_id` claim to tokens
- **API Gateway:** REQUEST type Lambda authorizer validates **access tokens** sent via Authorization header
- **Multi-issuer Support:** Lambda authorizer validates tokens against multiple Cognito issuers (one per tenant)
- **Tenant Isolation:** AssumeRole with tenant session tags ensures S3 access is scoped to tenant prefix
- **Authorization Chain:** Login with tenant → User Pool discovery → JWT with tenant claim → Multi-issuer validation → AssumeRole with tags → S3 access
- **Session Duration:** 3 hours for assumed role credentials, 2 hours for presigned URLs

### Deployment Strategy
- Use `provided.al2023` runtime with compiled Go binary named `bootstrap`
- CloudFormation template includes: Lambda, API Gateway, Cognito, S3 bucket, IAM roles
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
1. User authenticates with `/login` endpoint providing tenant, username, and password
2. Login Lambda discovers correct User Pool by naming convention (`{stack-name}-{tenant-id}-user-pool`)
3. Cognito authenticates user and triggers tenant-specific pre-token Lambda
4. Pre-token Lambda adds `tenant_id` claim to both ID and access tokens
5. Client receives JWT tokens with embedded tenant information
6. API request includes access token in Authorization header
7. API Gateway invokes REQUEST type Lambda authorizer for validation
8. Lambda authorizer tries each valid issuer until token validates successfully
9. Authorizer extracts tenant_id claim and passes it in request context
10. Upload Lambda assumes role with tenant session tags for S3 access
11. Lambda stores files in shared bucket with tenant-prefixed paths using assumed credentials

### File Storage Pattern
- **Direct upload path:** `s3://store-shared/{tenant-id}/YYYY/MM/DD/{guid}.json`
- **Multipart upload path:** `s3://store-shared/{tenant-id}/{container-key}/{guid}`
- Single bucket with tenant-prefixed paths for isolation
- GUID ensures unique filenames

### Multipart Upload Flow
1. **Initiate:** Client calls `/upload/initiate` with file size and part size
2. **Upload Parts:** Client uploads directly to S3 using presigned URLs (2-hour validity)
3. **Complete:** Client calls `/upload/complete` with ETags from S3
4. **Refresh:** Client can call `/upload/refresh` to get new presigned URLs if needed
5. **Abort:** Client can call `/upload/abort` to cancel an in-progress upload

### Security Layers
1. **API Gateway:** REQUEST type Lambda authorizer validates access tokens from Authorization header
2. **Lambda Function:** Extracts tenant claims from authorizer context
3. **Lambda IAM:** AssumeRole with session tagging for tenant-scoped S3 access
4. **S3 Access:** Presigned URLs generated with tenant-scoped credentials for multipart uploads

## Cognito Configuration
- **User Pools:** Separate pools per tenant following naming convention:
  - `{stack-name}-tenant-a-user-pool` for tenant A
  - `{stack-name}-tenant-b-user-pool` for tenant B
- **User Pool Clients:** Each pool has its own client for JWT token generation
- **Pre-token Lambdas:** Tenant-specific Lambdas triggered by V2_0 hooks:
  - Each Lambda has `TENANT_ID` environment variable set
  - Adds `tenant_id` claim to both ID and access tokens
- **Token Usage:** Lambda authorizer validates **access tokens** via Authorization header
- **User Discovery:** Login endpoint uses tenant parameter to find correct User Pool

## AWS Resources Created
- **Lambda Functions:**
  - Upload Lambda with execution role
  - Login Lambda with Cognito permissions
  - Authorizer Lambda for JWT validation
  - Pre-token Lambdas (one per tenant)
- **API Gateway:** REST API with custom domain and REQUEST authorizer
- **Cognito Resources:**
  - User Pool for tenant-a with client
  - User Pool for tenant-b with client
  - Pre-token Lambda triggers for each pool
- **Storage:** Single S3 bucket with tenant-prefixed paths
- **IAM:** Roles and policies for tag-based tenant isolation
- **DNS:** Route53 record for custom domain (using existing hosted zone)

## Multi-tenant Architecture Benefits

### Separate User Pools Per Tenant
- **Complete Isolation:** Each tenant has its own authentication boundary
- **Independent Configuration:** Password policies, MFA settings, and user attributes can vary per tenant
- **Separate User Namespaces:** Users can have the same username across different tenants
- **Compliance:** Easier to meet data residency and compliance requirements per tenant
- **Scalability:** User pools can be managed, scaled, and migrated independently

### Naming Convention Discovery
- **No Additional Infrastructure:** No need for DynamoDB tables or configuration services
- **Predictable:** User pool location follows `{stack-name}-{tenant-id}-user-pool` pattern
- **Easy Testing:** Clear which resources belong to which tenant
- **CloudFormation Friendly:** Resources can be created and managed declaratively

### Security Considerations
- **Multi-issuer Validation:** Authorizer validates tokens from any registered tenant's user pool
- **Tenant Claims:** Pre-token Lambda ensures tenant_id is always present in tokens
- **No Cross-tenant Access:** Users authenticated in one tenant cannot access another tenant's resources
- **Session Tagging:** S3 access is further restricted by IAM session tags matching the tenant

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

After deployment, you need to create users in each tenant's Cognito User Pool:

#### Tenant A Users

1. Create john in tenant-a:
   ```bash
   aws cognito-idp admin-create-user \
     --user-pool-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolIdTenantA'].OutputValue" --output text) \
     --username john \
     --user-attributes Name=email,Value=john@tenant-a.com \
     --message-action SUPPRESS \
     --temporary-password TempPass123! \
     --profile personal \
     --region eu-central-1
   ```

2. Set permanent password for john:
   ```bash
   aws cognito-idp admin-set-user-password \
     --user-pool-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolIdTenantA'].OutputValue" --output text) \
     --username john \
     --password TestPass123! \
     --permanent \
     --profile personal \
     --region eu-central-1
   ```

3. Create mary in tenant-a:
   ```bash
   aws cognito-idp admin-create-user \
     --user-pool-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolIdTenantA'].OutputValue" --output text) \
     --username mary \
     --user-attributes Name=email,Value=mary@tenant-a.com \
     --message-action SUPPRESS \
     --temporary-password TempPass123! \
     --profile personal \
     --region eu-central-1
   ```

4. Set permanent password for mary:
   ```bash
   aws cognito-idp admin-set-user-password \
     --user-pool-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolIdTenantA'].OutputValue" --output text) \
     --username mary \
     --password TestPass123! \
     --permanent \
     --profile personal \
     --region eu-central-1
   ```

#### Tenant B Users

5. Create bob in tenant-b:
   ```bash
   aws cognito-idp admin-create-user \
     --user-pool-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolIdTenantB'].OutputValue" --output text) \
     --username bob \
     --user-attributes Name=email,Value=bob@tenant-b.com \
     --message-action SUPPRESS \
     --temporary-password TempPass123! \
     --profile personal \
     --region eu-central-1
   ```

6. Set permanent password for bob:
   ```bash
   aws cognito-idp admin-set-user-password \
     --user-pool-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolIdTenantB'].OutputValue" --output text) \
     --username bob \
     --password TestPass123! \
     --permanent \
     --profile personal \
     --region eu-central-1
   ```

7. Create alice in tenant-b:
   ```bash
   aws cognito-idp admin-create-user \
     --user-pool-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolIdTenantB'].OutputValue" --output text) \
     --username alice \
     --user-attributes Name=email,Value=alice@tenant-b.com \
     --message-action SUPPRESS \
     --temporary-password TempPass123! \
     --profile personal \
     --region eu-central-1
   ```

8. Set permanent password for alice:
   ```bash
   aws cognito-idp admin-set-user-password \
     --user-pool-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolIdTenantB'].OutputValue" --output text) \
     --username alice \
     --password TestPass123! \
     --permanent \
     --profile personal \
     --region eu-central-1
   ```

### Authentication and API Testing

Both the custom domain (upload-api.stefando.me) and direct API Gateway endpoint can be used for testing.

#### Step 1: Get Access Tokens

**Using Multi-tenant Login API:**

1. Login as john from tenant-a:
   ```bash
   # Using custom domain
   curl -X POST https://upload-api.stefando.me/login \
     -H "Content-Type: application/json" \
     -d '{"tenant": "tenant-a", "username": "john", "password": "TestPass123!"}' \
     | jq -r '.access_token'
   
   # Or using direct API endpoint
   curl -X POST https://${API_ID}.execute-api.eu-central-1.amazonaws.com/prod/login \
     -H "Content-Type: application/json" \
     -d '{"tenant": "tenant-a", "username": "john", "password": "TestPass123!"}' \
     | jq -r '.access_token'
   ```

2. Login as bob from tenant-b:
   ```bash
   # Using custom domain
   curl -X POST https://upload-api.stefando.me/login \
     -H "Content-Type: application/json" \
     -d '{"tenant": "tenant-b", "username": "bob", "password": "TestPass123!"}' \
     | jq -r '.access_token'
   ```

3. Test invalid tenant (should fail):
   ```bash
   curl -X POST https://upload-api.stefando.me/login \
     -H "Content-Type: application/json" \
     -d '{"tenant": "invalid-tenant", "username": "john", "password": "TestPass123!"}'
   ```

**Alternative Method - Using AWS CLI (direct Cognito access):**

1. Get access token for john in tenant-a:
   ```bash
   aws cognito-idp initiate-auth \
     --auth-flow USER_PASSWORD_AUTH \
     --client-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolClientIdTenantA'].OutputValue" --output text) \
     --auth-parameters USERNAME=john,PASSWORD=TestPass123! \
     --profile personal \
     --region eu-central-1 \
     --query "AuthenticationResult.AccessToken" --output text
   ```

2. Get access token for bob in tenant-b:
   ```bash
   aws cognito-idp initiate-auth \
     --auth-flow USER_PASSWORD_AUTH \
     --client-id $(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='UserPoolClientIdTenantB'].OutputValue" --output text) \
     --auth-parameters USERNAME=bob,PASSWORD=TestPass123! \
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

Use verbose curl without progress meter. You can use either the direct API Gateway endpoint or the custom domain:

1. Upload as tenant-a (using direct endpoint):
   ```bash
   curl -v -s -X POST \
     https://${API_ID}.execute-api.eu-central-1.amazonaws.com/prod/upload \
     -H "Authorization: Bearer [ACCESS_TOKEN_A]" \
     -H "Content-Type: application/json" \
     -d '{"test": "data from tenant-a"}'
   ```

   Or using custom domain:
   ```bash
   curl -v -s -X POST \
     https://upload-api.stefando.me/upload \
     -H "Authorization: Bearer [ACCESS_TOKEN_A]" \
     -H "Content-Type: application/json" \
     -d '{"test": "data from tenant-a"}'
   ```

2. Upload as tenant-b (using direct endpoint):
   ```bash
   curl -v -s -X POST \
     https://${API_ID}.execute-api.eu-central-1.amazonaws.com/prod/upload \
     -H "Authorization: Bearer [ACCESS_TOKEN_B]" \
     -H "Content-Type: application/json" \
     -d '{"test": "data from tenant-b"}'
   ```

   Or using custom domain:
   ```bash
   curl -v -s -X POST \
     https://upload-api.stefando.me/upload \
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
{"file_path":"tenant-a/2025/05/22/[guid].json","status":"success","tenant_id":"tenant-a"}
```

### Verifying Tenant Isolation

1. List all files in the shared bucket (shows tenant prefixes):
   ```bash
   aws s3 ls s3://$(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='SharedStorageBucket'].OutputValue" --output text)/ --recursive --profile personal --region eu-central-1
   ```

2. List files for tenant-a only:
   ```bash
   aws s3 ls s3://$(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='SharedStorageBucket'].OutputValue" --output text)/tenant-a/ --recursive --profile personal --region eu-central-1
   ```

3. List files for tenant-b only:
   ```bash
   aws s3 ls s3://$(aws cloudformation describe-stacks --stack-name upload-demo-stack --profile personal --region eu-central-1 --query "Stacks[0].Outputs[?OutputKey=='SharedStorageBucket'].OutputValue" --output text)/tenant-b/ --recursive --profile personal --region eu-central-1
   ```

### Testing with JetBrains HTTP Client

The project includes HTTP test files in `test/http/api-tests.http` that can be run using JetBrains IDEs or the CLI tool.

#### Installing the HTTP Client CLI

```bash
# Install via Homebrew
brew install ijhttp

# Install Java 17+ if needed (required by ijhttp)
brew install --cask temurin
```

#### Running Tests

1. **Run all tests in the file:**
   ```bash
   cd /path/to/project
   ijhttp test/http/api-tests.http
   ```

2. **Run with verbose logging:**
   ```bash
   ijhttp -L VERBOSE test/http/api-tests.http
   ```

3. **Run with environment variables:**
   ```bash
   # Create environment file (http-client.env.json)
   {
     "dev": {
       "baseUrl": "https://upload-api.stefando.me",
       "password": "TestPass123!"
     }
   }
   
   # Run with environment
   ijhttp --env-file http-client.env.json --env dev test/http/api-tests.http
   ```

4. **Generate test report:**
   ```bash
   ijhttp --report test/http/api-tests.http
   ```

#### Test File Structure

The `api-tests.http` file contains:
- Multi-tenant login tests for different users and tenants
- Upload tests demonstrating tenant isolation
- Multipart upload workflow tests
- Error cases (invalid tenant, missing auth)

Each test includes assertions to validate responses:
```http
### Login as john from tenant-a
POST {{baseUrl}}/login
Content-Type: application/json

{
  "tenant": "tenant-a",
  "username": "john",
  "password": "TestPass123!"
}

> {%
    client.test("Login successful", function() {
        client.assert(response.status === 200, "Response status is not 200");
        client.assert(response.body.access_token, "No access token in response");
    });
    client.global.set("accessTokenAJohn", response.body.access_token);
%}
```

### Cleanup

To delete the entire stack and resources:
```bash
aws cloudformation delete-stack --stack-name upload-demo-stack --profile personal --region eu-central-1
```

## Memory Notes
- **Authorization Chain:** REQUEST type Lambda authorizer validates access tokens using OIDC library
- **Custom Domain:** REGIONAL endpoint configuration enables custom domain support (upload-api.stefando.me)
- **curl Commands:** Always use `curl -v -s` (verbose without progress meter) for debugging authorization
- **Access Tokens:** Use AccessToken (not IdToken) sent via Authorization header with Bearer prefix
- **Header Processing:** Lambda authorizer extracts token from Authorization header with Bearer prefix
- **Direct API Endpoint:** Use `https://{api-id}.execute-api.{region}.amazonaws.com/{stage}` format for testing