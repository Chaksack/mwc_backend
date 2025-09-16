#!/bin/bash

# Test script to verify Forgot Password functionality
# This script tests the complete forgot password flow:
# 1. User requests password reset (forgot-password)
# 2. System sends reset email with token
# 3. User resets password using token (reset-password)
# 4. User can log in with new password

BASE_URL="http://localhost:8080/api/v1"

echo "=== Forgot Password Functionality Test ==="
echo

# Function to generate random email
generate_random_email() {
    echo "testuser$(date +%s%N | cut -b1-13)@example.com"
}

# Function to test user registration (setup)
test_registration() {
    local email=$1
    echo "Setting up test user..."
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d '{
            "email": "'$email'",
            "password": "OriginalPassword123!",
            "first_name": "Test",
            "last_name": "User",
            "role": "parent"
        }' \
        "$BASE_URL/register")
    
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo $response | sed -E 's/HTTPSTATUS:[0-9]{3}$//')
    
    echo "Registration HTTP Status: $http_code"
    
    if [ "$http_code" -eq 201 ]; then
        echo "✅ PASS: Test user registered successfully"
        return 0
    else
        echo "❌ FAIL: Test user registration failed with status $http_code"
        echo "Response: $body"
        return 1
    fi
    echo
}

# Function to test forgot password request
test_forgot_password_valid() {
    local email=$1
    echo "Testing forgot password with valid email..."
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d '{
            "email": "'$email'"
        }' \
        "$BASE_URL/forgot-password")
    
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo $response | sed -E 's/HTTPSTATUS:[0-9]{3}$//')
    
    echo "HTTP Status: $http_code"
    echo "Response: $body"
    
    if [ "$http_code" -eq 200 ]; then
        echo "✅ PASS: Forgot password request accepted"
        if echo $body | grep -q "password reset link"; then
            echo "✅ PASS: Appropriate success message returned"
        else
            echo "❌ FAIL: Success message should mention reset link"
        fi
    else
        echo "❌ FAIL: Forgot password should return 200 status"
    fi
    echo
}

# Function to test forgot password with invalid email
test_forgot_password_invalid() {
    echo "Testing forgot password with non-existent email..."
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d '{
            "email": "nonexistent@example.com"
        }' \
        "$BASE_URL/forgot-password")
    
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo $response | sed -E 's/HTTPSTATUS:[0-9]{3}$//')
    
    echo "HTTP Status: $http_code"
    echo "Response: $body"
    
    if [ "$http_code" -eq 200 ]; then
        echo "✅ PASS: Security maintained - same response for non-existent email"
        if echo $body | grep -q "password reset link"; then
            echo "✅ PASS: Generic message doesn't reveal email existence"
        fi
    else
        echo "❌ FAIL: Should return 200 to avoid email enumeration"
    fi
    echo
}

# Function to test forgot password with invalid JSON
test_forgot_password_invalid_json() {
    echo "Testing forgot password with invalid request format..."
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d '{
            "invalid": "data"
        }' \
        "$BASE_URL/forgot-password")
    
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo $response | sed -E 's/HTTPSTATUS:[0-9]{3}$//')
    
    echo "HTTP Status: $http_code"
    echo "Response: $body"
    
    if [ "$http_code" -eq 400 ]; then
        echo "✅ PASS: Invalid request properly rejected with 400"
    else
        echo "❌ FAIL: Invalid request should return 400 status"
    fi
    echo
}

# Function to test reset password with invalid token
test_reset_password_invalid_token() {
    echo "Testing reset password with invalid token..."
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d '{
            "token": "invalid_token_12345",
            "new_password": "NewPassword123!"
        }' \
        "$BASE_URL/reset-password")
    
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo $response | sed -E 's/HTTPSTATUS:[0-9]{3}$//')
    
    echo "HTTP Status: $http_code"
    echo "Response: $body"
    
    if [ "$http_code" -eq 404 ]; then
        echo "✅ PASS: Invalid token correctly rejected with 404"
        if echo $body | grep -q "Invalid or expired"; then
            echo "✅ PASS: Appropriate error message for invalid token"
        fi
    else
        echo "❌ FAIL: Invalid token should return 404 status"
    fi
    echo
}

# Function to test reset password with weak password
test_reset_password_weak_password() {
    echo "Testing reset password with weak password..."
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d '{
            "token": "valid_token_example",
            "new_password": "weak"
        }' \
        "$BASE_URL/reset-password")
    
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo $response | sed -E 's/HTTPSTATUS:[0-9]{3}$//')
    
    echo "HTTP Status: $http_code"
    echo "Response: $body"
    
    if [ "$http_code" -eq 400 ]; then
        echo "✅ PASS: Weak password properly rejected with 400"
        if echo $body | grep -q "8 characters"; then
            echo "✅ PASS: Password length requirement enforced"
        fi
    else
        echo "❌ FAIL: Weak password should be rejected with 400 status"
    fi
    echo
}

# Function to test reset password without required fields
test_reset_password_missing_fields() {
    echo "Testing reset password with missing fields..."
    
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -d '{
            "token": "some_token"
        }' \
        "$BASE_URL/reset-password")
    
    http_code=$(echo $response | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    body=$(echo $response | sed -E 's/HTTPSTATUS:[0-9]{3}$//')
    
    echo "HTTP Status: $http_code"
    echo "Response: $body"
    
    if [ "$http_code" -eq 400 ]; then
        echo "✅ PASS: Missing password field properly rejected with 400"
    else
        echo "❌ FAIL: Missing required field should return 400 status"
    fi
    echo
}

# Function to test simulation of successful password reset
test_password_reset_simulation() {
    local email=$1
    echo "Testing password reset simulation..."
    echo "Note: In a real scenario, the user would get the reset token from their email."
    echo "The reset endpoint is available at: POST $BASE_URL/reset-password"
    echo "Expected payload: {\"token\": \"<reset_token>\", \"new_password\": \"NewPassword123!\"}"
    echo "After successful reset, user should be able to log in with the new password."
    echo
}

# Function to test public endpoints (should still work)
test_public_endpoints() {
    echo "Testing that other public endpoints still work..."
    
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
echo "Starting forgot password functionality tests..."
echo

# Test public endpoints first
test_public_endpoints

# Generate random email for testing
TEST_EMAIL=$(generate_random_email)
echo "Using test email: $TEST_EMAIL"
echo

# Set up test user (register and verify email if needed)
if test_registration "$TEST_EMAIL"; then
    echo "Proceeding with forgot password tests..."
    echo
    
    # Test forgot password functionality
    test_forgot_password_valid "$TEST_EMAIL"
    test_forgot_password_invalid
    test_forgot_password_invalid_json
    
    # Test reset password functionality
    test_reset_password_invalid_token
    test_reset_password_weak_password
    test_reset_password_missing_fields
    test_password_reset_simulation "$TEST_EMAIL"
else
    echo "Skipping forgot password tests due to registration failure"
fi

echo "=== Forgot Password Functionality Test Complete ==="
echo
echo "Summary of expected behavior:"
echo "- Forgot password should accept valid email addresses (200)"
echo "- Forgot password should not reveal if email exists or not (security)"
echo "- Reset password should validate tokens and reject invalid/expired ones (404)"
echo "- Reset password should enforce password strength requirements (400)"
echo "- Reset password should require both token and new_password fields (400)"
echo "- System should send emails with reset links to valid email addresses"
echo "- Reset links should point to: montessoriworldconnect/reset-password"
echo
echo "Manual verification steps:"
echo "1. Check email service logs to see if reset emails are sent"
echo "2. If using real SMTP, check actual email for reset link"
echo "3. Extract token from email and test actual password reset"
echo "4. After reset, verify login works with new password"
echo "5. Verify old password no longer works"