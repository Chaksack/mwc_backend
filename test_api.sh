#!/bin/bash

# Test script for API functionality
# This script tests various API endpoints to verify they are functioning properly

# Set the base URL for the API
BASE_URL="http://localhost:8080/api/v1"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Variables to store tokens and IDs
ADMIN_TOKEN=""
INSTITUTION_TOKEN=""
EDUCATOR_TOKEN=""
PARENT_TOKEN=""
SCHOOL_ID=""
JOB_ID=""
REVIEW_ID=""
EVENT_ID=""
BLOG_POST_ID=""

# Function to make a request and check the response
test_request() {
  local description=$1
  local method=$2
  local endpoint=$3
  local auth_header=$4
  local data=$5
  local expected_status=$6

  echo -e "\n${GREEN}Testing: $description${NC}"
  echo "Method: $method"
  echo "Endpoint: $endpoint"

  # Build curl command
  curl_cmd="curl -s -w \"\n%{http_code}\" -X $method \"$BASE_URL$endpoint\""

  # Add auth header if provided
  if [ ! -z "$auth_header" ]; then
    echo "Auth Header: $auth_header"
    curl_cmd="$curl_cmd -H \"Authorization: $auth_header\""
  fi

  # Add data if provided
  if [ ! -z "$data" ]; then
    echo "Data: $data"
    curl_cmd="$curl_cmd -H \"Content-Type: application/json\" -d '$data'"
  fi

  # Execute the curl command
  response=$(eval $curl_cmd)

  # Extract status code and body
  status_code=$(echo "$response" | tail -n1)
  body=$(echo "$response" | sed '$d')

  # Check status code
  if [ "$status_code" -eq "$expected_status" ]; then
    echo -e "${GREEN}✓ Status code matches: $status_code${NC}"
  else
    echo -e "${RED}✗ Status code mismatch: Expected $expected_status, got $status_code${NC}"
  fi

  echo "Response body: $body"

  # Return the response body for further processing if needed
  echo "$body"
}

# Function to extract token from login response
extract_token() {
  local response=$1
  echo "$response" | grep -o '"token":"[^"]*"' | cut -d'"' -f4
}

# Function to test public endpoints
test_public_endpoints() {
  echo -e "\n${BLUE}=== Testing Public Endpoints ===${NC}"

  # Test root endpoint
  test_request "Root endpoint" "GET" "/" "" "" 200

  # Test public schools endpoint
  test_request "Public schools" "GET" "/schools/public" "" "" 200

  # Test public jobs endpoint
  test_request "Public jobs" "GET" "/jobs" "" "" 200

  # Test public events endpoint
  test_request "Public events" "GET" "/events" "" "" 200

  # Test public blog posts endpoint
  test_request "Public blog posts" "GET" "/blog" "" "" 200

  # Test public school reviews endpoint
  # We need a valid school ID for this test
  if [ -z "$SCHOOL_ID" ]; then
    echo -e "${YELLOW}Skipping school reviews test - no school ID available${NC}"
  else
    test_request "Public school reviews" "GET" "/schools/$SCHOOL_ID/reviews" "" "" 200
  fi
}

# Function to test authentication
test_authentication() {
  echo -e "\n${BLUE}=== Testing Authentication ===${NC}"

  # Test login with admin credentials
  echo -e "\n${YELLOW}Testing Admin Login${NC}"
  admin_login_response=$(test_request "Admin login" "POST" "/login" "" '{"email":"admin@example.com","password":"Admin123!"}' 200)
  ADMIN_TOKEN=$(extract_token "$admin_login_response")

  if [ -z "$ADMIN_TOKEN" ]; then
    echo -e "${RED}Failed to get admin token from login response${NC}"
    exit 1
  fi

  echo "Admin Token: $ADMIN_TOKEN"

  # Test login with institution credentials
  echo -e "\n${YELLOW}Testing Institution Login${NC}"
  institution_login_response=$(test_request "Institution login" "POST" "/login" "" '{"email":"institution@example.com","password":"Inst123!"}' 200)
  INSTITUTION_TOKEN=$(extract_token "$institution_login_response")

  if [ -z "$INSTITUTION_TOKEN" ]; then
    echo -e "${YELLOW}Warning: Failed to get institution token. Some tests will be skipped.${NC}"
  else
    echo "Institution Token: $INSTITUTION_TOKEN"
  fi

  # Test login with educator credentials
  echo -e "\n${YELLOW}Testing Educator Login${NC}"
  educator_login_response=$(test_request "Educator login" "POST" "/login" "" '{"email":"educator@example.com","password":"Edu123!"}' 200)
  EDUCATOR_TOKEN=$(extract_token "$educator_login_response")

  if [ -z "$EDUCATOR_TOKEN" ]; then
    echo -e "${YELLOW}Warning: Failed to get educator token. Some tests will be skipped.${NC}"
  else
    echo "Educator Token: $EDUCATOR_TOKEN"
  fi

  # Test login with parent credentials
  echo -e "\n${YELLOW}Testing Parent Login${NC}"
  parent_login_response=$(test_request "Parent login" "POST" "/login" "" '{"email":"parent@example.com","password":"Parent123!"}' 200)
  PARENT_TOKEN=$(extract_token "$parent_login_response")

  if [ -z "$PARENT_TOKEN" ]; then
    echo -e "${YELLOW}Warning: Failed to get parent token. Some tests will be skipped.${NC}"
  else
    echo "Parent Token: $PARENT_TOKEN"
  fi

  # Test get current user with admin token
  test_request "Get current user (admin)" "GET" "/me" "$ADMIN_TOKEN" "" 200
}

# Function to test admin endpoints
test_admin_endpoints() {
  echo -e "\n${BLUE}=== Testing Admin Endpoints ===${NC}"

  if [ -z "$ADMIN_TOKEN" ]; then
    echo -e "${RED}Skipping admin tests - no admin token available${NC}"
    return
  fi

  # Test get all users
  test_request "Get all users" "GET" "/admin/users" "$ADMIN_TOKEN" "" 200

  # Test get schools by country
  test_request "Get schools by country" "GET" "/admin/schools?country_code=US" "$ADMIN_TOKEN" "" 200

  # Test get action logs
  test_request "Get action logs" "GET" "/admin/action-logs" "$ADMIN_TOKEN" "" 200

  # Test get pending reviews
  test_request "Get pending reviews" "GET" "/admin/reviews/pending" "$ADMIN_TOKEN" "" 200
}

# Function to test institution endpoints
test_institution_endpoints() {
  echo -e "\n${BLUE}=== Testing Institution Endpoints ===${NC}"

  if [ -z "$INSTITUTION_TOKEN" ]; then
    echo -e "${RED}Skipping institution tests - no institution token available${NC}"
    return
  fi

  # Test get institution profile
  test_request "Get institution profile" "GET" "/me" "$INSTITUTION_TOKEN" "" 200

  # Test get available schools
  test_request "Get available schools" "GET" "/institution/schools/available" "$INSTITUTION_TOKEN" "" 200

  # Test get institution jobs
  test_request "Get institution jobs" "GET" "/institution/jobs" "$INSTITUTION_TOKEN" "" 200
}

# Function to test educator endpoints
test_educator_endpoints() {
  echo -e "\n${BLUE}=== Testing Educator Endpoints ===${NC}"

  if [ -z "$EDUCATOR_TOKEN" ]; then
    echo -e "${RED}Skipping educator tests - no educator token available${NC}"
    return
  fi

  # Test get educator profile
  test_request "Get educator profile" "GET" "/me" "$EDUCATOR_TOKEN" "" 200

  # Test search schools
  test_request "Search schools" "GET" "/educator/schools/search" "$EDUCATOR_TOKEN" "" 200

  # Test get saved schools
  test_request "Get saved schools" "GET" "/educator/schools/saved" "$EDUCATOR_TOKEN" "" 200

  # Test get applied jobs
  test_request "Get applied jobs" "GET" "/educator/jobs/applied" "$EDUCATOR_TOKEN" "" 200
}

# Function to test parent endpoints
test_parent_endpoints() {
  echo -e "\n${BLUE}=== Testing Parent Endpoints ===${NC}"

  if [ -z "$PARENT_TOKEN" ]; then
    echo -e "${RED}Skipping parent tests - no parent token available${NC}"
    return
  fi

  # Test get parent profile
  test_request "Get parent profile" "GET" "/me" "$PARENT_TOKEN" "" 200

  # Test search schools
  test_request "Search schools" "GET" "/parent/schools/search" "$PARENT_TOKEN" "" 200

  # Test get saved schools
  test_request "Get saved schools" "GET" "/parent/schools/saved" "$PARENT_TOKEN" "" 200

  # Test get messages
  test_request "Get messages" "GET" "/parent/messages" "$PARENT_TOKEN" "" 200
}

# Function to test error handling
test_error_handling() {
  echo -e "\n${BLUE}=== Testing Error Handling ===${NC}"

  # Test invalid token
  test_request "Invalid token" "GET" "/me" "invalid.token.here" "" 401

  # Test missing token
  test_request "Missing token" "GET" "/me" "" "" 401

  # Test access denied (try to access admin endpoint as non-admin)
  if [ ! -z "$PARENT_TOKEN" ]; then
    test_request "Access denied (parent trying to access admin endpoint)" "GET" "/admin/users" "$PARENT_TOKEN" "" 403
  elif [ ! -z "$EDUCATOR_TOKEN" ]; then
    test_request "Access denied (educator trying to access admin endpoint)" "GET" "/admin/users" "$EDUCATOR_TOKEN" "" 403
  elif [ ! -z "$INSTITUTION_TOKEN" ]; then
    test_request "Access denied (institution trying to access admin endpoint)" "GET" "/admin/users" "$INSTITUTION_TOKEN" "" 403
  fi

  # Test resource not found
  test_request "Resource not found" "GET" "/schools/999999" "$ADMIN_TOKEN" "" 404
}

# Function to test CRUD operations for schools
test_school_crud() {
  echo -e "\n${BLUE}=== Testing School CRUD Operations ===${NC}"

  if [ -z "$ADMIN_TOKEN" ]; then
    echo -e "${RED}Skipping school CRUD tests - no admin token available${NC}"
    return
  fi

  # Create a school
  echo -e "\n${YELLOW}Creating a school${NC}"
  create_school_response=$(test_request "Create school" "POST" "/admin/schools" "$ADMIN_TOKEN" '{
    "name": "Test Montessori School",
    "address": "123 Test St",
    "city": "Test City",
    "state": "TS",
    "zipCode": "12345",
    "countryCode": "US",
    "phone": "555-123-4567",
    "email": "test@school.com",
    "website": "https://testschool.com",
    "description": "A test school created by the API test script"
  }' 201)

  # Extract school ID from response
  SCHOOL_ID=$(echo "$create_school_response" | grep -o '"id":[0-9]*' | head -1 | cut -d':' -f2)

  if [ -z "$SCHOOL_ID" ]; then
    echo -e "${YELLOW}Warning: Failed to get school ID. Some tests will be skipped.${NC}"
  else
    echo "School ID: $SCHOOL_ID"

    # Read the school
    test_request "Get school" "GET" "/admin/schools/$SCHOOL_ID" "$ADMIN_TOKEN" "" 200

    # Update the school
    test_request "Update school" "PUT" "/admin/schools/$SCHOOL_ID" "$ADMIN_TOKEN" '{
      "name": "Updated Test Montessori School",
      "description": "An updated test school"
    }' 200

    # Delete the school
    test_request "Delete school" "DELETE" "/admin/schools/$SCHOOL_ID" "$ADMIN_TOKEN" "" 200
  fi
}

# Function to test CRUD operations for jobs
test_job_crud() {
  echo -e "\n${BLUE}=== Testing Job CRUD Operations ===${NC}"

  if [ -z "$INSTITUTION_TOKEN" ]; then
    echo -e "${RED}Skipping job CRUD tests - no institution token available${NC}"
    return
  fi

  # Create a job
  echo -e "\n${YELLOW}Creating a job${NC}"
  create_job_response=$(test_request "Create job" "POST" "/institution/jobs" "$INSTITUTION_TOKEN" '{
    "title": "Test Teacher Position",
    "description": "A test job created by the API test script",
    "requirements": "Testing experience",
    "location": "Test City, TS",
    "salary": "Competitive",
    "isRemote": false,
    "isActive": true
  }' 201)

  # Extract job ID from response
  JOB_ID=$(echo "$create_job_response" | grep -o '"id":[0-9]*' | head -1 | cut -d':' -f2)

  if [ -z "$JOB_ID" ]; then
    echo -e "${YELLOW}Warning: Failed to get job ID. Some tests will be skipped.${NC}"
  else
    echo "Job ID: $JOB_ID"

    # Read the job (through the public endpoint)
    test_request "Get job (public)" "GET" "/jobs" "" "" 200

    # Update the job
    test_request "Update job" "PUT" "/institution/jobs/$JOB_ID" "$INSTITUTION_TOKEN" '{
      "title": "Updated Test Teacher Position",
      "description": "An updated test job"
    }' 200

    # Delete the job
    test_request "Delete job" "DELETE" "/institution/jobs/$JOB_ID" "$INSTITUTION_TOKEN" "" 200
  fi
}

# Function to test CRUD operations for reviews
test_review_crud() {
  echo -e "\n${BLUE}=== Testing Review CRUD Operations ===${NC}"

  if [ -z "$PARENT_TOKEN" ] || [ -z "$SCHOOL_ID" ]; then
    echo -e "${RED}Skipping review CRUD tests - missing parent token or school ID${NC}"
    return
  fi

  # Create a review
  echo -e "\n${YELLOW}Creating a review${NC}"
  create_review_response=$(test_request "Create review" "POST" "/reviews" "$PARENT_TOKEN" '{
    "schoolId": '$SCHOOL_ID',
    "rating": 5,
    "title": "Test Review",
    "content": "This is a test review created by the API test script",
    "pros": "Great testing environment",
    "cons": "None"
  }' 201)

  # Extract review ID from response
  REVIEW_ID=$(echo "$create_review_response" | grep -o '"id":[0-9]*' | head -1 | cut -d':' -f2)

  if [ -z "$REVIEW_ID" ]; then
    echo -e "${YELLOW}Warning: Failed to get review ID. Some tests will be skipped.${NC}"
  else
    echo "Review ID: $REVIEW_ID"

    # Read the user's reviews
    test_request "Get user reviews" "GET" "/reviews/user" "$PARENT_TOKEN" "" 200

    # Update the review
    test_request "Update review" "PUT" "/reviews/$REVIEW_ID" "$PARENT_TOKEN" '{
      "rating": 4,
      "title": "Updated Test Review",
      "content": "This is an updated test review"
    }' 200

    # Delete the review
    test_request "Delete review" "DELETE" "/reviews/$REVIEW_ID" "$PARENT_TOKEN" "" 200
  fi
}

# Function to test CRUD operations for events
test_event_crud() {
  echo -e "\n${BLUE}=== Testing Event CRUD Operations ===${NC}"

  if [ -z "$INSTITUTION_TOKEN" ]; then
    echo -e "${RED}Skipping event CRUD tests - no institution token available${NC}"
    return
  fi

  # Create an event
  echo -e "\n${YELLOW}Creating an event${NC}"
  create_event_response=$(test_request "Create event" "POST" "/institution/events" "$INSTITUTION_TOKEN" '{
    "title": "Test Open House",
    "description": "A test event created by the API test script",
    "startDate": "'$(date -d "+1 day" +"%Y-%m-%d")'T10:00:00Z",
    "endDate": "'$(date -d "+1 day" +"%Y-%m-%d")'T12:00:00Z",
    "location": "Test Location",
    "isVirtual": false,
    "registrationUrl": "https://example.com/register"
  }' 201)

  # Extract event ID from response
  EVENT_ID=$(echo "$create_event_response" | grep -o '"id":[0-9]*' | head -1 | cut -d':' -f2)

  if [ -z "$EVENT_ID" ]; then
    echo -e "${YELLOW}Warning: Failed to get event ID. Some tests will be skipped.${NC}"
  else
    echo "Event ID: $EVENT_ID"

    # Read the event (through the public endpoint)
    test_request "Get event (public)" "GET" "/events/$EVENT_ID" "" "" 200

    # Update the event
    test_request "Update event" "PUT" "/institution/events/$EVENT_ID" "$INSTITUTION_TOKEN" '{
      "title": "Updated Test Open House",
      "description": "An updated test event"
    }' 200

    # Delete the event
    test_request "Delete event" "DELETE" "/institution/events/$EVENT_ID" "$INSTITUTION_TOKEN" "" 200
  fi
}

# Function to test CRUD operations for blog posts
test_blog_crud() {
  echo -e "\n${BLUE}=== Testing Blog CRUD Operations ===${NC}"

  if [ -z "$ADMIN_TOKEN" ]; then
    echo -e "${RED}Skipping blog CRUD tests - no admin token available${NC}"
    return
  fi

  # Create a blog post
  echo -e "\n${YELLOW}Creating a blog post${NC}"
  create_blog_response=$(test_request "Create blog post" "POST" "/admin/blog" "$ADMIN_TOKEN" '{
    "title": "Test Blog Post",
    "content": "This is a test blog post created by the API test script",
    "excerpt": "A test blog post",
    "slug": "test-blog-post-'$(date +%s)'",
    "categories": ["Test"],
    "tags": ["test", "api"]
  }' 201)

  # Extract blog post ID from response
  BLOG_POST_ID=$(echo "$create_blog_response" | grep -o '"id":[0-9]*' | head -1 | cut -d':' -f2)

  if [ -z "$BLOG_POST_ID" ]; then
    echo -e "${YELLOW}Warning: Failed to get blog post ID. Some tests will be skipped.${NC}"
  else
    echo "Blog Post ID: $BLOG_POST_ID"

    # Read the blog post (through the public endpoint)
    test_request "Get blog posts (public)" "GET" "/blog" "" "" 200

    # Update the blog post
    test_request "Update blog post" "PUT" "/admin/blog/$BLOG_POST_ID" "$ADMIN_TOKEN" '{
      "title": "Updated Test Blog Post",
      "content": "This is an updated test blog post"
    }' 200

    # Delete the blog post
    test_request "Delete blog post" "DELETE" "/admin/blog/$BLOG_POST_ID" "$ADMIN_TOKEN" "" 200
  fi
}

# Function to generate a test summary
generate_test_summary() {
  echo -e "\n${BLUE}=== Test Summary ===${NC}"

  echo -e "${GREEN}Public Endpoints:${NC} Tested"
  echo -e "${GREEN}Authentication:${NC} Tested"

  if [ -z "$ADMIN_TOKEN" ]; then
    echo -e "${RED}Admin Endpoints:${NC} Not tested (no admin token)"
  else
    echo -e "${GREEN}Admin Endpoints:${NC} Tested"
  fi

  if [ -z "$INSTITUTION_TOKEN" ]; then
    echo -e "${RED}Institution Endpoints:${NC} Not tested (no institution token)"
  else
    echo -e "${GREEN}Institution Endpoints:${NC} Tested"
  fi

  if [ -z "$EDUCATOR_TOKEN" ]; then
    echo -e "${RED}Educator Endpoints:${NC} Not tested (no educator token)"
  else
    echo -e "${GREEN}Educator Endpoints:${NC} Tested"
  fi

  if [ -z "$PARENT_TOKEN" ]; then
    echo -e "${RED}Parent Endpoints:${NC} Not tested (no parent token)"
  else
    echo -e "${GREEN}Parent Endpoints:${NC} Tested"
  fi

  echo -e "${GREEN}Error Handling:${NC} Tested"

  if [ -z "$SCHOOL_ID" ]; then
    echo -e "${RED}School CRUD:${NC} Not tested (no school ID)"
  else
    echo -e "${GREEN}School CRUD:${NC} Tested"
  fi

  if [ -z "$JOB_ID" ]; then
    echo -e "${RED}Job CRUD:${NC} Not tested (no job ID)"
  else
    echo -e "${GREEN}Job CRUD:${NC} Tested"
  fi

  if [ -z "$REVIEW_ID" ]; then
    echo -e "${RED}Review CRUD:${NC} Not tested (no review ID)"
  else
    echo -e "${GREEN}Review CRUD:${NC} Tested"
  fi

  if [ -z "$EVENT_ID" ]; then
    echo -e "${RED}Event CRUD:${NC} Not tested (no event ID)"
  else
    echo -e "${GREEN}Event CRUD:${NC} Tested"
  fi

  if [ -z "$BLOG_POST_ID" ]; then
    echo -e "${RED}Blog CRUD:${NC} Not tested (no blog post ID)"
  else
    echo -e "${GREEN}Blog CRUD:${NC} Tested"
  fi
}

# Main function to run all tests
run_all_tests() {
  echo -e "${GREEN}Starting API Tests${NC}"

  # Run all test functions
  test_public_endpoints
  test_authentication
  test_admin_endpoints
  test_institution_endpoints
  test_educator_endpoints
  test_parent_endpoints
  test_error_handling

  # Run CRUD tests
  test_school_crud
  test_job_crud
  test_review_crud
  test_event_crud
  test_blog_crud

  # Generate test summary
  generate_test_summary

  echo -e "\n${GREEN}All tests completed${NC}"
}

# Run all tests
run_all_tests
