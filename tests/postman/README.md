# MWC Postman Tests

This folder contains a Postman collection and environment to test the API by user role.

Contents:
- mwc.postman_collection.json — Role-based folders and tests
- mwc.local.postman_environment.json — Local variables and credentials

## Prerequisites
- Running API (default: http://localhost:8080)
- Existing users with roles and credentials set in the environment
- Postman or Newman

## Quick Start (Postman)
1. Import `mwc.postman_collection.json` and `mwc.local.postman_environment.json`.
2. Edit the environment variables for emails/passwords.
3. Run the `Auth` folder to populate tokens.
4. Run role folders (Institution / Training Center, Montessori Professional, Parent, Admin).

## Quick Start (Newman)
Install newman globally:

```bash
npm install -g newman
```

Run all tests:

```bash
newman run tests/postman/mwc.postman_collection.json \
  -e tests/postman/mwc.local.postman_environment.json \
  --insecure
```

Run a specific folder (e.g., Institution):

```bash
newman run tests/postman/mwc.postman_collection.json \
  -e tests/postman/mwc.local.postman_environment.json \
  --folder "Institution / Training Center" \
  --insecure
```

## Notes
- Authorization header uses `Bearer {{token_*}}`. Middleware now accepts both Bearer and raw token.
- `Institution / Training Center` updates the institution profile, then asserts `/me` reflects the `displayName`.
- Adjust emails/passwords to match your seed or real users. If you change `baseUrl`, update the environment.
