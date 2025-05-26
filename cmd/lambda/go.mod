module github.com/stefando/uploadDemoAWS/cmd/lambda

go 1.24

require (
    github.com/aws/aws-lambda-go v1.48.0
    github.com/aws/aws-sdk-go-v2 v1.36.0
    github.com/aws/aws-sdk-go-v2/config v1.29.1
    github.com/aws/aws-sdk-go-v2/service/s3 v1.77.0
    github.com/aws/aws-sdk-go-v2/service/sts v1.33.4
    github.com/go-chi/chi/v5 v5.2.0
    github.com/google/uuid v1.6.0
    github.com/stefando/uploadDemoAWS v0.0.0
)

replace github.com/stefando/uploadDemoAWS => ../..