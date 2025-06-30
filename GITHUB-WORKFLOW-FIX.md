# GitHub Workflow Fix for Environment Variable Placeholders

## Issue

The GitHub Actions workflow was failing with the following errors:

```
sed: -e expression #1, char 38: unknown option to `s'
Error: Process completed with exit code 1.

Run # Set environment-specific tag
The push refers to repository [***.dkr.ecr.us-east-1.amazonaws.com/***]
An image does not exist locally with the tag: ***.dkr.ecr.us-east-1.amazonaws.com/***
Error: Process completed with exit code 1.
```

## Root Cause Analysis

After examining the workflow file, three issues were identified:

1. **Sed Command Errors**: The `sed` commands used to replace placeholders in the task definition were failing due to special characters in the GitHub secrets. When special characters like `/`, `&`, or `$` are present in the replacement string, they need to be escaped properly in `sed` commands.

2. **Job Dependency Issue**: The `deploy-to-ecs` job was depending on the `build-and-push` job, but not on the `push-image-to-ecr` job. This meant that the deployment job could start before the image was actually pushed to ECR, resulting in the "image does not exist locally" error.

3. **YAML Indentation Issues**: The Slack notification steps in the `deploy-to-ecs` job were incorrectly indented, causing YAML parsing errors.

## Changes Made

### 1. Replaced Sed Commands with JQ

Instead of using `sed` commands to replace placeholders in the task definition, we now use `jq` to modify the JSON file. This approach:

- Creates a temporary JSON file with all the environment variables
- Uses `jq` to update the task definition by replacing placeholder values with actual values
- Handles special characters properly since it's using JSON parsing instead of text replacement

```yaml
- name: Replace environment variables with GitHub secrets
  run: |
    # Create a temporary file with environment variables
    cat > env-values.json << EOF
    {
      "DATABASE_URL": "${{ secrets.DATABASE_URL }}",
      "RABBITMQ_URL": "${{ secrets.RABBITMQ_URL }}",
      # ... other environment variables ...
    }
    EOF
    
    # Use jq to update the task definition
    jq --slurpfile env env-values.json '
      .containerDefinitions[0].environment |= map(
        if .value | endswith("_PLACEHOLDER") then
          $key = .name;
          $placeholder = .value;
          $value = $env[0][$key];
          .value = $value
        else
          .
        end
      )
    ' task-definition.json > task-definition-updated.json
    
    # Replace the original file
    mv task-definition-updated.json task-definition.json
    
    # Clean up
    rm env-values.json
```

### 2. Fixed Job Dependency

Changed the `needs` value for the `deploy-to-ecs` job from `build-and-push` to `push-image-to-ecr` to ensure the deployment job runs after the image is pushed to ECR:

```yaml
deploy-to-ecs:
  name: Deploy to ECS
  runs-on: ubuntu-latest
  needs: push-image-to-ecr
```

### 3. Fixed Indentation Issues

Properly indented the Slack notification steps in the `deploy-to-ecs` job:

```yaml
# Send Slack notification for successful deployment
- name: Send Slack notification - Success
  if: success()
  uses: rtCamp/action-slack-notify@v2
  env:
    SLACK_CHANNEL: deployments
    # ... other environment variables ...

# Send Slack notification for failed deployment
- name: Send Slack notification - Failure
  if: failure()
  uses: rtCamp/action-slack-notify@v2
  env:
    SLACK_CHANNEL: deployments
    # ... other environment variables ...
```

## Benefits

These changes provide the following benefits:

1. **Improved Reliability**: The workflow is now more reliable as it properly handles special characters in GitHub secrets.

2. **Correct Job Execution Order**: The deployment job now runs after the image is pushed to ECR, ensuring the image is available when needed.

3. **Valid YAML Syntax**: The workflow file now has valid YAML syntax with proper indentation.

## Verification

After making these changes, the workflow should run successfully without the previous errors. The task definition should be properly updated with the values from GitHub secrets, and the deployment to ECS should proceed after the image is pushed to ECR.