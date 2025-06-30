# AWS ECS Deployment Guide

This guide provides instructions for deploying the MWC Backend application to AWS Elastic Container Service (ECS) using the Docker image stored in Amazon Elastic Container Registry (ECR).

## Overview

The deployment process consists of two main steps:
1. Building and pushing the Docker image to ECR
2. Deploying the image to ECS

These steps are automated using GitHub Actions workflows.

## Prerequisites

Before you begin, ensure you have:

1. An AWS account with appropriate permissions
2. The following AWS resources set up:
   - ECR repository
   - ECS cluster
   - VPC, subnets, and security groups
   - IAM roles for ECS tasks
   - AWS Secrets Manager for storing sensitive environment variables

## Setup Steps

### 1. Set up AWS Resources

Follow the detailed instructions in the [AWS ECS Deployment Plan](aws-ecs-deployment-plan.md) to set up the required AWS resources.

### 2. Configure GitHub Secrets

Add the following secrets to your GitHub repository:

- `AWS_ACCESS_KEY_ID`: Your AWS access key with permissions to push to ECR and deploy to ECS
- `AWS_SECRET_ACCESS_KEY`: Your AWS secret access key
- `SLACK_WEBHOOK`: Your Slack webhook URL for sending notifications (optional)
- `DATABASE_URL`: PostgreSQL connection string
- `RABBITMQ_URL`: RabbitMQ connection string
- `JWT_SECRET`: Secret key for JWT token generation
- `SMTP_HOST`: SMTP server host
- `SMTP_PORT`: SMTP server port
- `SMTP_USER`: SMTP username
- `SMTP_PASSWORD`: SMTP password
- `EMAIL_FROM`: Email sender address
- `STRIPE_SECRET_KEY`: Stripe API secret key
- `STRIPE_PUBLISHABLE_KEY`: Stripe API publishable key
- `STRIPE_WEBHOOK_SECRET`: Stripe webhook secret
- `STRIPE_MONTHLY_PRICE_ID`: Stripe price ID for monthly subscription
- `STRIPE_ANNUAL_PRICE_ID`: Stripe price ID for annual subscription

### 3. Update Task Definition

Edit the `task-definition.json` file to replace `ACCOUNT_ID` with your actual AWS account ID and update any other values as needed.

### 4. Deploy to ECS

The deployment to ECS is automated using GitHub Actions workflows:

1. **Build and Push to ECR**: This workflow builds the Docker image and pushes it to ECR. It is triggered on pushes to the main branch or can be manually triggered.

2. **Deploy to ECS**: This workflow deploys the image from ECR to ECS. It is triggered automatically after the "Build and Push to ECR" workflow completes successfully or can be manually triggered.

## Manual Deployment

### Manual ECR Push

To manually trigger the ECR push workflow:

1. Go to the "Actions" tab in your GitHub repository
2. Select the "Build and Push to AWS ECR" workflow
3. Click "Run workflow"
4. Select the target environment (dev, staging, or prod)
5. Click "Run workflow" to start the build and push process

### Manual ECS Deployment

To manually trigger the ECS deployment workflow:

1. Go to the "Actions" tab in your GitHub repository
2. Select the "Deploy to AWS ECS" workflow
3. Click "Run workflow"
4. Select the target environment (dev, staging, or prod)
5. Click "Run workflow" to start the deployment process

## Monitoring and Troubleshooting

### Viewing Logs

To view the application logs:

```bash
aws logs get-log-events \
  --log-group-name /ecs/mwc-backend \
  --log-stream-name ecs/mwc-backend/TASK_ID
```

### Executing Commands in Running Container

To execute commands in a running container:

```bash
aws ecs execute-command \
  --cluster mwc-cluster \
  --task TASK_ID \
  --container mwc-backend \
  --interactive \
  --command "/bin/sh"
```

### Rollback Process

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

## Additional Resources

- [AWS ECS Documentation](https://docs.aws.amazon.com/ecs/)
- [AWS ECR Documentation](https://docs.aws.amazon.com/ecr/)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)