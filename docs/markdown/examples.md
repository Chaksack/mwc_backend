# API Examples

This guide provides practical examples for common API operations.

## Authentication Examples

### Register a New User

**Institution:**
```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "school@example.com",
    "password": "SecurePass123",
    "first_name": "Maria",
    "last_name": "Montessori",
    "role": "institution",
    "institution_name": "Bright Future Montessori"
  }'
```

**Parent:**
```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "parent@example.com",
    "password": "SecurePass123",
    "first_name": "John",
    "last_name": "Doe",
    "role": "parent"
  }'
```

### Login

```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "your@email.com",
    "password": "SecurePass123"
  }'
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
    "role": "parent",
    "email_verified": true
  }
}
```

## School Search Examples

### Search Public Schools

```bash
curl -X GET "https://api.montessoriworldconnect.com/api/v1/schools/public?name=montessori&city=New%20York&page=1&limit=10"
```

### Search Institutions

```bash
curl -X GET "https://api.montessoriworldconnect.com/api/v1/institutions/search?q=bright&country_code=US&verified=true&page=1&limit=10"
```

**Response:**
```json
{
  "data": [
    {
      "id": 1,
      "institution_name": "Bright Future Montessori",
      "is_verified": true,
      "profile_picture_url": "https://...",
      "school": {
        "id": 123,
        "name": "Bright Future Montessori School",
        "city": "New York",
        "state": "NY",
        "country": "United States",
        "country_code": "US"
      }
    }
  ],
  "meta": {
    "total": 1,
    "page": 1,
    "limit": 10,
    "last_page": 1
  }
}
```

## Institution Profile Examples

### Create/Update Institution Profile

```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/institution/profile \
  -H "Authorization: your-jwt-token" \
  -F "institution_name=Bright Future Montessori" \
  -F "school_name=Bright Future School" \
  -F "school_city=New York" \
  -F "school_state=NY" \
  -F "school_country=United States" \
  -F "school_country_code=US" \
  -F "school_email=info@brightfuture.com" \
  -F "school_phone=+1234567890" \
  -F "profile_picture=@/path/to/image.jpg"
```

### Get Institution Details

```bash
curl -X GET https://api.montessoriworldconnect.com/api/v1/institutions/123 \
  -H "Authorization: your-jwt-token"
```

## Job Posting Examples

### Create Job Posting

```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/institution/jobs \
  -H "Authorization: your-jwt-token" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Lead Montessori Teacher",
    "description": "We are seeking an experienced Montessori teacher...",
    "location": "New York, NY",
    "job_type": "full-time",
    "salary_min": 45000,
    "salary_max": 65000,
    "requirements": "AMI/AMS certification required"
  }'
```

### Browse All Jobs

```bash
curl -X GET "https://api.montessoriworldconnect.com/api/v1/jobs?page=1&limit=20"
```

### Apply for Job (Montessori Professional)

```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/montessori-professional/jobs/apply/456 \
  -H "Authorization: your-jwt-token" \
  -H "Content-Type: application/json" \
  -d '{
    "cover_letter": "I am very interested in this position..."
  }'
```

## Parent Profile Examples

### Update Parent Profile

```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/parent/profile \
  -H "Authorization: your-jwt-token" \
  -H "Content-Type: application/json" \
  -d '{
    "profile_visibility": "public",
    "parent_age": 35,
    "schools": [123, 456]
  }'
```

### Create School Review

```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/parent/reviews \
  -H "Authorization: your-jwt-token" \
  -H "Content-Type: application/json" \
  -d '{
    "school_id": 123,
    "rating": 5,
    "comment": "Excellent school with dedicated teachers!"
  }'
```

### View Public Parents

```bash
curl -X GET "https://api.montessoriworldconnect.com/api/v1/parent/public-parents?page=1&limit=10" \
  -H "Authorization: your-jwt-token"
```

## Subscription Examples

### Get Current Subscription

```bash
curl -X GET https://api.montessoriworldconnect.com/api/v1/subscription \
  -H "Authorization: your-jwt-token"
```

### Create Checkout Session

```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/subscription/checkout \
  -H "Authorization: your-jwt-token" \
  -H "Content-Type: application/json" \
  -d '{
    "plan": "monthly",
    "success_url": "https://yourapp.com/success",
    "cancel_url": "https://yourapp.com/cancel"
  }'
```

### Cancel Subscription

```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/subscription/cancel \
  -H "Authorization: your-jwt-token" \
  -H "Content-Type: application/json" \
  -d '{
    "reason": "Found another solution"
  }'
```

## Notification Examples

### Get Notifications

```bash
curl -X GET https://api.montessoriworldconnect.com/api/v1/notifications \
  -H "Authorization: your-jwt-token"
```

### Mark Notification as Read

```bash
curl -X PUT https://api.montessoriworldconnect.com/api/v1/notifications/123/read \
  -H "Authorization: your-jwt-token"
```

## Messaging Examples

### Send Message

```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/messages \
  -H "Authorization: your-jwt-token" \
  -H "Content-Type: application/json" \
  -d '{
    "recipient_id": 456,
    "content": "Hello, I am interested in your school..."
  }'
```

### Get Messages with User

```bash
curl -X GET https://api.montessoriworldconnect.com/api/v1/messages/456 \
  -H "Authorization: your-jwt-token"
```

## WebSocket Connection

### Connect to WebSocket

```javascript
const token = 'your-jwt-token';
const ws = new WebSocket(`wss://api.montessoriworldconnect.com/wss?token=${token}`);

ws.onopen = () => {
  console.log('Connected to WebSocket');
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Received:', data);
};

ws.onerror = (error) => {
  console.error('WebSocket error:', error);
};

ws.onclose = () => {
  console.log('WebSocket connection closed');
};
```

## JavaScript SDK Example

```javascript
class MontessoriAPI {
  constructor(baseURL, token) {
    this.baseURL = baseURL || 'https://api.montessoriworldconnect.com';
    this.token = token;
  }

  async request(endpoint, options = {}) {
    const url = `${this.baseURL}${endpoint}`;
    const headers = {
      'Content-Type': 'application/json',
      ...options.headers,
    };

    if (this.token) {
      headers['Authorization'] = this.token;
    }

    const response = await fetch(url, {
      ...options,
      headers,
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error || 'Request failed');
    }

    return response.json();
  }

  // Authentication
  async register(userData) {
    return this.request('/api/v1/register', {
      method: 'POST',
      body: JSON.stringify(userData),
    });
  }

  async login(email, password) {
    const response = await this.request('/api/v1/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    });
    this.token = response.token;
    return response;
  }

  // Schools
  async searchSchools(params) {
    const query = new URLSearchParams(params).toString();
    return this.request(`/api/v1/schools/public?${query}`);
  }

  // Subscriptions
  async getSubscription() {
    return this.request('/api/v1/subscription');
  }

  async createCheckout(plan, successUrl, cancelUrl) {
    return this.request('/api/v1/subscription/checkout', {
      method: 'POST',
      body: JSON.stringify({ plan, success_url: successUrl, cancel_url: cancelUrl }),
    });
  }
}

// Usage
const api = new MontessoriAPI();
await api.login('user@example.com', 'password123');
const schools = await api.searchSchools({ city: 'New York', page: 1 });
```
