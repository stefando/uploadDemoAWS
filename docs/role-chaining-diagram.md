# Role Chaining vs Direct Assume Role

## Our Architecture (No Role Chaining) ✅

```mermaid
graph TD
    A[Lambda Function] -->|Has Execution Role| B[LambdaExecutionRole]
    B -->|AssumeRole with 3hr duration| C[TenantAccessRole]
    C -->|Access| D[S3 Bucket]
    
    style A fill:#90EE90
    style B fill:#87CEEB
    style C fill:#FFD700
    style D fill:#FFA500
```

**Result:** ✅ Can use 3-hour sessions

## Colleague's Possible Architecture (Role Chaining) ❌

### Scenario 1: Local Development
```mermaid
graph TD
    A[Developer] -->|AssumeRole| B[DeveloperRole]
    B -->|AssumeRole - CHAINING!| C[TenantAccessRole]
    C -->|Access| D[S3 Bucket]
    
    style A fill:#FFB6C1
    style B fill:#FF6347
    style C fill:#FF0000
    style D fill:#FFA500
```

**Result:** ❌ Limited to 1-hour sessions

### Scenario 2: Cross-Account
```mermaid
graph TD
    A[Lambda in Account A] -->|AssumeRole| B[CrossAccountRole in Account B]
    B -->|AssumeRole - CHAINING!| C[TenantAccessRole in Account B]
    C -->|Access| D[S3 Bucket]
    
    style A fill:#FFB6C1
    style B fill:#FF6347
    style C fill:#FF0000
    style D fill:#FFA500
```

**Result:** ❌ Limited to 1-hour sessions

## Key Differences

| Aspect | Our Setup | Colleague's Setup |
|--------|-----------|-------------------|
| First Principal | Lambda Execution Role (attached) | Assumed Role |
| Role Hops | 1 (direct) | 2+ (chained) |
| Max Duration | 3 hours | 1 hour |
| Error Message | None | "exceeds the 1 hour session limit" |

## How Lambda Execution Roles Work

Lambda execution roles are special:
- Directly attached to the Lambda function
- Not considered "assumed roles" for chaining purposes
- AWS Lambda service assumes the role on behalf of your function
- Credentials are injected into the Lambda environment

This is why our Lambda → TenantAccessRole is NOT role chaining!