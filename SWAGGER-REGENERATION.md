# Swagger Documentation Regeneration

## Overview

This document explains the process of regenerating the Swagger documentation for the MWC Backend API.

## Background

The MWC Backend API uses Swagger/OpenAPI for API documentation. The Swagger documentation is generated using the [swaggo/swag](https://github.com/swaggo/swag) tool, which extracts annotations from the Go code to generate the swagger.json and swagger.yaml files.

## Changes Made

1. **Updated Host Annotation**: The host annotation in `internal/api/swagger.go` was updated to include both localhost and the production URL:
   ```go
   // @host localhost:8080, https://api.montessoriworldconnect.com
   ```

2. **Regenerated Swagger Documentation**: The Swagger documentation was regenerated using the swag tool:
   ```bash
   swag init -g internal/api/swagger.go -o ./docs
   ```

## Files Updated

- `/internal/api/swagger.go`: Updated the host annotation
- `/docs/swagger.json`: Regenerated from the updated annotations
- `/docs/swagger.yaml`: Regenerated from the updated annotations
- `/docs/docs.go`: Regenerated from the updated annotations

## How to Regenerate Swagger Documentation

To regenerate the Swagger documentation in the future, follow these steps:

1. Make sure you have the swag tool installed:
   ```bash
   go get -u github.com/swaggo/swag/cmd/swag
   ```

2. Run the swag init command from the project root:
   ```bash
   swag init -g internal/api/swagger.go -o ./docs
   ```

3. Verify that the generated files are correct by checking the swagger.json file.

## Notes

- The Swagger UI is available at `/swagger/index.html` when the application is running.
- The swagger.json file is served at `/docs/swagger.json`.
- The host field in the swagger.json file now includes both localhost and the production URL, allowing the API to be accessed from both environments.