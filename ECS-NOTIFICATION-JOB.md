# ECS Notification Job

## Overview

This document explains the changes made to the GitHub Actions workflow to move the ECS Slack notifications to a separate job that depends on the deploy-to-ecs job.

## Changes Made

The following changes were made to the `.github/workflows/aws-ecr-push.yml` file:

1. **Added Outputs to deploy-to-ecs Job**
   - Added outputs for the task definition revision and image information
   - This allows the notification job to access these values

```yaml
  deploy-to-ecs:
    name: Deploy to ECS
    runs-on: ubuntu-latest
    needs: build-and-push
    outputs:
      revision: ${{ steps.register-task.outputs.revision }}
      image: ${{ steps.update-image.outputs.image }}
```

2. **Created New Job for ECS Slack Notifications**
   - Created a new job called "send-ecs-notification" that depends on the deploy-to-ecs job
   - Added if: always() to ensure it runs even if the deploy-to-ecs job fails
   - Moved the Slack notification steps from the deploy-to-ecs job to the new job
   - Updated the notification steps to use the outputs from the deploy-to-ecs job

```yaml
  send-ecs-notification:
    name: Send ECS Deployment Notification
    runs-on: ubuntu-latest
    needs: deploy-to-ecs
    if: always()
    env:
      ENVIRONMENT: ${{ github.event.inputs.environment || (github.ref == 'refs/heads/main' && 'prod') || 'staging' }}
    steps:
      # Send Slack notification for successful deployment
      - name: Send Slack notification - Success
        if: needs.deploy-to-ecs.result == 'success'
        uses: rtCamp/action-slack-notify@v2
        env:
          SLACK_CHANNEL: deployments
          SLACK_COLOR: good
          SLACK_ICON: https://github.com/rtCamp.png?size=48
          SLACK_MESSAGE: |
            ✅ Successfully deployed to ECS in ${{ env.ENVIRONMENT }} environment
            *Cluster:* ${{ env.ECS_CLUSTER }}
            *Service:* ${{ env.ECS_SERVICE }}
            *Task Definition:* ${{ env.ECS_TASK_DEFINITION }}:${{ needs.deploy-to-ecs.outputs.revision }}
            *Image:* ${{ needs.deploy-to-ecs.outputs.image }}
          SLACK_TITLE: ECS Deployment Success
          SLACK_USERNAME: GitHub Actions

      # Send Slack notification for failed deployment
      - name: Send Slack notification - Failure
        if: needs.deploy-to-ecs.result != 'success'
        uses: rtCamp/action-slack-notify@v2
        env:
          SLACK_CHANNEL: deployments
          SLACK_COLOR: danger
          SLACK_ICON: https://github.com/rtCamp.png?size=48
          SLACK_MESSAGE: |
            ❌ Failed to deploy to ECS in ${{ env.ENVIRONMENT }} environment
            *Repository:* ${{ github.repository }}
            *Workflow:* ${{ github.workflow }}
            *Check the [GitHub Actions logs](https://github.com/${{ github.repository }}/actions/runs/${{ github.run_id }}) for more details*
          SLACK_TITLE: ECS Deployment Failure
          SLACK_USERNAME: GitHub Actions
```

## Benefits

These changes provide the following benefits:

1. **Separation of Concerns**: The deploy-to-ecs job now focuses solely on deploying to ECS, while the notification job handles sending notifications.

2. **Improved Workflow Visibility**: The workflow now clearly shows the deployment and notification steps as separate jobs in the GitHub Actions UI.

3. **Better Error Handling**: The notification job runs even if the deployment job fails, ensuring that notifications are always sent.

4. **Easier Maintenance**: The notification job can be modified or extended without affecting the deployment job.

## Verification

The changes have been verified to ensure that:

1. The deploy-to-ecs job correctly outputs the task definition revision and image information.
2. The send-ecs-notification job correctly depends on the deploy-to-ecs job.
3. The send-ecs-notification job runs even if the deploy-to-ecs job fails.
4. The Slack notifications include the correct information from the deploy-to-ecs job.