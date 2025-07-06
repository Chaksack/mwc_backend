#!/bin/bash

# Set the base URL for the API
BASE_URL="http://localhost:8080/api/v1"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

# Login as educator
echo -e "\n${YELLOW}Testing Educator Login${NC}"
educator_login_response=$(test_request "Educator login" "POST" "/login" "" '{"email":"educator@example.com","password":"Edu123!"}' 200)
EDUCATOR_TOKEN=$(extract_token "$educator_login_response")

if [ -z "$EDUCATOR_TOKEN" ]; then
  echo -e "${RED}Failed to get educator token. Test cannot continue.${NC}"
  exit 1
else
  echo "Educator Token: $EDUCATOR_TOKEN"
fi

# Create a job to apply for (as institution)
echo -e "\n${YELLOW}Testing Institution Login${NC}"
institution_login_response=$(test_request "Institution login" "POST" "/login" "" '{"email":"institution@example.com","password":"Inst123!"}' 200)
INSTITUTION_TOKEN=$(extract_token "$institution_login_response")

if [ -z "$INSTITUTION_TOKEN" ]; then
  echo -e "${RED}Failed to get institution token. Test cannot continue.${NC}"
  exit 1
else
  echo "Institution Token: $INSTITUTION_TOKEN"
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
  echo -e "${RED}Failed to get job ID. Test cannot continue.${NC}"
  exit 1
else
  echo "Job ID: $JOB_ID"
fi

# Test educator job application with file upload
echo -e "\n${BLUE}=== Testing Educator Job Application with File Upload ===${NC}"

# Create a temporary resume file
TEMP_RESUME_FILE="/tmp/test_resume.pdf"
echo "This is a test resume" > "$TEMP_RESUME_FILE"

echo -e "\n${YELLOW}Testing job application with cover letter string and resume file${NC}"
echo "Method: POST"
echo "Endpoint: /educator/jobs/$JOB_ID/apply"
echo "Auth Header: $EDUCATOR_TOKEN"

# Use curl with -F option to send multipart/form-data
response=$(curl -s -w "\n%{http_code}" -X POST \
  "$BASE_URL/educator/jobs/$JOB_ID/apply" \
  -H "Authorization: $EDUCATOR_TOKEN" \
  -F "cover_letter=This is a test cover letter" \
  -F "resume=@$TEMP_RESUME_FILE")

# Extract status code and body
status_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')

# Check status code (expecting 201 Created)
if [ "$status_code" -eq 201 ]; then
  echo -e "${GREEN}✓ Status code matches: $status_code${NC}"
  echo -e "${GREEN}✓ Test passed: Educator can submit a string for cover_letter and file for resume${NC}"
else
  echo -e "${RED}✗ Status code mismatch: Expected 201, got $status_code${NC}"
  echo -e "${RED}✗ Test failed: Educator cannot submit a string for cover_letter and file for resume${NC}"
fi

echo "Response body: $body"

# Clean up the temporary file
rm -f "$TEMP_RESUME_FILE"

# Clean up the job
echo -e "\n${YELLOW}Cleaning up: Deleting the job${NC}"
test_request "Delete job" "DELETE" "/institution/jobs/$JOB_ID" "$INSTITUTION_TOKEN" "" 200

echo -e "\n${GREEN}Test completed${NC}"