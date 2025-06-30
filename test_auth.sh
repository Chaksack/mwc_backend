#!/bin/bash

# Test script for authentication middleware
# This script tests various authentication scenarios to verify the error messages

# Set the base URL for the API
BASE_URL="http://localhost:8080/api/v1"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Function to make a request and check the response
test_request() {
  local description=$1
  local endpoint=$2
  local auth_header=$3
  local expected_status=$4
  local expected_error=$5

  echo -e "\n${GREEN}Testing: $description${NC}"
  echo "Endpoint: $endpoint"
  echo "Auth Header: $auth_header"
  
  # Make the request
  if [ -z "$auth_header" ]; then
    response=$(curl -s -w "\n%{http_code}" "$BASE_URL$endpoint")
  else
    response=$(curl -s -w "\n%{http_code}" -H "Authorization: $auth_header" "$BASE_URL$endpoint")
  fi
  
  # Extract status code and body
  status_code=$(echo "$response" | tail -n1)
  body=$(echo "$response" | sed '$d')
  
  # Check status code
  if [ "$status_code" -eq "$expected_status" ]; then
    echo -e "${GREEN}✓ Status code matches: $status_code${NC}"
  else
    echo -e "${RED}✗ Status code mismatch: Expected $expected_status, got $status_code${NC}"
  fi
  
  # Check error message if expected
  if [ ! -z "$expected_error" ]; then
    if echo "$body" | grep -q "$expected_error"; then
      echo -e "${GREEN}✓ Error message contains: $expected_error${NC}"
    else
      echo -e "${RED}✗ Error message does not contain expected text${NC}"
      echo "Response body: $body"
    fi
  fi
  
  echo "Response body: $body"
}

# Test 1: No Authorization header
test_request "No Authorization header" "/admin/users" "" 401 "Missing Authorization header"

# Test 2: Malformed Authorization header (no Bearer)
test_request "Malformed Authorization header (no Bearer)" "/admin/users" "token123" 401 "Malformed Authorization header"

# Test 3: Empty token
test_request "Empty token" "/admin/users" "Bearer " 401 "Empty JWT token"

# Test 4: Invalid token
test_request "Invalid token" "/admin/users" "Bearer invalid.token.here" 401 "Invalid JWT token"

# Test 5: Login to get a valid token
echo -e "\n${GREEN}Logging in to get a valid token${NC}"
login_response=$(curl -s -X POST "$BASE_URL/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"Admin123!"}')

echo "Login response: $login_response"

# Extract token from login response
token=$(echo "$login_response" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

if [ -z "$token" ]; then
  echo -e "${RED}Failed to get token from login response${NC}"
  exit 1
fi

echo "Token: $token"

# Test 6: Valid token
test_request "Valid token" "/admin/users" "Bearer $token" 200 ""

echo -e "\n${GREEN}All tests completed${NC}"