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
  - **Pre-token Lambda** (`lambda/pre-token`): Single shared Lambda that adds tenant claims to Cognito tokens
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
- **Pre-token Generation:** Single shared Lambda with V2_0 trigger that adds `tenant_id` claim to tokens
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
- **Tenant Management:**
  - Add tenant: `task tenant-add TENANT_ID=tenant-name`
  - Remove tenant: `task tenant-remove TENANT_ID=tenant-name [DELETE_S3=true]`
  - List tenants: `task tenant-list`
  - Add user: `task user-add TENANT_ID=tenant-name USERNAME=john [EMAIL=john@example.com] [PASSWORD=Pass123!]`

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
8. Lambda authorizer extracts issuer from token and validates against any Cognito User Pool
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
2. **Lambda Authorizer:** Accepts tokens from any Cognito User Pool that contains tenant_id claim
3. **Lambda Function:** Extracts tenant claims from authorizer context
4. **Lambda IAM:** AssumeRole with session tagging for tenant-scoped S3 access
5. **S3 Access:** Presigned URLs generated with tenant-scoped credentials for multipart uploads

### Lambda Authorizer Architecture
- **Token Validation:** Accepts JWT tokens from any Cognito User Pool in the region
- **Issuer Discovery:** Extracts issuer from token payload to determine which User Pool to validate against
- **OIDC Verification:** Uses the OIDC library to fetch public keys and verify token signature
- **Tenant Claim:** Only accepts tokens that contain the `tenant_id` custom claim (added by pre-token Lambda)
- **No Hardcoded Issuers:** Dynamic validation allows adding new tenants without updating authorizer code
- **Context Propagation:** Passes tenant_id, username, and token_expiration to downstream Lambda

## Cognito Configuration
- **User Pools:** Created per tenant using task commands (not in CloudFormation)
  - Naming convention: `{stack-name}-{tenant-id}-user-pool`
  - Each pool configured with password policy and pre-token Lambda trigger
- **User Pool Clients:** Each pool has its own client for JWT token generation
  - Configured with USER_PASSWORD_AUTH flow
  - No client secret (public client)
- **Pre-token Lambda:** Single shared Lambda for all tenants
  - Triggered by Cognito V2_0 hooks
  - Looks up tenant_id in DynamoDB using pool_id
  - Adds `tenant_id` claim to both ID and access tokens
- **DynamoDB Table:** Maps User Pool IDs to tenant IDs
  - Table name: `{stack-name}-pool-tenant-mapping`
  - Primary key: `pool_id` (User Pool ID)
  - Attribute: `tenant_id` (Tenant identifier)
- **Token Usage:** Lambda authorizer validates **access tokens** via Authorization header
- **User Discovery:** Login endpoint uses tenant parameter to find correct User Pool by naming convention

## AWS Resources Created
- **Lambda Functions:**
  - Upload Lambda with execution role
  - Login Lambda with Cognito permissions
  - Authorizer Lambda for JWT validation
  - Single Pre-token Lambda (shared by all tenants)
- **API Gateway:** REST API with custom domain and REQUEST authorizer
- **Cognito Resources:**
  - Single Pre-token Lambda (shared by all User Pools)
  - User Pools created per tenant via task commands (not in CloudFormation)
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

### Environment Setup

Set these environment variables to simplify commands throughout the demo:

```bash
# Core configuration
export AWS_PROFILE=personal
export AWS_REGION=eu-central-1
export STACK_NAME=upload-demo-stack

# After deployment, get these values from CloudFormation outputs
export USER_POOL_A=$(aws cloudformation describe-stacks --stack-name $STACK_NAME --profile $AWS_PROFILE --region $AWS_REGION --query "Stacks[0].Outputs[?OutputKey=='UserPoolIdTenantA'].OutputValue" --output text)
export USER_POOL_B=$(aws cloudformation describe-stacks --stack-name $STACK_NAME --profile $AWS_PROFILE --region $AWS_REGION --query "Stacks[0].Outputs[?OutputKey=='UserPoolIdTenantB'].OutputValue" --output text)
export CLIENT_ID_A=$(aws cloudformation describe-stacks --stack-name $STACK_NAME --profile $AWS_PROFILE --region $AWS_REGION --query "Stacks[0].Outputs[?OutputKey=='UserPoolClientIdTenantA'].OutputValue" --output text)
export CLIENT_ID_B=$(aws cloudformation describe-stacks --stack-name $STACK_NAME --profile $AWS_PROFILE --region $AWS_REGION --query "Stacks[0].Outputs[?OutputKey=='UserPoolClientIdTenantB'].OutputValue" --output text)
export S3_BUCKET=$(aws cloudformation describe-stacks --stack-name $STACK_NAME --profile $AWS_PROFILE --region $AWS_REGION --query "Stacks[0].Outputs[?OutputKey=='SharedStorageBucket'].OutputValue" --output text)
export API_URL=$(aws cloudformation describe-stacks --stack-name $STACK_NAME --profile $AWS_PROFILE --region $AWS_REGION --query "Stacks[0].Outputs[?OutputKey=='ApiUrl'].OutputValue" --output text)

# API Gateway direct endpoint (optional)
export API_ID=$(aws apigateway get-rest-apis --profile $AWS_PROFILE --region $AWS_REGION --query "items[?name=='${STACK_NAME}-api'].id" --output text)
export API_ENDPOINT="https://${API_ID}.execute-api.${AWS_REGION}.amazonaws.com/prod"

# Common test password
export TEST_PASSWORD="TestPass123!"
```

You can save these in a file and source it:
```bash
# Save to .env file (add to .gitignore!)
cat > .env.demo <<'EOF'
export AWS_PROFILE=personal
export AWS_REGION=eu-central-1
export STACK_NAME=upload-demo-stack
export TEST_PASSWORD="TestPass123!"
EOF

# Source before running commands
source .env.demo

# After deployment, update with outputs
./scripts/update-env.sh  # Script to fetch and update CloudFormation outputs
```

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

### Tenant Management

After deployment, use these commands to manage tenants:

#### Adding a New Tenant

```bash
# Add a new tenant (creates User Pool, Client, and DynamoDB mapping)
task tenant-add TENANT_ID=tenant-c

# Add users to the new tenant
task user-add TENANT_ID=tenant-c USERNAME=alice
task user-add TENANT_ID=tenant-c USERNAME=bob EMAIL=bob@custom.com PASSWORD=MyPass123!
```

#### Managing Tenants

```bash
# List all configured tenants
task tenant-list

# Remove a tenant (keeps S3 data by default)
task tenant-remove TENANT_ID=tenant-c

# Remove a tenant and delete all S3 data
task tenant-remove TENANT_ID=tenant-c DELETE_S3=true
```

#### Initial Demo Setup

For the initial demo, create two tenants:

```bash
# Create tenant-a
task tenant-add TENANT_ID=tenant-a
task user-add TENANT_ID=tenant-a USERNAME=tom EMAIL=tom@tenant-a.com
task user-add TENANT_ID=tenant-a USERNAME=jerry EMAIL=jerry@tenant-a.com

# Create tenant-b
task tenant-add TENANT_ID=tenant-b
task user-add TENANT_ID=tenant-b USERNAME=sylvester EMAIL=sylvester@tenant-b.com
task user-add TENANT_ID=tenant-b USERNAME=tweety EMAIL=tweety@tenant-b.com
```

### Setting Up Test Users (Manual Method)

After deployment and setting up environment variables, create users in each tenant's Cognito User Pool:

#### Tenant A Users

1. Create john in tenant-a:
   ```bash
   aws cognito-idp admin-create-user \
     --user-pool-id $USER_POOL_A \
     --username john \
     --user-attributes Name=email,Value=john@tenant-a.com \
     --message-action SUPPRESS \
     --temporary-password TempPass123! \
     --profile $AWS_PROFILE \
     --region $AWS_REGION
   ```

2. Set permanent password for john:
   ```bash
   aws cognito-idp admin-set-user-password \
     --user-pool-id $USER_POOL_A \
     --username john \
     --password $TEST_PASSWORD \
     --permanent \
     --profile $AWS_PROFILE \
     --region $AWS_REGION
   ```

3. Create mary in tenant-a:
   ```bash
   aws cognito-idp admin-create-user \
     --user-pool-id $USER_POOL_A \
     --username mary \
     --user-attributes Name=email,Value=mary@tenant-a.com \
     --message-action SUPPRESS \
     --temporary-password TempPass123! \
     --profile $AWS_PROFILE \
     --region $AWS_REGION
   ```

4. Set permanent password for mary:
   ```bash
   aws cognito-idp admin-set-user-password \
     --user-pool-id $USER_POOL_A \
     --username mary \
     --password $TEST_PASSWORD \
     --permanent \
     --profile $AWS_PROFILE \
     --region $AWS_REGION
   ```

#### Tenant B Users

1. Create bob in tenant-b:
   ```bash
   aws cognito-idp admin-create-user \
     --user-pool-id $USER_POOL_B \
     --username bob \
     --user-attributes Name=email,Value=bob@tenant-b.com \
     --message-action SUPPRESS \
     --temporary-password TempPass123! \
     --profile $AWS_PROFILE \
     --region $AWS_REGION
   ```

2. Set permanent password for bob:
   ```bash
   aws cognito-idp admin-set-user-password \
     --user-pool-id $USER_POOL_B \
     --username bob \
     --password $TEST_PASSWORD \
     --permanent \
     --profile $AWS_PROFILE \
     --region $AWS_REGION
   ```

3. Create alice in tenant-b:
   ```bash
   aws cognito-idp admin-create-user \
     --user-pool-id $USER_POOL_B \
     --username alice \
     --user-attributes Name=email,Value=alice@tenant-b.com \
     --message-action SUPPRESS \
     --temporary-password TempPass123! \
     --profile $AWS_PROFILE \
     --region $AWS_REGION
   ```

4. Set permanent password for alice:
   ```bash
   aws cognito-idp admin-set-user-password \
     --user-pool-id $USER_POOL_B \
     --username alice \
     --password $TEST_PASSWORD \
     --permanent \
     --profile $AWS_PROFILE \
     --region $AWS_REGION
   ```

### Authentication and API Testing

Both the custom domain (upload-api.stefando.me) and direct API Gateway endpoint can be used for testing.

#### Step 1: Get Access Tokens

**Using Multi-tenant Login API:**

1. Login as tom from tenant-a:
   ```bash
   # Using custom domain
   curl -X POST https://upload-api.stefando.me/login \
     -H "Content-Type: application/json" \
     -d '{"tenant": "tenant-a", "username": "tom", "password": "TestPass123!"}' \
     | jq -r '.access_token'
   
   # Or using direct API endpoint
   curl -X POST https://${API_ID}.execute-api.eu-central-1.amazonaws.com/prod/login \
     -H "Content-Type: application/json" \
     -d '{"tenant": "tenant-a", "username": "tom", "password": "TestPass123!"}' \
     | jq -r '.access_token'
   ```

2. Login as sylvester from tenant-b:
   ```bash
   # Using custom domain
   curl -X POST https://upload-api.stefando.me/login \
     -H "Content-Type: application/json" \
     -d '{"tenant": "tenant-b", "username": "sylvester", "password": "TestPass123!"}' \
     | jq -r '.access_token'
   ```

3. Test invalid tenant (should fail):
   ```bash
   curl -X POST https://upload-api.stefando.me/login \
     -H "Content-Type: application/json" \
     -d '{"tenant": "invalid-tenant", "username": "tom", "password": "TestPass123!"}'
   ```

**Alternative Method - Using AWS CLI (direct Cognito access):**

1. Get access token for john in tenant-a:
   ```bash
   aws cognito-idp initiate-auth \
     --auth-flow USER_PASSWORD_AUTH \
     --client-id $CLIENT_ID_A \
     --auth-parameters USERNAME=john,PASSWORD=$TEST_PASSWORD \
     --profile $AWS_PROFILE \
     --region $AWS_REGION \
     --query "AuthenticationResult.AccessToken" --output text
   ```

2. Get access token for bob in tenant-b:
   ```bash
   aws cognito-idp initiate-auth \
     --auth-flow USER_PASSWORD_AUTH \
     --client-id $CLIENT_ID_B \
     --auth-parameters USERNAME=bob,PASSWORD=$TEST_PASSWORD \
     --profile $AWS_PROFILE \
     --region $AWS_REGION \
     --query "AuthenticationResult.AccessToken" --output text
   ```

#### Step 2: Get API Gateway Direct Endpoint

The API Gateway ID was already set in the environment setup section above. To verify:
```bash
echo "Direct API endpoint: $API_ENDPOINT"
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
   aws s3 ls s3://$S3_BUCKET/ --recursive --profile $AWS_PROFILE --region $AWS_REGION
   ```

2. List files for tenant-a only:
   ```bash
   aws s3 ls s3://$S3_BUCKET/tenant-a/ --recursive --profile $AWS_PROFILE --region $AWS_REGION
   ```

3. List files for tenant-b only:
   ```bash
   aws s3 ls s3://$S3_BUCKET/tenant-b/ --recursive --profile $AWS_PROFILE --region $AWS_REGION
   ```

### Testing with JetBrains HTTP Client

The project includes HTTP test files in `test/http/api-tests.http` that can be run using JetBrains IDEs or the CLI tool.

#### Installing the HTTP Client CLI

```bash
# Install via Homebrew
brew install ijhttp

# Install Java 17 (required by ijhttp - newer versions may cause compatibility issues)
brew install --cask temurin@17
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
### Login as tom from tenant-a
POST {{baseUrl}}/login
Content-Type: application/json

{
  "tenant": "{{tenantA}}",
  "username": "{{userA1}}",
  "password": "{{password}}"
}

> {%
    client.test("Login successful", function() {
        client.assert(response.status === 200, "Response status is not 200");
        client.assert(response.body.access_token, "No access token in response");
    });
    client.global.set("accessTokenATom", response.body.access_token);
%}
```

### AWS Cost Monitoring

Track your AWS spending for this demo:
```bash
# Get current month's total AWS account costs
aws ce get-cost-and-usage \
  --profile $AWS_PROFILE \
  --time-period Start=$(date -v1d +%Y-%m-%d),End=$(date -v+1d +%Y-%m-%d) \
  --granularity=MONTHLY \
  --metrics "UnblendedCost"

# Activate CloudFormation cost allocation tags (one-time setup)
aws ce update-cost-allocation-tags-status \
  --profile $AWS_PROFILE \
  --cost-allocation-tags-status TagKey=aws:cloudformation:stack-name,Status=Active

# Get costs for this specific CloudFormation stack
aws ce get-cost-and-usage \
  --profile $AWS_PROFILE \
  --time-period Start=$(date -v1d +%Y-%m-%d),End=$(date -v+1d +%Y-%m-%d) \
  --granularity=MONTHLY \
  --metrics "UnblendedCost" \
  --filter '{
    "Tags": {
      "Key": "aws:cloudformation:stack-name",
      "Values": ["'$STACK_NAME'"]
    }
  }'
```

**Note:** Stack-specific cost tracking:
- Tag activation takes up to 24 hours to take effect
- Costs only appear after resources incur charges (not retroactive)
- Once activated, the tag remains active for all stacks in your account

### Cleanup

To delete the entire stack and resources:
```bash
aws cloudformation delete-stack --stack-name $STACK_NAME --profile $AWS_PROFILE --region $AWS_REGION
```

Or using the task command:
```bash
task delete
```

## Memory Notes
- **Authorization Chain:** REQUEST type Lambda authorizer validates access tokens using OIDC library
- **Custom Domain:** REGIONAL endpoint configuration enables custom domain support (upload-api.stefando.me)
- **curl Commands:** Always use `curl -v -s` (verbose without progress meter) for debugging authorization
- **Access Tokens:** Use AccessToken (not IdToken) sent via Authorization header with Bearer prefix
- **Header Processing:** Lambda authorizer extracts token from Authorization header with Bearer prefix
- **Direct API Endpoint:** Use `https://{api-id}.execute-api.{region}.amazonaws.com/{stage}` format for testing