# Advanced Features

This guide covers advanced features and integrations available in the Montessori World Connect API.

## File Uploads

### Profile Pictures

Upload profile pictures for institutions and users using multipart/form-data.

**Institution Profile Picture:**
```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/institution/profile \
  -H "Authorization: your-jwt-token" \
  -F "institution_name=Bright Future Montessori" \
  -F "profile_picture=@/path/to/image.jpg" \
  -F "school_city=New York"
```

**Supported Formats:**
- JPEG (.jpg, .jpeg)
- PNG (.png)
- GIF (.gif)
- Max size: 10MB

**User Profile Picture:**
```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/me/profile-picture \
  -H "Authorization: your-jwt-token" \
  -F "profile_picture=@/path/to/image.jpg"
```

### Verification Documents

Institutions can upload verification documents during profile creation:

```bash
curl -X POST https://api.montessoriworldconnect.com/api/v1/institution/profile \
  -H "Authorization: your-jwt-token" \
  -F "verification_docs=@/path/to/document.pdf" \
  -F "institution_name=Bright Future Montessori"
```

## Search & Filtering

### Advanced School Search

The school search endpoint supports multiple filters for precise results:

```bash
GET /api/v1/schools/public?
  name=montessori&
  city=New+York&
  country_code=US&
  category=school&
  page=1&
  limit=20
```

**Available Filters:**
- `name` - School name (partial match)
- `city` - City name (partial match)
- `country_code` - ISO country code (exact match)
- `category` - "school" or "training_center"
- `page` - Page number
- `limit` - Results per page (max 100)

### Institution Realtime Search

Search institutions with advanced filtering:

```bash
GET /api/v1/institutions/search?
  q=bright&
  city=New+York&
  country_code=US&
  verified=true&
  category=school&
  page=1&
  limit=10
```

**Available Filters:**
- `q` - Search query (searches institution name and school name)
- `city` - Filter by city
- `country_code` - Filter by country
- `verified` - Filter by verification status (true/false)
- `category` - Filter by category
- `page` - Page number
- `limit` - Results per page

## Messaging System

### Send Direct Message

```bash
POST /api/v1/messages
Authorization: your-jwt-token
Content-Type: application/json

{
  "recipient_id": 456,
  "content": "Hello, I'm interested in your school..."
}
```

### Retrieve Conversation

```bash
GET /api/v1/messages/456?page=1&limit=50
Authorization: your-jwt-token
```

### Mark Messages as Read

```bash
PUT /api/v1/messages/123/read
Authorization: your-jwt-token
```

### Get Unread Count

```bash
GET /api/v1/messages/unread-count
Authorization: your-jwt-token
```

## Real-Time Notifications

### WebSocket Connection

Connect to receive real-time updates:

```javascript
const token = 'your-jwt-token';
const ws = new WebSocket(`wss://api.montessoriworldconnect.com/wss?token=${token}`);

ws.onopen = () => {
  console.log('Connected');
};

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  
  switch(message.type) {
    case 'new_message':
      handleNewMessage(message.data);
      break;
    case 'notification':
      handleNotification(message.data);
      break;
    case 'job_application':
      handleJobApplication(message.data);
      break;
  }
};

// Send heartbeat every 30 seconds
setInterval(() => {
  if (ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({ type: 'ping' }));
  }
}, 30000);
```

### In-App Notifications

```bash
# Get all notifications
GET /api/v1/notifications?page=1&limit=20
Authorization: your-jwt-token

# Mark as read
PUT /api/v1/notifications/123/read
Authorization: your-jwt-token

# Mark all as read
PUT /api/v1/notifications/mark-all-read
Authorization: your-jwt-token
```

## Review System

### Create School Review

```bash
POST /api/v1/parent/reviews
Authorization: your-jwt-token
Content-Type: application/json

{
  "school_id": 123,
  "rating": 5,
  "comment": "Excellent school with dedicated teachers!",
  "recommend": true
}
```

**Validation:**
- Rating: 1-5 (required)
- Comment: min 10 characters (required)
- Recommend: boolean (optional)

### Get School Reviews

```bash
GET /api/v1/schools/123/reviews?page=1&limit=10&status=approved
```

**Status Filters:**
- `pending` - Awaiting moderation
- `approved` - Published reviews
- `rejected` - Rejected reviews

### Moderate Reviews (Admin Only)

```bash
PUT /api/v1/admin/reviews/456/moderate
Authorization: admin-token
Content-Type: application/json

{
  "status": "approved",
  "admin_comment": "Review meets guidelines"
}
```

## Saved Schools

### Save a School

```bash
POST /api/v1/me/schools/saved/123
Authorization: your-jwt-token
```

### Remove Saved School

```bash
DELETE /api/v1/me/schools/saved/123
Authorization: your-jwt-token
```

### Get Saved Schools

```bash
GET /api/v1/me/schools/saved?page=1&limit=20
Authorization: your-jwt-token
```

## Job Management

### Post Job (Institution)

```bash
POST /api/v1/institution/jobs
Authorization: your-jwt-token
Content-Type: application/json

{
  "title": "Lead Montessori Teacher",
  "description": "We are seeking an experienced AMI-certified teacher...",
  "location": "New York, NY",
  "job_type": "full-time",
  "salary_min": 45000,
  "salary_max": 65000,
  "requirements": "AMI/AMS certification, 3+ years experience",
  "benefits": "Health insurance, 401k, professional development"
}
```

### Update Job

```bash
PUT /api/v1/institution/jobs/789
Authorization: your-jwt-token
Content-Type: application/json

{
  "title": "Updated Title",
  "description": "Updated description"
}
```

### Get Job Applicants

```bash
GET /api/v1/institution/jobs/789/applicants?page=1&limit=20
Authorization: your-jwt-token
```

### Apply for Job (Professional)

```bash
POST /api/v1/montessori-professional/jobs/apply/789
Authorization: your-jwt-token
Content-Type: application/json

{
  "cover_letter": "I am very interested in this position because..."
}
```

### Set Job Preferences

```bash
POST /api/v1/montessori-professional/job-preferences
Authorization: your-jwt-token
Content-Type: application/json

{
  "preferred_location": "New York, NY",
  "job_type": "full-time",
  "salary_expectation_min": 50000,
  "salary_expectation_max": 70000,
  "available_from": "2025-01-01"
}
```

## Parent Features

### View Public Parents

```bash
GET /api/v1/parent/public-parents?page=1&limit=20
Authorization: your-jwt-token
```

### Get Parent Details

```bash
GET /api/v1/parent/public-parents/456
Authorization: your-jwt-token
```

### Update Profile Visibility

```bash
POST /api/v1/parent/profile
Authorization: your-jwt-token
Content-Type: application/json

{
  "profile_visibility": "public",
  "parent_age": 35
}
```

## Admin Features

### Batch Upload Schools

```bash
POST /api/v1/admin/schools/batch-upload
Authorization: admin-token
Content-Type: multipart/form-data

file=@schools.csv
```

**CSV Format:**
```csv
name,address,city,state,country,country_code,zip_code,phone,website,email,latitude,longitude,category
"ABC Montessori","123 Main St","New York","NY","United States","US","10001","555-1234","https://abc.com","info@abc.com",40.7128,-74.0060,"school"
```

### Create Blog Post

```bash
POST /api/v1/admin/blogs
Authorization: admin-token
Content-Type: multipart/form-data

# Required text fields
title=The Benefits of Montessori Education
slug=benefits-of-montessori-education
content=Lorem ipsum...

# Optional fields
summary=Short excerpt...
tags=["education","montessori","children"]
is_published=true
is_featured=false

# Optional YouTube video (youtube.com or youtu.be)
youtube_url=https://youtu.be/dQw4w9WgXcQ

# Upload one or more images (the backend stores them in S3 when configured)
images=@header.jpg
images=@gallery1.png
images=@gallery2.webp

# Select which uploaded image is used as thumbnail/header by index (0-based)
thumbnail_index=1
header_index=0
```

### Upload Blog Images (append)

```bash
POST /api/v1/admin/blogs/{id}/images
Authorization: admin-token
Content-Type: multipart/form-data

images=@new1.jpg
images=@new2.png

# If the blog doesn't already have selections, you must set them:
thumbnail_index=0
header_index=1

# Optional: replace existing images instead of appending
replace_images=true
```

### Upload Inline Content Image (WYSIWYG)

Use this when your word editor needs to upload an image and immediately insert it into the `content` HTML.

```bash
POST /api/v1/admin/blogs/content-images
Authorization: admin-token
Content-Type: multipart/form-data

image=@inline.png
alt=Montessori classroom
```

**Response (example):**

```json
{
  "url": "https://cdn.example.com/uploads/blogs/content-images/1_...png",
  "html": "<img src=\"https://cdn.example.com/uploads/blogs/content-images/1_...png\" alt=\"Montessori classroom\" loading=\"lazy\" />"
}
```

If you already have hosted image URLs (or want to use the legacy single field), JSON is also supported:

### Generate Inline YouTube Embed (WYSIWYG)

Use this when your word editor has a “Insert YouTube video” action and you want the backend to generate a safe, allowlisted `<iframe>` snippet.

```bash
POST /api/v1/admin/blogs/content-youtube
Authorization: admin-token
Content-Type: application/json

{
  "youtube_url": "https://youtu.be/dQw4w9WgXcQ",
  "width": 560,
  "height": 315,
  "title": "Classroom tour"
}
```

**Response (example):**

```json
{
  "watch_url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
  "embed_url": "https://www.youtube.com/embed/dQw4w9WgXcQ",
  "html": "<iframe width=\"560\" height=\"315\" src=\"https://www.youtube.com/embed/dQw4w9WgXcQ\" title=\"Classroom tour\" frameborder=\"0\" allow=\"accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share\" allowfullscreen></iframe>"
}
```

```json
{
  "title": "The Benefits of Montessori Education",
  "slug": "benefits-of-montessori-education",
  "content": "<h2>Why Montessori works</h2><p>This content can come from a word editor/WYSIWYG (HTML).</p><p><img src=\"https://cdn.example.com/blogs/gallery1.jpg\" alt=\"Classroom\" /></p><p>Video:</p><iframe width=\"560\" height=\"315\" src=\"https://www.youtube.com/embed/dQw4w9WgXcQ\" title=\"YouTube video player\" frameborder=\"0\" allow=\"accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share\" allowfullscreen></iframe>",
  "tags": ["education", "montessori", "children"],
  "images": [
    "https://cdn.example.com/blogs/header.jpg",
    "https://cdn.example.com/blogs/thumb.jpg"
  ],
  "thumbnail_image": "https://cdn.example.com/blogs/thumb.jpg",
  "header_image": "https://cdn.example.com/blogs/header.jpg",
  "youtube_url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",

  "is_published": true,
  "is_featured": false
}
```

Note: the backend sanitizes editor HTML. It allows safe formatting tags, inline `<img>` (http/https), and YouTube embeds via `<iframe src="https://www.youtube.com/embed/<id>">`.

### Create Event

```bash
POST /api/v1/admin/events
Authorization: admin-token
Content-Type: application/json

{
  "title": "Montessori Conference 2025",
  "description": "Annual conference for educators",
  "location": "New York, NY",
  "start_date": "2025-06-01T09:00:00Z",
  "end_date": "2025-06-03T17:00:00Z",
  "registration_url": "https://conference.com"
}
```

### View Action Logs

```bash
GET /api/v1/admin/action-logs?
  page=1&
  limit=50&
  action_type=USER_LOGIN&
  user_id=123&
  start_date=2025-01-01&
  end_date=2025-12-31
Authorization: admin-token
```

## Continent & Location Features

### Get School Counts by Continent

```bash
GET /api/v1/schools/continent-counts
```

**Response:**
```json
{
  "Africa": 150,
  "Asia": 320,
  "Europe": 450,
  "North America": 890,
  "South America": 120,
  "Oceania": 80
}
```

### List Continents

```bash
GET /api/v1/schools/continents
```

## Error Handling Best Practices

### Retry Logic

```javascript
async function apiCallWithRetry(url, options, maxRetries = 3) {
  for (let i = 0; i < maxRetries; i++) {
    try {
      const response = await fetch(url, options);
      
      if (response.status === 429) {
        // Rate limited - wait and retry
        const retryAfter = response.headers.get('Retry-After') || Math.pow(2, i);
        await new Promise(resolve => setTimeout(resolve, retryAfter * 1000));
        continue;
      }
      
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      
      return await response.json();
    } catch (error) {
      if (i === maxRetries - 1) throw error;
      await new Promise(resolve => setTimeout(resolve, Math.pow(2, i) * 1000));
    }
  }
}
```

### Token Refresh

```javascript
class APIClient {
  constructor() {
    this.token = localStorage.getItem('token');
    this.tokenExpiry = localStorage.getItem('tokenExpiry');
  }

  async request(endpoint, options = {}) {
    // Check if token is expired
    if (this.isTokenExpired()) {
      await this.refreshToken();
    }

    const response = await fetch(endpoint, {
      ...options,
      headers: {
        ...options.headers,
        'Authorization': this.token,
      },
    });

    if (response.status === 401) {
      // Token invalid - redirect to login
      window.location.href = '/login';
      return;
    }

    return response.json();
  }

  isTokenExpired() {
    if (!this.tokenExpiry) return true;
    return Date.now() > parseInt(this.tokenExpiry);
  }
}
```
