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