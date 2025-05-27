#!/bin/bash
# Script to verify we're not using role chaining

echo "Checking for potential role chaining issues..."
echo

# Check if running in Lambda
if [ -n "$AWS_LAMBDA_FUNCTION_NAME" ]; then
    echo "✓ Running in Lambda environment"
    echo "  Function: $AWS_LAMBDA_FUNCTION_NAME"
    echo "  Execution Role: $AWS_LAMBDA_FUNCTION_ROLE"
    
    # Check credentials
    echo
    echo "Checking credential chain..."
    aws sts get-caller-identity
    
    # Check if credentials are from assumed role
    if aws sts get-caller-identity | grep -q "assumed-role"; then
        echo "⚠️  WARNING: Running with assumed role credentials"
        echo "  This could lead to role chaining if you assume another role"
    else
        echo "✓ Using direct IAM credentials (no role chaining risk)"
    fi
else
    echo "⚠️  Not running in Lambda environment"
    echo "  Local testing might use different credential chain"
    
    # Check current credentials
    echo
    echo "Current credentials:"
    aws sts get-caller-identity
    
    # Check if using assumed role
    if aws sts get-caller-identity | grep -q "assumed-role"; then
        echo
        echo "⚠️  WARNING: You're using assumed role credentials!"
        echo "  If you AssumeRole from here, you'll be role chaining"
        echo "  Maximum session duration will be limited to 1 hour"
    fi
fi

echo
echo "Testing AssumeRole duration limits..."
echo "(This will fail if role chaining is detected)"

# Try to assume role with 3-hour duration
ROLE_ARN="${TENANT_ACCESS_ROLE_ARN:-arn:aws:iam::123456789012:role/TenantAccessRole}"
echo "Attempting to assume $ROLE_ARN for 3 hours..."

aws sts assume-role \
    --role-arn "$ROLE_ARN" \
    --role-session-name "test-session" \
    --duration-seconds 10800 \
    --tags Key=tenant_id,Value=test-tenant \
    2>&1 | grep -E "(AssumedRoleUser|Error|ValidationError)"