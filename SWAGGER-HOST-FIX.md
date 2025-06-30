# Swagger Host Field Fix

## Issue

The issue was with the `host` field in the swagger.json file, which was set to:

```json
"host": "localhost:8080, https://api.montessoriworldconnect.com",
```

This format is not correct according to the OpenAPI/Swagger 2.0 specification.

## OpenAPI/Swagger 2.0 Specification

According to the OpenAPI/Swagger 2.0 specification:

1. The `host` field should be the host (name or IP) serving the API.
2. It MUST be the host only and does not include the scheme (http:// or https://) nor sub-paths.
3. It MAY include a port.
4. The host does not support multiple values or a list of hosts.

## Issues with the Previous Configuration

The previous configuration had two issues:

1. It included multiple hosts separated by a comma, which is not supported by the specification.
2. One of the hosts included the scheme (https://), which is also not allowed.

## Changes Made

1. Updated the host annotation in `internal/api/swagger.go`:
   ```go
   // Before
   // @host localhost:8080, https://api.montessoriworldconnect.com
   
   // After
   // @host localhost:8080
   ```

2. Regenerated the swagger.json file using the swag tool:
   ```bash
   swag init -g internal/api/swagger.go -o ./docs
   ```

## Result

The `host` field in the swagger.json file is now correctly set to:

```json
"host": "localhost:8080",
```

This follows the OpenAPI/Swagger 2.0 specification and should work correctly with Swagger UI and other tools that consume the swagger.json file.

## Alternative Solutions

If you need to support multiple environments (development and production), consider:

1. Using environment-specific swagger.json files
2. Using a proxy or gateway that handles the routing based on the environment
3. Using a server-side solution to dynamically modify the swagger.json file based on the environment

## Conclusion

The `host` field in the swagger.json file has been corrected to follow the OpenAPI/Swagger 2.0 specification. This should ensure that the Swagger UI and other tools that consume the swagger.json file work correctly.