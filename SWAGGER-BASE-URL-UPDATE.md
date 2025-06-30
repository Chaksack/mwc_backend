# Swagger Base URL and Schemes Update

## Overview

This document explains the changes made to the Swagger configuration to support multiple base URLs and schemes as requested.

## Changes Made

1. **Updated Host**: Changed the host in the Swagger configuration from `localhost:8080` to `api.montessoriworldconnect.com`.
2. **Updated Schemes**: Added both `http` and `https` schemes to the Swagger configuration.

### Before:
```go
// @host localhost:8080
// @BasePath /api/v1
// @schemes http
```

### After:
```go
// @host api.montessoriworldconnect.com
// @BasePath /api/v1
// @schemes http https
```

## Limitations

The OpenAPI/Swagger 2.0 specification, which is used by this project, has some limitations:

1. **Single Host Only**: The specification only supports a single host in the `host` field. It's not possible to specify multiple hosts (like both `localhost:8080` and `api.montessoriworldconnect.com`) in a single Swagger document.

2. **No Server Variables**: Unlike OpenAPI 3.0, which introduced server variables to handle multiple environments, OpenAPI 2.0 doesn't have this feature.

## Workarounds

To work with both development (localhost) and production environments, consider the following approaches:

1. **Environment-Specific Swagger Files**: Generate different Swagger files for different environments.
2. **Dynamic Host Resolution**: Implement client-side logic to determine the appropriate host based on the environment.
3. **Proxy Configuration**: Use a proxy in development that forwards requests to the appropriate host.
4. **Upgrade to OpenAPI 3.0**: Consider upgrading to OpenAPI 3.0, which supports server variables for multiple environments.

## Verification

The changes have been verified by regenerating the Swagger documentation using the `swag` tool:

```bash
swag init -g internal/api/swagger.go -o ./docs
```

The updated `swagger.json` file now includes both schemes and the production host:

```json
{
    "schemes": [
        "http",
        "https"
    ],
    "swagger": "2.0",
    "info": {
        "title": "Montessori World Connect API",
        "version": "1.0"
    },
    "host": "api.montessoriworldconnect.com",
    "basePath": "/api/v1"
}
```

## Conclusion

The Swagger configuration has been updated to use the production host and support both HTTP and HTTPS schemes. While this doesn't fully address the requirement to support both localhost and production hosts in a single Swagger document (due to limitations in the OpenAPI 2.0 specification), it provides a working solution for the production environment.
