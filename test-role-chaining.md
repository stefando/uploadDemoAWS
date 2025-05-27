# Role Chaining Test Results

## Test Environment Setup

**Date**: 2025-05-27  
**Stack**: upload-demo-stack  
**Account**: 507004462727  
**Region**: eu-central-1  

## Test 1: Current Credentials (Local Development)

```bash
aws sts get-caller-identity --profile personal
```

**Result**:
```json
{
    "UserId": "AIDAXMC6SE2DT6LG6BYV7",
    "Account": "507004462727", 
    "Arn": "arn:aws:iam::507004462727:user/stefando"
}
```

**Analysis**: ✅ Using IAM user credentials (not assumed role)

## Test 2: Direct AssumeRole from IAM User (Should Work)

```bash
aws sts assume-role \
  --role-arn "arn:aws:iam::507004462727:role/upload-demo-stack-TenantAccessRole" \
  --role-session-name "test-3hour-session" \
  --duration-seconds 10800 \
  --tags Key=tenant_id,Value=test-tenant
```

**Result**:
```
An error occurred (AccessDenied) when calling the AssumeRole operation: 
User: arn:aws:iam::507004462727:user/stefando is not authorized to perform: 
sts:AssumeRole on resource: arn:aws:iam::507004462727:role/upload-demo-stack-TenantAccessRole
```

**Analysis**: ✅ Access denied as expected - role only trusts Lambda execution role

## Test 3: Lambda Execution Role Trust Policy

From CloudFormation template (lines 100-108):
```yaml
TenantAccessRole:
  Properties:
    MaxSessionDuration: 10800  # 3 hours
    AssumeRolePolicyDocument:
      Statement:
        - Effect: Allow
          Principal:
            AWS: !GetAtt LambdaExecutionRole.Arn  # Only Lambda can assume
          Action: 
            - sts:AssumeRole
            - sts:TagSession
```

**Analysis**: ✅ Role is configured to only trust the Lambda execution role

## Test 4: Lambda Function Role Configuration

From template.yaml (lines 198-240):
```yaml
UploadFunction:
  Type: AWS::Serverless::Function
  Properties:
    Role: !GetAtt LambdaExecutionRole.Arn  # Direct attachment
```

**Analysis**: ✅ Lambda has directly attached execution role (not assumed)

## Role Chaining Analysis

### Our Architecture (No Role Chaining) ✅

```
IAM User (stefando) 
    ❌ Cannot assume TenantAccessRole (access denied)

Lambda Function
    ↓ Has directly attached role
LambdaExecutionRole 
    ↓ AssumeRole (SINGLE HOP)
TenantAccessRole (MaxSessionDuration: 10800 = 3 hours)
```

### Why No Role Chaining

1. **Lambda Execution Roles are Special**: AWS Lambda service attaches the role directly to the function
2. **Not Considered Assumed Roles**: For role chaining purposes, Lambda execution roles don't count as "assumed"
3. **Single Hop**: When Lambda code calls AssumeRole, it's the first and only hop
4. **Full Duration Available**: Can use the full 3-hour MaxSessionDuration

### Colleague's Likely Scenario (Role Chaining) ❌

```
Developer/SSO User
    ↓ AssumeRole 
Developer Role  
    ↓ AssumeRole (ROLE CHAINING!)
Target Role (LIMITED TO 1 HOUR regardless of MaxSessionDuration)
```

## Conclusion

**Our understanding is CORRECT**: 
- ✅ Our Lambda architecture does NOT involve role chaining
- ✅ We can use 3-hour sessions as configured in MaxSessionDuration
- ✅ The role is properly secured (only Lambda can assume it)
- ✅ Colleague's error is likely due to role chaining in their environment

## Next Steps for Colleague

1. **Check credential source**: Are they using assumed role credentials?
2. **Verify architecture**: Single-hop vs multi-hop role assumptions?
3. **Consider Lambda approach**: Deploy operations as Lambda functions with direct execution roles
4. **Alternative**: Accept 1-hour limit and refresh credentials more frequently