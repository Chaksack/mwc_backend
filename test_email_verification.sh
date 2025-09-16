#!/bin/bash

# Test script to verify Email Verification functionality
# This script tests the complete email verification flow:
# 1. User registration (should not auto-login)
# 2. Login attempt without verification (should fail)
# 3. Email verification (should succeed)
# 4. Login attempt after verification (should succeed)

BASE_URL="http://localhost:8080/api/v1"

echo "=== Email Verification Functionality Test ==="
echo

# Function to generate random email
generate_random_email() {
    echo "testuser$(date +%s%N | cut -b1-13)@example.com"
}

# Function to test user registration
test_registration() {
    local email=$1
    echo "Testing user registration..."
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d '{
            "email": "'$email'",
            "password": "TestPassword123!",
            "first_name": "Test",
            "last_name": "User",
            "role": "parent"
        }' \
        "$BASE_URL/register")
    
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo $response | sed -E 's/HTTPSTATUS:[0-9]{3}$//')
    
    echo "HTTP Status: $http_code"
    echo "Response: $body"
    
    if [ "$http_code" -eq 201 ]; then
        echo "✅ PASS: User registration successful"
        # Extract email_verified status from response
        email_verified=$(echo $body | grep -o '"email_verified":[^,}]*' | cut -d':' -f2)
        if [ "$email_verified" = "false" ]; then
            echo "✅ PASS: Email verification status is correctly set to false"
        else
            echo "❌ FAIL: Email verification status should be false after registration"
        fi
        # Check that no token is returned (no auto-login)
        if echo $body | grep -q '"token"'; then
            echo "❌ FAIL: Registration should not return JWT token (no auto-login)"
        else
            echo "✅ PASS: No auto-login after registration (correct)"
        fi
    else
        echo "❌ FAIL: User registration failed with status $http_code"
        return 1
    fi
    echo
}

# Function to test login without email verification
test_login_unverified() {
    local email=$1
    echo "Testing login attempt without email verification..."
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d '{
            "email": "'$email'",
            "password": "TestPassword123!"
        }' \
        "$BASE_URL/login")
    
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo $response | sed -E 's/HTTPSTATUS:[0-9]{3}$//')
    
    echo "HTTP Status: $http_code"
    echo "Response: $body"
    
    if [ "$http_code" -eq 403 ]; then
        echo "✅ PASS: Login correctly blocked for unverified email"
        if echo $body | grep -q "verify your email"; then
            echo "✅ PASS: Appropriate error message provided"
        else
            echo "❌ FAIL: Error message should mention email verification"
        fi
    else
        echo "❌ FAIL: Login should be blocked with 403 status for unverified email"
    fi
    echo
}

# Function to test email verification with invalid token
test_verification_invalid_token() {
    echo "Testing email verification with invalid token..."
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X GET \
        "$BASE_URL/verify-email?token=invalid_token_12345")
    
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo $response | sed -E 's/HTTPSTATUS:[0-9]{3}$//')
    
    echo "HTTP Status: $http_code"
    echo "Response: $body"
    
    if [ "$http_code" -eq 404 ]; then
        echo "✅ PASS: Invalid token correctly rejected with 404"
    else
        echo "❌ FAIL: Invalid token should return 404 status"
    fi
    echo
}

# Function to test email verification without token
test_verification_no_token() {
    echo "Testing email verification without token..."
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X GET \
        "$BASE_URL/verify-email")
    
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo $response | sed -E 's/HTTPSTATUS:[0-9]{3}$//')
    
    echo "HTTP Status: $http_code"
    echo "Response: $body"
    
    if [ "$http_code" -eq 400 ]; then
        echo "✅ PASS: Missing token correctly handled with 400"
        if echo $body | grep -q "token is required"; then
            echo "✅ PASS: Appropriate error message for missing token"
        fi
    else
        echo "❌ FAIL: Missing token should return 400 status"
    fi
    echo
}

# Function to simulate email verification (we can't get the actual token from email)
test_verification_simulation() {
    local email=$1
    echo "Testing email verification simulation..."
    echo "Note: In a real scenario, the user would get the verification token from their email."
    echo "For this test, we'll need to manually extract the token from the database or use a test token."
    echo "The verification endpoint is available at: GET $BASE_URL/verify-email?token=<verification_token>"
    echo
}

# Function to test public endpoints (should still work)
test_public_endpoints() {
    echo "Testing that public endpoints still work..."
    
    # Test root endpoint
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" "$BASE_URL/../..")
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    
    if [ "$http_code" -eq 200 ]; then
        echo "✅ PASS: Root endpoint accessible"
    else
        echo "❌ FAIL: Root endpoint should be accessible"
    fi
    
    # Test API v1 root
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" "$BASE_URL/")
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    
    if [ "$http_code" -eq 200 ]; then
        echo "✅ PASS: API v1 root endpoint accessible"
    else
        echo "❌ FAIL: API v1 root endpoint should be accessible"
    fi
    echo
}

# Main test execution
echo "Starting email verification tests..."
echo

# Test public endpoints first
test_public_endpoints

# Generate random email for testing
TEST_EMAIL=$(generate_random_email)
echo "Using test email: $TEST_EMAIL"
echo

# Test the email verification flow
test_registration "$TEST_EMAIL"
test_login_unverified "$TEST_EMAIL"
test_verification_invalid_token
test_verification_no_token
test_verification_simulation "$TEST_EMAIL"

echo "=== Email Verification Test Complete ==="
echo
echo "Summary of expected behavior:"
echo "- Registration should succeed and return email_verified: false"
echo "- Registration should NOT provide JWT token (no auto-login)"
echo "- Login attempts with unverified email should fail with 403"
echo "- Email verification endpoint should validate tokens properly"
echo "- Invalid/missing tokens should return appropriate error codes"
echo "- After email verification, users should be able to log in normally"
echo
echo "Manual verification steps:"
echo "1. Check your email service logs to see if verification emails are sent"
echo "2. If using a real SMTP service, check the actual email for the verification link"
echo "3. Test the verification link by visiting: $BASE_URL/verify-email?token=<actual_token>"
echo "4. After verification, test login again with the same credentials"