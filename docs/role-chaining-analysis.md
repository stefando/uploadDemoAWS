# Role Chaining Analysis and Solutions

## The Issue

AWS enforces a 1-hour maximum session duration when role chaining occurs, regardless of the `MaxSessionDuration` setting on the target role.

## What is Role Chaining?

Role chaining happens when you:
1. Assume Role A (from user or another principal)
2. Use Role A's temporary credentials to assume Role B
3. Role B's session is limited to 1 hour max (role chaining limit)

## Our Architecture (Safe from Role Chaining)

```
Lambda Function
├── Has: LambdaExecutionRole (directly attached)
└── Can Assume: TenantAccessRole (up to 3 hours)
```

**Why we're safe:**
- Lambda execution roles are NOT assumed roles
- Single-hop AssumeRole from Lambda → TenantAccessRole
- Can use full 3-hour MaxSessionDuration

## Common Scenarios That Cause Role Chaining

### 1. Local Development Testing
```
Developer → AssumeRole → DevRole → AssumeRole → TenantAccessRole
                                    ↑
                            ROLE CHAINING (1 hour limit)
```

**Solution:** Use IAM user credentials or AWS SSO for local testing

### 2. Cross-Account Access
```
Lambda (Account A) → AssumeRole → CrossAccountRole → AssumeRole → TargetRole
                                                      ↑
                                              ROLE CHAINING (1 hour limit)
```

**Solution:** Grant direct cross-account access from Lambda execution role

### 3. Using AWS SSO/Identity Center
```
SSO User → AssumeRole → PermissionSet → AssumeRole → TargetRole
                                         ↑
                                 ROLE CHAINING (1 hour limit)
```

**Solution:** Configure SSO to directly assume the target role

## How to Detect Role Chaining

### Check Current Principal
```bash
aws sts get-caller-identity
```

If the output shows `arn:aws:sts::...:assumed-role/...`, you're using assumed role credentials.

### Test AssumeRole Duration
```bash
# This will fail with role chaining
aws sts assume-role \
  --role-arn arn:aws:iam::123456789012:role/TargetRole \
  --role-session-name test \
  --duration-seconds 3600
```

Error message will mention "1 hour session limit for roles assumed by role chaining"

## Solutions for Your Colleague

### Option 1: Direct IAM User Access (Development)
```python
# Use IAM user credentials directly
session = boto3.Session(
    aws_access_key_id='AKIAIOSFODNN7EXAMPLE',
    aws_secret_access_key='wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY'
)
```

### Option 2: Modify Trust Policy
Allow the first role to directly access S3 without assuming another role:
```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["s3:*"],
    "Resource": "arn:aws:s3:::bucket/*",
    "Condition": {
      "StringEquals": {
        "aws:PrincipalTag/tenant_id": "${aws:PrincipalTag/tenant_id}"
      }
    }
  }]
}
```

### Option 3: Use Lambda for Long Sessions
Deploy operations requiring long sessions as Lambda functions with direct execution roles.

### Option 4: External ID Pattern
Instead of role chaining, use external IDs for cross-account access:
```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {
      "AWS": "arn:aws:iam::ACCOUNT-B:root"
    },
    "Action": "sts:AssumeRole",
    "Condition": {
      "StringEquals": {
        "sts:ExternalId": "unique-external-id"
      }
    }
  }]
}
```

## Best Practices

1. **Avoid Role Chaining** when long sessions are needed
2. **Use Direct Execution Roles** for Lambda functions
3. **Monitor Session Duration** in CloudTrail logs
4. **Document Credential Chain** in your architecture
5. **Test Locally** with the same credential pattern as production

## Testing Script

Use the provided `scripts/verify-role-chain.sh` to test your environment for role chaining issues.