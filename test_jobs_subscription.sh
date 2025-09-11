#!/bin/bash

# Test script to verify that jobs endpoint requires paid subscription
echo "Testing jobs endpoint subscription requirement..."

BASE_URL="http://localhost:8080/api/v1"

echo "1. Testing jobs endpoint without authentication (should fail with 401)..."
curl -s -w "HTTP Status: %{http_code}\n" -o /tmp/response1.json "$BASE_URL/jobs"
cat /tmp/response1.json
echo ""

echo "2. Testing jobs endpoint with invalid token (should fail with 401)..."
curl -s -w "HTTP Status: %{http_code}\n" -H "Authorization: invalid_token" -o /tmp/response2.json "$BASE_URL/jobs"
cat /tmp/response2.json
echo ""

echo "3. Testing public endpoints still work..."
echo "   - Testing /schools/public:"
curl -s -w "HTTP Status: %{http_code}\n" -o /tmp/response3.json "$BASE_URL/schools/public"
echo "Response length: $(cat /tmp/response3.json | wc -c) characters"
echo ""

echo "   - Testing /events:"
curl -s -w "HTTP Status: %{http_code}\n" -o /tmp/response4.json "$BASE_URL/events"
echo "Response length: $(cat /tmp/response4.json | wc -c) characters"
echo ""

echo "4. Testing API root endpoint (jobs should not be listed in public endpoints)..."
curl -s -w "HTTP Status: %{http_code}\n" -o /tmp/response5.json "$BASE_URL/"
echo "API response:"
cat /tmp/response5.json | grep -o '"endpoints":\[.*\]' || echo "Could not find endpoints array"
echo ""

# Note: To test with valid authentication and subscription, you would need:
# - A valid JWT token from a logged-in user
# - That user having an active paid subscription
# Example (commented out as it requires actual auth setup):
# echo "5. Testing with valid auth and subscription (requires setup)..."
# TOKEN="your_jwt_token_here"
# curl -s -w "HTTP Status: %{http_code}\n" -H "Authorization: $TOKEN" -o /tmp/response6.json "$BASE_URL/jobs"

echo "Test completed. Check the HTTP status codes:"
echo "- Without auth: should be 401"
echo "- With invalid token: should be 401" 
echo "- With valid auth but no subscription: should be 403"
echo "- With valid auth and active subscription: should be 200"

# Clean up temp files
rm -f /tmp/response*.json