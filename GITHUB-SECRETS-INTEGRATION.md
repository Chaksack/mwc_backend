# GitHub Secrets Integration for AWS ECS Deployment

## Overview

This document explains the changes made to use GitHub secrets instead of AWS Secrets Manager for environment variables in the ECS deployment process.

## Changes Made

The following changes were made to enable the use of GitHub secrets:

1. **Updated task-definition.json**:
   - Removed the "secrets" section that was using AWS Secrets Manager
   - Added all sensitive environment variables to the "environment" section with placeholder values
   - This allows the task definition to use environment variables that can be replaced with GitHub secrets during deployment

2. **Updated aws-ecs-deploy.yml workflow**:
   - Added a new step called "Replace environment variables with GitHub secrets"
   - This step uses sed commands to replace each placeholder in the task definition with the corresponding GitHub secret
   - The GitHub secrets are accessed using the `${{ secrets.SECRET_NAME }}` syntax

## Benefits

Using GitHub secrets instead of AWS Secrets Manager provides several benefits:

1. **Simplified Management**: All secrets can be managed in one place (GitHub) rather than across multiple services
2. **Reduced AWS Costs**: No need to pay for AWS Secrets Manager
3. **Improved Security**: Secrets are never stored in the repository, only referenced during deployment
4. **Easier Updates**: Secrets can be updated directly in GitHub without needing to update AWS Secrets Manager

## Required GitHub Secrets

The following secrets need to be configured in your GitHub repository:

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

## How to Configure GitHub Secrets

1. Go to your GitHub repository
2. Click on "Settings"
3. Click on "Secrets and variables" in the left sidebar
4. Click on "Actions"
5. Click on "New repository secret"
6. Enter the name of the secret (e.g., `DATABASE_URL`)
7. Enter the value of the secret
8. Click on "Add secret"
9. Repeat for each required secret

## Conclusion

With these changes, the ECS deployment process now uses GitHub secrets instead of AWS Secrets Manager for environment variables. This simplifies the management of secrets and reduces the dependency on AWS Secrets Manager.