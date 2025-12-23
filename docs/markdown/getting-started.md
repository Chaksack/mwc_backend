# Getting Started

## Authentication

Most endpoints in the Montessori World Connect API require authentication using **JWT (JSON Web Token)**. Here's how to get started:

### Step 1: Register an Account

Create a new account by sending a POST request to `/api/v1/register`:

```json
POST /api/v1/register
Content-Type: application/json

{
  "email": "your@email.com",
  "password": "SecurePassword123",
  "first_name": "John",
  "last_name": "Doe",
  "role": "parent",
  "institution_name": "Optional (required for institution/training_center)"
}
```

**Available Roles:**
- `institution` - Montessori schools
- `training_center` - Montessori training centers
- `montessori_professional` - Educators
- `parent` - Parents

### Step 2: Verify Email

After registration, check your email for a verification link. Click the link or use the token to verify your account.

### Step 3: Login

Once verified, login to receive your JWT token:

```json
POST /api/v1/login
Content-Type: application/json

{
  "email": "your@email.com",
  "password": "SecurePassword123"
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": 123,
    "email": "your@email.com",
    "first_name": "John",
    "last_name": "Doe",
    "role": "parent"
  }
}
```

### Step 4: Use the Token

Include the token in the `Authorization` header of your requests:

```
Authorization: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Important:** Send the token directly without any prefix like "Bearer".

## Public Endpoints

Some endpoints don't require authentication:

- `POST /api/v1/register` - Register new account
- `POST /api/v1/login` - User login
- `GET /api/v1/schools/public` - Search public schools
- `GET /api/v1/institutions/search` - Search institutions
- `GET /api/v1/institutions/:id` - View institution details
- `GET /api/v1/jobs` - Browse job listings
- `GET /api/v1/events` - View events
- `GET /api/v1/blogs` - Read blog posts
- `GET /api/v1/subscription/plans` - View subscription plans

## Free Trial

All new users automatically receive a **60-day free trial** with access to:

✅ Advanced school search and filtering  
✅ Direct messaging with institutions  
✅ Priority job listings and applications  
✅ Exclusive educational resources  
✅ Community forums and networking  

After the trial period, upgrade to a paid plan to continue accessing premium features.

## Error Handling

The API uses standard HTTP status codes and returns errors in this format:

```json
{
  "error": "Descriptive error message"
}
```

### Common Status Codes

| Code | Meaning | Description |
|------|---------|-------------|
| 200 | OK | Request succeeded |
| 201 | Created | Resource created successfully |
| 400 | Bad Request | Invalid request parameters |
| 401 | Unauthorized | Authentication required or invalid token |
| 403 | Forbidden | Insufficient permissions |
| 404 | Not Found | Resource not found |
| 409 | Conflict | Resource conflict (e.g., duplicate email) |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | Server error |

## Rate Limiting

To ensure fair usage and system stability, API requests are rate-limited. If you exceed the limit, you'll receive a `429 Too Many Requests` response. Implement exponential backoff in your application to handle this gracefully.

## Pagination

List endpoints support pagination using query parameters:

```
GET /api/v1/schools/public?page=1&limit=10
```

**Parameters:**
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 10, max: 100)

**Response includes metadata:**
```json
{
  "data": [...],
  "meta": {
    "total": 100,
    "page": 1,
    "limit": 10,
    "last_page": 10,
    "has_more": true
  }
}
```
