# Multi-Tenant File Upload Service

AWS Lambda-based multi-tenant file upload service written in Go. This demo showcases modern serverless architecture with tenant isolation, JWT authentication, and session-tagged S3 access control.

## Quick Start

```bash
# Deploy the stack
task deploy

# Add a tenant and users
task tenant-add TENANT_ID=demo-tenant
task user-add TENANT_ID=demo-tenant USERNAME=alice
task user-add TENANT_ID=demo-tenant USERNAME=bob

# Test the API
curl -X POST https://upload-api.stefando.me/login \
  -H "Content-Type: application/json" \
  -d '{"tenant": "demo-tenant", "username": "alice", "password": "TestPass123!"}'
```

## Architecture Overview

### Lambda Functions
- **Upload API** (`lambdas/api/upload`) - File operations with multipart upload support
- **Login API** (`lambdas/api/login`) - Multi-tenant authentication
- **JWT Authorizer** (`lambdas/cognito/authorizer`) - Token validation for protected endpoints
- **Pre-token Hook** (`lambdas/cognito/pre-token`) - Adds tenant claims to Cognito tokens

### Multi-Tenancy Model
- **Separate Cognito User Pools** per tenant (naming convention: `{stack}-{tenant}-user-pool`)
- **Shared S3 bucket** with tenant-prefixed paths (`s3://bucket/{tenant-id}/...`)
- **Session tag-based isolation** via AssumeRole with tenant tags
- **JWT tokens** contain `tenant_id` claim for authorization

### Security Flow
1. Login with tenant parameter → discovers User Pool by naming convention
2. Cognito authenticates → Pre-token Lambda adds `tenant_id` claim
3. API requests include access token → JWT Authorizer validates multi-issuer tokens
4. Lambda assumes role with tenant session tags → S3 access scoped to tenant prefix

## Development Setup

### Prerequisites
- Go 1.24+ with workspaces support
- AWS CLI configured with `personal` profile
- SAM CLI for deployment
- [Task](https://taskfile.dev) for automation

### Go Workspaces
Each Lambda function is an independent Go module:
```
lambdas/
├── api/
│   ├── upload/     # Business logic - file operations
│   └── login/      # Business logic - authentication  
└── cognito/
    ├── authorizer/ # Infrastructure - JWT validation
    └── pre-token/  # Infrastructure - token enrichment
```

Root `go.work` file manages all modules together while maintaining dependency isolation.

## Common Tasks

```bash
# Development
task build          # Build all Lambda functions
task test           # Run tests
task local          # Start local API Gateway
task fmt            # Format and lint code

# Deployment
task deploy         # Deploy stack with git commit tracking
task delete         # Delete entire stack

# Tenant Management
task tenant-add TENANT_ID=my-tenant
task user-add TENANT_ID=my-tenant USERNAME=john
task tenant-list
task tenant-remove TENANT_ID=my-tenant [DELETE_S3=true]
```

## API Endpoints

| Endpoint | Auth | Description |
|----------|------|-------------|
| `POST /login` | None | Authenticate with tenant parameter |
| `POST /upload` | JWT | Direct JSON upload |
| `POST /upload/initiate` | JWT | Start multipart upload |
| `POST /upload/complete` | JWT | Complete multipart upload |
| `POST /upload/abort` | JWT | Cancel multipart upload |
| `POST /upload/refresh` | JWT | Refresh presigned URLs |
| `GET /health` | None | Health check |

## Example: Multipart Upload

```bash
# 1. Login and get token
ACCESS_TOKEN=$(curl -s -X POST https://upload-api.stefando.me/login \
  -H "Content-Type: application/json" \
  -d '{"tenant": "demo-tenant", "username": "alice", "password": "TestPass123!"}' \
  | jq -r '.access_token')

# 2. Initiate multipart upload
UPLOAD_ID=$(curl -s -X POST https://upload-api.stefando.me/upload/initiate \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"file_size": 10485760, "part_size": 5242880}' \
  | jq -r '.upload_id')

# 3. Upload parts directly to S3 using presigned URLs
# 4. Complete upload with ETags
curl -X POST https://upload-api.stefando.me/upload/complete \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"upload_id": "'$UPLOAD_ID'", "parts": [{"part_number": 1, "etag": "..."}]}'
```

## IAM Security Architecture

- **LambdaExecutionRole**: Basic Lambda execution permissions
- **TenantAccessRole**: S3 access role that trusts LambdaExecutionRole
- **LambdaAssumeRolePolicy**: Separate policy granting assume permissions (avoids circular dependencies)
- **Session Tagging**: `tenant_id` tags on assumed role restrict S3 access to tenant prefix

## Configuration

### Environment Variables (Deployment)
- `AWS_PROFILE=personal` - AWS profile for deployment
- `AWS_REGION=eu-central-1` - Target region
- `STACK_NAME=upload-demo-stack` - CloudFormation stack name

### Lambda Environment Variables (Runtime)
- `TENANT_ACCESS_ROLE_ARN` - Role to assume for S3 operations
- `SHARED_BUCKET` - S3 bucket name for file storage
- `STACK_NAME` - Used for User Pool discovery
- `LOG_LEVEL` - Logging verbosity

## Monitoring

```bash
# Check deployment version
aws cloudformation describe-stacks --stack-name upload-demo-stack \
  --query "Stacks[0].Outputs[?OutputKey=='DeployedGitCommit'].OutputValue" --output text

# View Lambda logs
aws logs tail /aws/lambda/upload-demo-stack-upload-function --follow

# Monitor costs
aws ce get-cost-and-usage --time-period Start=2025-05-01,End=2025-06-01 \
  --granularity=MONTHLY --metrics "UnblendedCost"
```

## Testing

Use JetBrains HTTP Client CLI for comprehensive API testing:

```bash
# Install HTTP client
brew install ijhttp

# Run test suite
ijhttp test/http/api-tests.http
```

Test files include multi-tenant authentication, upload workflows, and error cases with built-in assertions.

## Troubleshooting

**Common Issues:**
- **Certificate delays**: SSL certificate validation can take 5-15 minutes on first deployment
- **User Pool not found**: Ensure tenant exists with `task tenant-list`
- **Access denied**: Check JWT token contains `tenant_id` claim
- **Circular dependency**: Fixed in current version using separate IAM policy

**Debug Commands:**
```bash
# Check stack status
aws cloudformation describe-stacks --stack-name upload-demo-stack \
  --query "Stacks[0].StackStatus" --output text

# Verify tenant setup
task tenant-list

# Test JWT token contents
echo $ACCESS_TOKEN | cut -d'.' -f2 | base64 -d | jq
```

## Architecture Benefits

- **True multi-tenancy** with separate User Pools and data isolation
- **Independent Lambda modules** with Go workspaces preventing dependency conflicts
- **Secure IAM architecture** following principle of least privilege
- **Scalable storage** with session-tagged S3 access control
- **Modern deployment** with SAM, git tracking, and infrastructure as code