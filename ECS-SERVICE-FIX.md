# ECS Service Fix

## Issue

The GitHub Actions workflow was failing with the following error:

```
Run aws ecs update-service \

An error occurred (ServiceNotFoundException) when calling the UpdateService operation: 
Error: Process completed with exit code 254.
```

This error occurs when the workflow tries to update an ECS service that doesn't exist.

## Root Cause

After examining the workflow file and the deployment plan, I identified that the issue was that the workflow was trying to update an ECS service (`mwc-backend-service` in the `mwc-cluster` cluster) that hadn't been created yet. The deployment plan includes steps to create the ECS cluster and service, but these steps weren't being executed as part of the automated workflow.

## Solution

I modified the `.github/workflows/aws-ecr-push.yml` file to add two new steps before the "Deploy to ECS" step:

1. **Check if ECS cluster exists and create if it doesn't**: This step checks if the ECS cluster specified by the `ECS_CLUSTER` environment variable exists, and creates it if it doesn't.

2. **Check if ECS service exists and create if it doesn't**: This step checks if the ECS service specified by the `ECS_SERVICE` environment variable exists in the cluster, and creates it if it doesn't. When creating the service, it uses the task definition that was just registered, and sets up the necessary network configuration using default subnets and security groups.

```yaml
- name: Check if ECS cluster exists and create if it doesn't
  run: |
    # Check if cluster exists
    if ! aws ecs describe-clusters --clusters ${{ env.ECS_CLUSTER }} --query "clusters[?clusterName=='${{ env.ECS_CLUSTER }}']" --output text | grep -q "${{ env.ECS_CLUSTER }}"; then
      echo "Cluster ${{ env.ECS_CLUSTER }} does not exist. Creating it..."
      aws ecs create-cluster --cluster-name ${{ env.ECS_CLUSTER }}
    else
      echo "Cluster ${{ env.ECS_CLUSTER }} already exists."
    fi

- name: Check if ECS service exists and create if it doesn't
  run: |
    # Check if service exists
    if ! aws ecs list-services --cluster ${{ env.ECS_CLUSTER }} --query "serviceArns[?contains(@, '${{ env.ECS_SERVICE }}')]" --output text | grep -q "${{ env.ECS_SERVICE }}"; then
      echo "Service ${{ env.ECS_SERVICE }} does not exist in cluster ${{ env.ECS_CLUSTER }}. Creating it..."
      aws ecs create-service \
        --cluster ${{ env.ECS_CLUSTER }} \
        --service-name ${{ env.ECS_SERVICE }} \
        --task-definition ${{ env.ECS_TASK_DEFINITION }}:${{ steps.register-task.outputs.revision }} \
        --desired-count 1 \
        --launch-type FARGATE \
        --platform-version LATEST \
        --network-configuration "awsvpcConfiguration={subnets=[$(aws ec2 describe-subnets --filters 'Name=default-for-az,Values=true' --query 'Subnets[0:2].SubnetId' --output text | sed 's/\t/,/g')],securityGroups=[$(aws ec2 describe-security-groups --filters 'Name=group-name,Values=default' --query 'SecurityGroups[0].GroupId' --output text)],assignPublicIp=ENABLED}" \
        --deployment-configuration "deploymentCircuitBreaker={enable=true,rollback=true},maximumPercent=200,minimumHealthyPercent=100" \
        --health-check-grace-period-seconds 60
    else
      echo "Service ${{ env.ECS_SERVICE }} already exists in cluster ${{ env.ECS_CLUSTER }}."
    fi
```

## Benefits

These changes provide the following benefits:

1. **Automated Setup**: The workflow now automatically creates the ECS cluster and service if they don't exist, eliminating the need for manual setup.

2. **Consistent Environment**: The cluster and service are created with consistent settings based on the environment variables defined in the workflow.

3. **Improved Reliability**: The workflow is now more reliable as it ensures that the necessary resources exist before attempting to use them.

4. **Simplified Deployment**: Developers no longer need to manually create the ECS cluster and service before running the workflow.

## Verification

After making these changes, the workflow should now:

1. Check if the ECS cluster exists and create it if it doesn't
2. Check if the ECS service exists and create it if it doesn't
3. Update the service with the new task definition

This ensures that the workflow will succeed even if the ECS cluster and service don't exist yet.