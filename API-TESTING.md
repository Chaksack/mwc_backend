# API Testing Guide

This document provides instructions on how to test the API functionality of the Montessori World Connect backend.

## Prerequisites

Before running the tests, make sure you have the following:

1. The backend server is running locally on port 8080
2. The database is properly set up with test data
3. Test users with different roles are available:
   - Admin user: admin@example.com / Admin123!
   - Institution user: institution@example.com / Inst123!
   - Educator user: educator@example.com / Edu123!
   - Parent user: parent@example.com / Parent123!

## Starting the Server

To start the server, run the following command from the project root:

```bash
go run main.go
```

This will start the server on http://localhost:8080.

## Running the Tests

Once the server is running, you can run the API tests using the provided test script:

```bash
./test_api.sh
```

This script will test various aspects of the API:

1. Public endpoints (no authentication required)
2. Authentication for different user roles
3. Role-specific endpoints (admin, institution, educator, parent)
4. CRUD operations for key resources (schools, jobs, reviews, events, blog posts)
5. Error handling

## Test Results

The test script will output the results of each test, including:

- The endpoint being tested
- The HTTP method used
- The expected status code
- The actual status code
- The response body

At the end of the tests, a summary will be displayed showing which tests were run successfully and which were skipped due to missing tokens or IDs.

## Troubleshooting

If the tests fail, check the following:

1. Make sure the server is running on http://localhost:8080
2. Verify that the database is properly set up with test data
3. Check that the test users exist with the correct credentials
4. Look for any error messages in the server logs

## Modifying the Tests

If you need to modify the tests, you can edit the `test_api.sh` script. The script is organized into functions for testing different aspects of the API:

- `test_public_endpoints()`: Tests public endpoints
- `test_authentication()`: Tests authentication for different user roles
- `test_admin_endpoints()`: Tests admin-specific endpoints
- `test_institution_endpoints()`: Tests institution-specific endpoints
- `test_educator_endpoints()`: Tests educator-specific endpoints
- `test_parent_endpoints()`: Tests parent-specific endpoints
- `test_error_handling()`: Tests error handling
- `test_school_crud()`: Tests CRUD operations for schools
- `test_job_crud()`: Tests CRUD operations for jobs
- `test_review_crud()`: Tests CRUD operations for reviews
- `test_event_crud()`: Tests CRUD operations for events
- `test_blog_crud()`: Tests CRUD operations for blog posts

You can modify these functions to add or remove tests as needed.

## Authentication Changes

The API now uses a simplified authentication method where the token is provided directly in the Authorization header without the "Bearer" prefix. This change has been reflected in the test script.

Example:
```
Authorization: your_token_here
```

Instead of:
```
Authorization: Bearer your_token_here
```

Make sure your API client is updated to use this new authentication method.