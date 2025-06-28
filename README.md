# MWC Backend

This is the backend service for the MWC application.

## Docker Setup

This project includes Docker configuration for easy setup and deployment.

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Docker Compose](https://docs.docker.com/compose/install/)

### Environment Variables

Before running the application, you need to set up the following environment variables in the `docker-compose.yml` file or create a `.env` file:

- `PORT`: The port on which the application will run (default: 8080)
- `DATABASE_URL`: PostgreSQL connection string
- `RABBITMQ_URL`: RabbitMQ connection string
- `SMTP_HOST`: SMTP server host
- `SMTP_PORT`: SMTP server port
- `SMTP_USER`: SMTP username
- `SMTP_PASSWORD`: SMTP password
- `EMAIL_FROM`: Email sender address
- `JWT_SECRET`: Secret key for JWT token generation
- `STRIPE_SECRET_KEY`: Stripe API secret key
- `STRIPE_WEBHOOK_SECRET`: Stripe webhook secret
- `STRIPE_MONTHLY_PRICE_ID`: Stripe price ID for monthly subscription
- `STRIPE_ANNUAL_PRICE_ID`: Stripe price ID for annual subscription

### Building and Running

To build and run the application using Docker Compose:

```bash
# Build the Docker images
docker-compose build

# Start the services
docker-compose up -d

# View logs
docker-compose logs -f
```

### Services

The Docker Compose setup includes the following services:

1. **app**: The main Go application
2. **postgres**: PostgreSQL database
3. **rabbitmq**: RabbitMQ message broker

### Accessing Services

- **Backend API**: http://localhost:8080
- **RabbitMQ Management UI**: http://localhost:15672 (username: guest, password: guest)

### Stopping the Services

```bash
# Stop the services
docker-compose down

# Stop the services and remove volumes
docker-compose down -v
```

## Development

For local development without Docker:

1. Install Go 1.23 or later
2. Install PostgreSQL and RabbitMQ
3. Set up environment variables or create a `.env` file
4. Run the application:

```bash
go run main.go
```

## GitHub Workflow for AWS ECR Deployment

This project includes a GitHub Actions workflow for automatically building and pushing the Docker image to AWS Elastic Container Registry (ECR).

### Workflow Features

- Automatically builds and pushes the Docker image to AWS ECR on pushes to the main branch
- Supports manual triggering with environment selection (dev, staging, prod)
- Uses Docker layer caching for faster builds
- Tags images with commit SHA, environment name, and 'latest'
- Sends Slack notifications for successful and failed deployments

### Required GitHub Secrets

To use this workflow, you need to set up the following secrets in your GitHub repository:

- `AWS_ACCESS_KEY_ID`: Your AWS access key with permissions to push to ECR
- `AWS_SECRET_ACCESS_KEY`: Your AWS secret access key
- `SLACK_WEBHOOK`: Your Slack webhook URL for sending notifications

### Slack Notifications Setup

To set up Slack notifications:

1. Create a Slack app in your workspace or use an existing one
2. Enable Incoming Webhooks for your Slack app
3. Create a new webhook URL for the channel where you want to receive deployment notifications
4. Add the webhook URL as the `SLACK_WEBHOOK` secret in your GitHub repository

The workflow will send notifications to the specified Slack channel when deployments succeed or fail.

### Configuration

The workflow can be configured by modifying the following environment variables in the `.github/workflows/aws-ecr-push.yml` file:

- `AWS_REGION`: The AWS region where your ECR repository is located (default: us-east-1)
- `ECR_REPOSITORY`: The name of your ECR repository (default: mwc-backend)

### Manual Deployment

To manually trigger a deployment:

1. Go to the "Actions" tab in your GitHub repository
2. Select the "Build and Push to AWS ECR" workflow
3. Click "Run workflow"
4. Select the target environment (dev, staging, or prod)
5. Click "Run workflow" to start the deployment

### AWS ECR Repository Setup

Before using this workflow, make sure you have:

1. Created an ECR repository in your AWS account
2. Created an IAM user with appropriate permissions for ECR
3. Generated access keys for the IAM user
4. Added the access keys as secrets in your GitHub repository
