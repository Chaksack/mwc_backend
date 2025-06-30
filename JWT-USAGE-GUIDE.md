# JWT Authentication Guide

## Introduction

This guide explains how to properly use JWT (JSON Web Token) authentication with the Montessori World Connect API. After logging in, you'll receive a JWT token that must be included in all subsequent requests to protected endpoints.

## Authentication Flow

1. **Login**: Send a POST request to `/api/v1/login` with your credentials to receive a JWT token.
2. **Use the token**: Include the token in the `Authorization` header of all subsequent requests to protected endpoints.

## Example Login Response

```json
{
  "message": "Login successful",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxLCJlbWFpbCI6ImFkbWluQGV4YW1wbGUuY29tIiwicm9sZSI6ImFkbWluIiwiaXNzIjoiZ29fZmliZXJfYXBwIiwiZXhwIjoxNzUxNTgwMTI3LCJuYmYiOjE3NTEzMjA5MjcsImlhdCI6MTc1MTMyMDkyN30.nHwOO38ceU8RjHatL4VbDnPzTTjqnYjydbjG4KlP1U8",
  "user": {
    "email": "admin@example.com",
    "firstName": "Admin",
    "id": 1,
    "lastName": "User",
    "role": "admin"
  }
}
```

## How to Include the Token in Requests

### Using cURL

```bash
curl -X GET "https://api.montessoriworldconnect.com/api/v1/admin/users" \
  -H "Authorization: Bearer YOUR_TOKEN_HERE"
```

Replace `YOUR_TOKEN_HERE` with the token you received from the login response.

### Using JavaScript Fetch API

```javascript
// First, login to get the token
async function login(email, password) {
  const response = await fetch('https://api.montessoriworldconnect.com/api/v1/login', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ email, password })
  });
  
  const data = await response.json();
  return data.token; // Store this token securely
}

// Then, use the token for subsequent requests
async function fetchProtectedResource(token) {
  const response = await fetch('https://api.montessoriworldconnect.com/api/v1/admin/users', {
    method: 'GET',
    headers: {
      'Authorization': `Bearer ${token}`
    }
  });
  
  return await response.json();
}

// Example usage
async function example() {
  const token = await login('admin@example.com', 'password');
  const users = await fetchProtectedResource(token);
  console.log(users);
}
```

### Using Axios

```javascript
// First, login to get the token
async function login(email, password) {
  const response = await axios.post('https://api.montessoriworldconnect.com/api/v1/login', {
    email,
    password
  });
  
  return response.data.token; // Store this token securely
}

// Then, use the token for subsequent requests
async function fetchProtectedResource(token) {
  const response = await axios.get('https://api.montessoriworldconnect.com/api/v1/admin/users', {
    headers: {
      'Authorization': `Bearer ${token}`
    }
  });
  
  return response.data;
}

// Example usage
async function example() {
  const token = await login('admin@example.com', 'password');
  const users = await fetchProtectedResource(token);
  console.log(users);
}
```

### Using Postman

1. Send a POST request to `/api/v1/login` with your credentials in the request body.
2. Copy the token from the response.
3. For subsequent requests, go to the "Authorization" tab, select "Bearer Token" from the dropdown, and paste your token in the "Token" field.

## Common Issues

### "Missing Authorization header" Error

If you receive the following error:

```json
{
  "error": "Missing Authorization header. Please include 'Authorization: Bearer your_token_here' in your request."
}
```

This means you haven't included the `Authorization` header in your request. Make sure to include it with the format `Bearer YOUR_TOKEN_HERE`.

### "Malformed Authorization header" Error

If you receive the following error:

```json
{
  "error": "Malformed Authorization header. Format should be 'Bearer your_token_here'."
}
```

This means the format of your `Authorization` header is incorrect. Make sure it starts with `Bearer ` followed by your token.

### "Invalid JWT token" Error

If you receive an error about an invalid JWT token, your token might be expired or malformed. Try logging in again to get a new token.

## Security Considerations

- Store the JWT token securely (e.g., in HttpOnly cookies for web applications).
- Never store tokens in local storage for web applications, as they are vulnerable to XSS attacks.
- The token has an expiration time. If you receive an "expired token" error, you'll need to log in again to get a new token.