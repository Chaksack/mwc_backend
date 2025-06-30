# AWS ECS Deployment Plan for MWC Backend

This document outlines the steps to deploy the MWC Backend application to AWS Elastic Container Service (ECS) using the Docker image stored in Amazon Elastic Container Registry (ECR).

## Prerequisites

- AWS Account with appropriate permissions
- AWS CLI installed and configured
- Docker installed locally (for testing)
- Access to the MWC Backend repository

## 1. ECR Setup (Already Implemented)

The application already has a GitHub Actions workflow (`aws-ecr-push.yml`) that handles:

- Building the Docker image
- Pushing the image to Amazon ECR
- Testing the container functionality
- Sending notifications about the deployment status

## 2. ECS Cluster Setup

### 2.1 Create an ECS Cluster

```bash
aws ecs create-cluster --cluster-name mwc-cluster
```

Choose the appropriate cluster type based on your needs:
- **Fargate**: Serverless compute engine (recommended for simplicity)
- **EC2**: More control over the underlying infrastructure

### 2.2 Configure Cluster Capacity Providers

For Fargate:
```bash
aws ecs put-cluster-capacity-providers \
  --cluster mwc-cluster \
  --capacity-providers FARGATE FARGATE_SPOT \
  --default-capacity-provider-strategy capacityProvider=FARGATE,weight=1
```

## 3. Networking Setup

### 3.1 Create a VPC (if not already available)

```bash
aws ec2 create-vpc --cidr-block 10.0.0.0/16 --tag-specifications 'ResourceType=vpc,Tags=[{Key=Name,Value=mwc-vpc}]'
```

### 3.2 Create Subnets

Create at least two subnets in different availability zones:

```bash
aws ec2 create-subnet --vpc-id vpc-xxxxxxxx --cidr-block 10.0.1.0/24 --availability-zone us-east-1a --tag-specifications 'ResourceType=subnet,Tags=[{Key=Name,Value=mwc-subnet-1a}]'
aws ec2 create-subnet --vpc-id vpc-xxxxxxxx --cidr-block 10.0.2.0/24 --availability-zone us-east-1b --tag-specifications 'ResourceType=subnet,Tags=[{Key=Name,Value=mwc-subnet-1b}]'
```

### 3.3 Create Internet Gateway

```bash
aws ec2 create-internet-gateway --tag-specifications 'ResourceType=internet-gateway,Tags=[{Key=Name,Value=mwc-igw}]'
aws ec2 attach-internet-gateway --internet-gateway-id igw-xxxxxxxx --vpc-id vpc-xxxxxxxx
```

### 3.4 Create Route Table

```bash
aws ec2 create-route-table --vpc-id vpc-xxxxxxxx --tag-specifications 'ResourceType=route-table,Tags=[{Key=Name,Value=mwc-rt}]'
aws ec2 create-route --route-table-id rtb-xxxxxxxx --destination-cidr-block 0.0.0.0/0 --gateway-id igw-xxxxxxxx
aws ec2 associate-route-table --route-table-id rtb-xxxxxxxx --subnet-id subnet-xxxxxxxx
aws ec2 associate-route-table --route-table-id rtb-xxxxxxxx --subnet-id subnet-yyyyyyyy
```

### 3.5 Create Security Group

```bash
aws ec2 create-security-group --group-name mwc-sg --description "Security group for MWC Backend" --vpc-id vpc-xxxxxxxx
aws ec2 authorize-security-group-ingress --group-id sg-xxxxxxxx --protocol tcp --port 8080 --cidr 0.0.0.0/0
```

## 4. IAM Roles Setup

### 4.1 Create ECS Task Execution Role

Create a file named `task-execution-assume-role.json`:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "",
      "Effect": "Allow",
      "Principal": {
        "Service": "ecs-tasks.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
```

Create the role:
```bash
aws iam create-role --role-name ecsTaskExecutionRole --assume-role-policy-document file://task-execution-assume-role.json
aws iam attach-role-policy --role-name ecsTaskExecutionRole --policy-arn arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy
```

### 4.2 Create ECS Task Role (for application permissions)

Create a file named `task-role-assume-role.json` with the same content as above, then:
```bash
aws iam create-role --role-name mwcTaskRole --assume-role-policy-document file://task-role-assume-role.json
```

Attach necessary policies based on your application's needs (e.g., S3, RDS, etc.):
```bash
aws iam attach-role-policy --role-name mwcTaskRole --policy-arn arn:aws:iam::aws:policy/AmazonRDSFullAccess
```

## 5. AWS Secrets Manager Setup

Store sensitive environment variables in AWS Secrets Manager:

```bash
aws secretsmanager create-secret --name mwc-backend-secrets \
  --description "Secrets for MWC Backend" \
  --secret-string "{\"DATABASE_URL\":\"postgres://username:password@hostname:5432/dbname\",\"RABBITMQ_URL\":\"amqps://username:password@hostname:5671\",\"JWT_SECRET\":\"your-jwt-secret\",\"SMTP_PASSWORD\":\"your-smtp-password\",\"STRIPE_SECRET_KEY\":\"your-stripe-secret-key\",\"STRIPE_WEBHOOK_SECRET\":\"your-stripe-webhook-secret\"}"
```

## 6. ECS Task Definition

Create a file named `task-definition.json`:

```json
{
  "family": "mwc-backend",
  "executionRoleArn": "arn:aws:iam::ACCOUNT_ID:role/ecsTaskExecutionRole",
  "taskRoleArn": "arn:aws:iam::ACCOUNT_ID:role/mwcTaskRole",
  "networkMode": "awsvpc",
  "containerDefinitions": [
    {
      "name": "mwc-backend",
      "image": "ACCOUNT_ID.dkr.ecr.us-east-1.amazonaws.com/mwc-backend:latest",
      "essential": true,
      "portMappings": [
        {
          "containerPort": 8080,
          "hostPort": 8080,
          "protocol": "tcp"
        }
      ],
      "environment": [
        {
          "name": "PORT",
          "value": "8080"
        },
        {
          "name": "WEBSOCKET_ENABLED",
          "value": "true"
        },
        {
          "name": "WEBSOCKET_PATH",
          "value": "/ws"
        },
        {
          "name": "DEFAULT_LANGUAGE",
          "value": "en"
        },
        {
          "name": "SUPPORTED_LANGUAGES",
          "value": "en,es,fr"
        },
        {
          "name": "RABBITMQ_USE_TLS",
          "value": "true"
        }
      ],
      "secrets": [
        {
          "name": "DATABASE_URL",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:DATABASE_URL::"
        },
        {
          "name": "RABBITMQ_URL",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:RABBITMQ_URL::"
        },
        {
          "name": "JWT_SECRET",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:JWT_SECRET::"
        },
        {
          "name": "SMTP_HOST",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:SMTP_HOST::"
        },
        {
          "name": "SMTP_PORT",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:SMTP_PORT::"
        },
        {
          "name": "SMTP_USER",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:SMTP_USER::"
        },
        {
          "name": "SMTP_PASSWORD",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:SMTP_PASSWORD::"
        },
        {
          "name": "EMAIL_FROM",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:EMAIL_FROM::"
        },
        {
          "name": "STRIPE_SECRET_KEY",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:STRIPE_SECRET_KEY::"
        },
        {
          "name": "STRIPE_PUBLISHABLE_KEY",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:STRIPE_PUBLISHABLE_KEY::"
        },
        {
          "name": "STRIPE_WEBHOOK_SECRET",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:STRIPE_WEBHOOK_SECRET::"
        },
        {
          "name": "STRIPE_MONTHLY_PRICE_ID",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:STRIPE_MONTHLY_PRICE_ID::"
        },
        {
          "name": "STRIPE_ANNUAL_PRICE_ID",
          "valueFrom": "arn:aws:secretsmanager:us-east-1:ACCOUNT_ID:secret:mwc-backend-secrets:STRIPE_ANNUAL_PRICE_ID::"
        }
      ],
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-group": "/ecs/mwc-backend",
          "awslogs-region": "us-east-1",
          "awslogs-stream-prefix": "ecs"
        }
      },
      "healthCheck": {
        "command": ["CMD-SHELL", "curl -f http://localhost:8080/ || exit 1"],
        "interval": 30,
        "timeout": 5,
        "retries": 3,
        "startPeriod": 60
      }
    }
  ],
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "512",
  "memory": "1024"
}
```

Register the task definition:
```bash
aws ecs register-task-definition --cli-input-json file://task-definition.json
```

## 7. Create CloudWatch Log Group

```bash
aws logs create-log-group --log-group-name /ecs/mwc-backend
```

## 8. Create ECS Service

```bash
aws ecs create-service \
  --cluster mwc-cluster \
  --service-name mwc-backend-service \
  --task-definition mwc-backend:1 \
  --desired-count 2 \
  --launch-type FARGATE \
  --platform-version LATEST \
  --network-configuration "awsvpcConfiguration={subnets=[subnet-xxxxxxxx,subnet-yyyyyyyy],securityGroups=[sg-xxxxxxxx],assignPublicIp=ENABLED}" \
  --deployment-configuration "deploymentCircuitBreaker={enable=true,rollback=true},maximumPercent=200,minimumHealthyPercent=100" \
  --health-check-grace-period-seconds 60 \
  --enable-execute-command
```

## 9. Set Up Load Balancer (Optional but Recommended)

### 9.1 Create Application Load Balancer

```bash
aws elbv2 create-load-balancer \
  --name mwc-alb \
  --subnets subnet-xxxxxxxx subnet-yyyyyyyy \
  --security-groups sg-xxxxxxxx \
  --type application
```

### 9.2 Create Target Group

```bash
aws elbv2 create-target-group \
  --name mwc-tg \
  --protocol HTTP \
  --port 8080 \
  --vpc-id vpc-xxxxxxxx \
  --target-type ip \
  --health-check-path / \
  --health-check-interval-seconds 30 \
  --health-check-timeout-seconds 5 \
  --healthy-threshold-count 2 \
  --unhealthy-threshold-count 2
```

### 9.3 Create Listener

```bash
aws elbv2 create-listener \
  --load-balancer-arn arn:aws:elasticloadbalancing:us-east-1:ACCOUNT_ID:loadbalancer/app/mwc-alb/xxxxxxxx \
  --protocol HTTP \
  --port 80 \
  --default-actions Type=forward,TargetGroupArn=arn:aws:elasticloadbalancing:us-east-1:ACCOUNT_ID:targetgroup/mwc-tg/xxxxxxxx
```

### 9.4 Update ECS Service to Use Load Balancer

```bash
aws ecs update-service \
  --cluster mwc-cluster \
  --service mwc-backend-service \
  --load-balancers "targetGroupArn=arn:aws:elasticloadbalancing:us-east-1:ACCOUNT_ID:targetgroup/mwc-tg/xxxxxxxx,containerName=mwc-backend,containerPort=8080"
```

## 10. Set Up Auto Scaling (Optional)

### 10.1 Register Scalable Target

```bash
aws application-autoscaling register-scalable-target \
  --service-namespace ecs \
  --resource-id service/mwc-cluster/mwc-backend-service \
  --scalable-dimension ecs:service:DesiredCount \
  --min-capacity 2 \
  --max-capacity 10
```

### 10.2 Create Scaling Policies

```bash
aws application-autoscaling put-scaling-policy \
  --service-namespace ecs \
  --resource-id service/mwc-cluster/mwc-backend-service \
  --scalable-dimension ecs:service:DesiredCount \
  --policy-name cpu-tracking-scaling-policy \
  --policy-type TargetTrackingScaling \
  --target-tracking-scaling-policy-configuration '{"TargetValue": 70.0, "PredefinedMetricSpecification": {"PredefinedMetricType": "ECSServiceAverageCPUUtilization"}}'
```

## 11. Set Up Monitoring and Logging

### 11.1 Create CloudWatch Dashboard

```bash
aws cloudwatch put-dashboard \
  --dashboard-name MWC-Backend-Dashboard \
  --dashboard-body file://dashboard.json
```

### 11.2 Set Up CloudWatch Alarms

```bash
aws cloudwatch put-metric-alarm \
  --alarm-name MWC-Backend-High-CPU \
  --alarm-description "Alarm when CPU exceeds 80% for 5 minutes" \
  --metric-name CPUUtilization \
  --namespace AWS/ECS \
  --statistic Average \
  --period 300 \
  --threshold 80 \
  --comparison-operator GreaterThanThreshold \
  --dimensions Name=ClusterName,Value=mwc-cluster Name=ServiceName,Value=mwc-backend-service \
  --evaluation-periods 1 \
  --alarm-actions arn:aws:sns:us-east-1:ACCOUNT_ID:mwc-alerts
```

## 12. GitHub Actions Workflow for ECS Deployment

Create a new GitHub Actions workflow file `.github/workflows/aws-ecs-deploy.yml`:

```yaml
name: Deploy to AWS ECS

on:
  workflow_run:
    workflows: ["Build and Push to AWS ECR"]
    types:
      - completed
    branches: [main, staging]
  workflow_dispatch:
    inputs:
      environment:
        description: 'Deployment environment'
        required: true
        default: 'prod'
        type: choice
        options:
          - dev
          - staging
          - prod

env:
  AWS_REGION: us-east-1
  ECS_CLUSTER: mwc-cluster
  ECS_SERVICE: mwc-backend-service
  ECS_TASK_DEFINITION: mwc-backend
  CONTAINER_NAME: mwc-backend

jobs:
  deploy:
    name: Deploy to ECS
    runs-on: ubuntu-latest
    if: ${{ github.event.workflow_run.conclusion == 'success' || github.event_name == 'workflow_dispatch' }}
    
    # Set environment based on trigger
    env:
      ENVIRONMENT: ${{ github.event.inputs.environment || (github.ref == 'refs/heads/main' && 'prod') || 'staging' }}
    
    steps:
      - name: Checkout code
        uses: actions/checkout@v3
      
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v2
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: ${{ env.AWS_REGION }}
      
      - name: Login to Amazon ECR
        id: login-ecr
        uses: aws-actions/amazon-ecr-login@v1
      
      - name: Download task definition
        run: |
          aws ecs describe-task-definition --task-definition ${{ env.ECS_TASK_DEFINITION }} \
          --query taskDefinition > task-definition.json
      
      - name: Update container image
        id: update-image
        env:
          ECR_REGISTRY: ${{ steps.login-ecr.outputs.registry }}
          ECR_REPOSITORY: mwc-backend
          IMAGE_TAG: ${{ env.ENVIRONMENT }}
        run: |
          # Update the image in the task definition
          sed -i "s|$ECR_REGISTRY/$ECR_REPOSITORY:[^ \"]*|$ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG|g" task-definition.json
          echo "::set-output name=image::$ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG"
      
      - name: Register new task definition
        id: register-task
        run: |
          NEW_TASK_DEF=$(aws ecs register-task-definition --cli-input-json file://task-definition.json)
          NEW_REVISION=$(echo $NEW_TASK_DEF | jq -r '.taskDefinition.revision')
          echo "::set-output name=revision::$NEW_REVISION"
      
      - name: Deploy to ECS
        run: |
          aws ecs update-service \
            --cluster ${{ env.ECS_CLUSTER }} \
            --service ${{ env.ECS_SERVICE }} \
            --task-definition ${{ env.ECS_TASK_DEFINITION }}:${{ steps.register-task.outputs.revision }} \
            --force-new-deployment
      
      - name: Wait for service stability
        run: |
          aws ecs wait services-stable \
            --cluster ${{ env.ECS_CLUSTER }} \
            --services ${{ env.ECS_SERVICE }}
      
      - name: Deployment Summary
        run: |
          echo "✅ Deployment Summary"
          echo "Environment: ${{ env.ENVIRONMENT }}"
          echo "Cluster: ${{ env.ECS_CLUSTER }}"
          echo "Service: ${{ env.ECS_SERVICE }}"
          echo "Task Definition: ${{ env.ECS_TASK_DEFINITION }}:${{ steps.register-task.outputs.revision }}"
          echo "Image: ${{ steps.update-image.outputs.image }}"
      
      # Send Slack notification for successful deployment
      - name: Send Slack notification - Success
        if: success()
        uses: rtCamp/action-slack-notify@v2
        env:
          SLACK_CHANNEL: deployments
          SLACK_COLOR: good
          SLACK_ICON: https://github.com/rtCamp.png?size=48
          SLACK_MESSAGE: |
            ✅ Successfully deployed to ECS in ${{ env.ENVIRONMENT }} environment
            *Cluster:* ${{ env.ECS_CLUSTER }}
            *Service:* ${{ env.ECS_SERVICE }}
            *Task Definition:* ${{ env.ECS_TASK_DEFINITION }}:${{ steps.register-task.outputs.revision }}
            *Image:* ${{ steps.update-image.outputs.image }}
          SLACK_TITLE: ECS Deployment Success
          SLACK_USERNAME: GitHub Actions
      
      # Send Slack notification for failed deployment
      - name: Send Slack notification - Failure
        if: failure()
        uses: rtCamp/action-slack-notify@v2
        env:
          SLACK_CHANNEL: deployments
          SLACK_COLOR: danger
          SLACK_ICON: https://github.com/rtCamp.png?size=48
          SLACK_MESSAGE: |
            ❌ Failed to deploy to ECS in ${{ env.ENVIRONMENT }} environment
            *Repository:* ${{ github.repository }}
            *Workflow:* ${{ github.workflow }}
            *Check the [GitHub Actions logs](https://github.com/${{ github.repository }}/actions/runs/${{ github.run_id }}) for more details*
          SLACK_TITLE: ECS Deployment Failure
          SLACK_USERNAME: GitHub Actions
```

## 13. Continuous Deployment Process

1. Developer pushes code to the repository
2. GitHub Actions workflow `aws-ecr-push.yml` builds and pushes the Docker image to ECR
3. GitHub Actions workflow `aws-ecs-deploy.yml` deploys the new image to ECS
4. ECS performs a rolling update of the service
5. CloudWatch monitors the application and sends alerts if necessary

## 14. Rollback Process

If a deployment fails or causes issues:

1. Identify the previous working task definition revision:
   ```bash
   aws ecs describe-task-definition --task-definition mwc-backend
   ```

2. Update the service to use the previous revision:
   ```bash
   aws ecs update-service \
     --cluster mwc-cluster \
     --service mwc-backend-service \
     --task-definition mwc-backend:PREVIOUS_REVISION \
     --force-new-deployment
   ```

## 15. Maintenance and Troubleshooting

### 15.1 View Service Logs

```bash
aws logs get-log-events \
  --log-group-name /ecs/mwc-backend \
  --log-stream-name ecs/mwc-backend/TASK_ID
```

### 15.2 Execute Commands in Running Container

```bash
aws ecs execute-command \
  --cluster mwc-cluster \
  --task TASK_ID \
  --container mwc-backend \
  --interactive \
  --command "/bin/sh"
```

### 15.3 Update Environment Variables

1. Update the secrets in AWS Secrets Manager
2. Register a new task definition with updated environment variables
3. Update the service to use the new task definition

## 16. Cost Optimization

- Use Fargate Spot for non-critical workloads
- Set up auto-scaling to scale down during low-traffic periods
- Use Reserved Instances for predictable workloads
- Monitor and optimize resource allocation (CPU, memory)
- Use AWS Cost Explorer to identify cost-saving opportunities

## 17. Security Considerations

- Use AWS Secrets Manager for sensitive information
- Implement least privilege IAM policies
- Enable VPC Flow Logs for network monitoring
- Set up AWS Config for compliance monitoring
- Implement AWS WAF for web application firewall protection
- Enable AWS GuardDuty for threat detection