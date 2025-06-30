# GitHub Workflow Update for ECS Deployment

## Overview

This document describes the changes made to the GitHub Actions workflow for deploying the MWC Backend application to AWS Elastic Container Service (ECS).

## Changes Made

The following changes were made to the `.github/workflows/aws-ecs-deploy.yml` file:

1. **Use Task Definition from Repository**
   - Instead of downloading the task definition from ECS, the workflow now uses the `task-definition.json` file from the repository.
   - This allows for version control of the task definition and ensures that the correct configuration is used for each deployment.

2. **Replace Placeholders in Task Definition**
   - Added a step to get the AWS account ID and store it as an output variable.
   - Added a step to replace the `ACCOUNT_ID` placeholder in the task definition with the actual AWS account ID.
   - Updated the container image URL in the task definition to use the correct registry, repository, and tag.

3. **Restart ECS Service**
   - The workflow already included a step to update the ECS service with the new task definition and force a new deployment.
   - This ensures that the service is restarted with the new task definition.

## Workflow Steps

The updated workflow now follows these steps:

1. Checkout code (which includes the task-definition.json file)
2. Configure AWS credentials
3. Login to Amazon ECR
4. Get AWS account ID
5. Prepare task definition by replacing placeholders
6. Update container image in task definition
7. Register new task definition
8. Deploy to ECS with the new task definition
9. Wait for service stability
10. Send notifications about the deployment status

## Benefits

These changes provide the following benefits:

1. **Version Control**: The task definition is now stored in the repository, allowing for version control and easier tracking of changes.
2. **Consistency**: The same task definition is used across all environments, with environment-specific values replaced at deployment time.
3. **Automation**: The entire process of updating the task definition and restarting the ECS service is automated.
4. **Transparency**: The workflow provides clear logs and notifications about the deployment status.

## Usage

The workflow can be triggered in two ways:

1. **Automatically**: When the "Build and Push to AWS ECR" workflow completes successfully.
2. **Manually**: By using the "Run workflow" button in the GitHub Actions UI and selecting the target environment.

No additional configuration is needed to use the updated workflow.