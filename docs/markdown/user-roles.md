# User Roles & Permissions

The Montessori World Connect platform supports multiple user roles, each with specific capabilities and access levels.

## Role Types

### 🏫 Institution
Schools and educational institutions offering Montessori programs.

**Capabilities:**
- Create and manage institution profile
- Upload profile pictures and verification documents
- Post job openings
- View and manage job applications
- Associate with schools in the database
- Receive notifications for applications
- Access subscription features

**Required Fields on Registration:**
- `institution_name` - Name of the institution (required)

### 🎓 Training Center
Organizations providing Montessori teacher training and certification.

**Capabilities:**
- Same as Institution role
- Can be distinguished by school category
- Specialized for training purposes

**Required Fields on Registration:**
- `institution_name` - Name of the training center (required)

### 👨‍🏫 Montessori Professional
Educators, teachers, and Montessori-trained professionals.

**Capabilities:**
- Create comprehensive professional profile
- Search and filter schools
- Save favorite schools
- Apply for job postings
- Set job preferences (location, job type, etc.)
- Toggle "looking for job" status
- Contact institutions directly
- Receive job opportunity notifications

**Profile Fields:**
- Bio
- Qualifications
- Experience
- Job preferences
- Availability status

### 👪 Parent
Parents seeking Montessori education for their children.

**Capabilities:**
- Search for Montessori schools
- View school details (requires active subscription)
- Create and manage reviews
- Save favorite schools
- Set profile visibility (public/private)
- View other public parent profiles
- Share schools their children attend

**Profile Fields:**
- Age
- Profile visibility setting
- Children's schools
- Saved schools

### 🛠️ Admin
Administrators with elevated permissions for content moderation.

**Capabilities:**
- Moderate reviews and content
- Manage user accounts
- Manually create schools
- Batch upload schools from CSV
- View system action logs
- Create and manage blog posts
- Create and manage events
- Update user roles and status

**Access Restrictions:**
- Can only be created by Super Admin
- Cannot self-register

### 👑 Super Admin
Highest level administrators with full system access.

**Capabilities:**
- All Admin capabilities, plus:
- Create and manage admin accounts
- Manage subscription plans
- Assign subscriptions to users
- Access all system features without restrictions
- View all logs and analytics

**Access Restrictions:**
- Cannot be created through API
- Created through system initialization or database

## Role-Based Endpoint Access

### Public Endpoints (No Authentication)
Anyone can access these endpoints without authentication:
- Registration and login
- Public school search
- Institution search
- Job listings
- Events and blogs

### Authenticated Endpoints
Require valid JWT token:
- Profile management
- Notifications
- Messages
- Saved schools

### Role-Specific Endpoints

#### Institution/Training Center Only:
- `POST /api/v1/institution/profile`
- `POST /api/v1/institution/jobs`
- `GET /api/v1/institution/jobs/:job_id/applicants`

#### Montessori Professional Only:
- `POST /api/v1/montessori-professional/profile`
- `GET /api/v1/montessori-professional/schools/search`
- `POST /api/v1/montessori-professional/jobs/apply/:job_id`

#### Parent Only:
- `POST /api/v1/parent/profile`
- `GET /api/v1/parent/schools/:school_id/details`
- `POST /api/v1/parent/reviews`

#### Admin Only:
- `POST /api/v1/admin/schools/batch-upload`
- `POST /api/v1/admin/schools/create`
- `GET /api/v1/admin/action-logs`
- `POST /api/v1/admin/blogs`

#### Super Admin Only:
- `POST /api/v1/admin/admins`
- `POST /api/v1/admin/subscription-plans`
- `PUT /api/v1/admin/users/:id/role`

## Changing User Roles

Only Super Admins can change user roles using:

```
PUT /api/v1/admin/users/:id/role
Authorization: <super-admin-token>
Content-Type: application/json

{
  "role": "admin"
}
```

## Role Verification

The API automatically verifies role permissions for protected endpoints. If a user attempts to access an endpoint they don't have permission for, they'll receive a `403 Forbidden` response.
