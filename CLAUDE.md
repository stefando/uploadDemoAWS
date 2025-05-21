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
- JWT bearer tokens from Cognito contain `tenant_id` claim
- Lambda execution role uses session tags based on JWT claims
- S3 bucket policies enforce `aws:PrincipalTag/TenantId` conditions
- Each tenant can only access their designated S3 bucket

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
1. User authenticates with Cognito → receives JWT with `tenant_id` claim
2. API request includes JWT as Bearer token
3. Lambda validates JWT and extracts tenant information
4. Session tags are applied based on tenant claim
5. S3 access is controlled via bucket policies matching session tags

### File Storage Pattern
- Path: `s3://store-{tenant-id}/YYYY/MM/DD/{guid}.json`
- Each tenant has isolated storage with date-based organization
- GUID ensures unique filenames within daily folders

### Security Layers
1. **API Gateway:** JWT validation at gateway level
2. **Lambda Function:** Additional JWT parsing and tenant extraction
3. **IAM Policies:** Tag-based conditions for S3 access
4. **S3 Bucket Policies:** Resource-level protection with principal tag matching

## Cognito Configuration
- **User Pool:** Manages user accounts and authentication
- **User Pool Client:** Handles JWT token generation and validation
- **Custom Claims:** Tenant ID embedded in JWT payload
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