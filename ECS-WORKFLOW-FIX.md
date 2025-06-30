# ECS Workflow Fix

## Issue
The GitHub Actions workflow for deploying to AWS ECS (`aws-ecs-deploy.yml`) was not running automatically when the ECR push workflow completed. The issue was reported as:

> only aws-ecr-push.yml is running the aws-ecs-deploy.yml doesn't run when pushed to git

## Root Cause
After examining both workflow files, I identified the root cause of the issue:

In the `aws-ecs-deploy.yml` file, the workflow_run trigger was configured to listen for the completion of a workflow named "Build and Push to AWS ecs":

```yaml
on:
  workflow_run:
    workflows: ["Build and Push to AWS ecs"]
    types:
      - completed
    branches: [staging]
```

However, the actual name of the ECR push workflow (defined in `aws-ecr-push.yml`) is "Build and Push to AWS ECR" (note the capitalization of "ECR").

This mismatch in the workflow name was preventing the ECS deployment workflow from being triggered automatically when the ECR push workflow completed.

## Solution
The fix was simple: update the workflow name in the workflow_run trigger to match the actual name of the ECR push workflow:

```yaml
on:
  workflow_run:
    workflows: ["Build and Push to AWS ECR"]
    types:
      - completed
    branches: [staging]
```

## Verification
After making this change, the ECS deployment workflow should now be triggered automatically when the ECR push workflow completes successfully. This ensures that the application is deployed to ECS after the Docker image is built and pushed to ECR.

## Additional Notes
- The workflow can still be triggered manually using the workflow_dispatch trigger.
- The workflow will only run if the ECR push workflow completes successfully, as specified by the condition: `if: ${{ github.event.workflow_run.conclusion == 'success' || github.event_name == 'workflow_dispatch' }}`
- The workflow is configured to deploy to the staging environment by default, but this can be overridden when triggering the workflow manually.