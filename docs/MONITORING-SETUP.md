# Monitoring and Alerting Setup

This document describes the monitoring and alerting setup for the MWC Backend application.

## Overview

The monitoring and alerting setup consists of the following components:

1. **SonarQube** - For code quality analysis
2. **Prometheus** - For metrics collection and monitoring
3. **AlertManager** - For alert management and notification
4. **Slack** - For notifications and alerts

## GitHub Actions Workflows

The following GitHub Actions workflows are used to set up and manage the monitoring and alerting system:

### 1. SonarQube Analysis

**Workflow file:** `.github/workflows/sonarqube-analysis.yml`

This workflow runs SonarQube analysis on the codebase to identify code quality issues, bugs, vulnerabilities, and code smells.

**Triggers:**
- Push to main or staging branches
- Pull requests to main or staging branches
- Manual trigger

**Configuration:**
- Set up the following secrets in your GitHub repository:
  - `SONAR_TOKEN`: Authentication token for SonarQube
  - `SONAR_HOST_URL`: URL of the SonarQube server
  - `SLACK_WEBHOOK`: Webhook URL for Slack notifications

**Usage:**
```bash
# Trigger manually
gh workflow run sonarqube-analysis.yml -f environment=prod
```

### 2. Prometheus and AlertManager Setup

**Workflow file:** `.github/workflows/prometheus-setup.yml`

This workflow sets up Prometheus and AlertManager for monitoring the application and sending alerts.

**Triggers:**
- Push to main or staging branches (when the workflow file or metrics code changes)
- Manual trigger

**Configuration:**
- Set up the following secrets in your GitHub repository:
  - `AWS_ACCESS_KEY_ID`: AWS access key ID
  - `AWS_SECRET_ACCESS_KEY`: AWS secret access key
  - `AWS_ACCOUNT_ID`: AWS account ID
  - `SLACK_WEBHOOK`: Webhook URL for Slack notifications

**Usage:**
```bash
# Trigger manually
gh workflow run prometheus-setup.yml -f environment=prod
```

### 3. Slack Notifications Setup

**Workflow file:** `.github/workflows/slack-notifications.yml`

This workflow sets up Slack notifications for the application.

**Triggers:**
- Push to main or staging branches (when the workflow file changes)
- Manual trigger

**Configuration:**
- Set up the following secrets in your GitHub repository:
  - `SLACK_WEBHOOK`: Webhook URL for Slack notifications

**Usage:**
```bash
# Trigger manually
gh workflow run slack-notifications.yml -f environment=prod
```

## Metrics Endpoints

The application exposes the following metrics endpoints:

- `/metrics` - HTML dashboard for viewing metrics
- `/metrics/api` - JSON API endpoint for raw metrics data
- `/metrics/monitor` - Fiber's built-in monitor
- `/metrics/prometheus` - Prometheus-compatible metrics endpoint

## Alert Rules

Alert rules are defined in the Prometheus configuration. The following alerts are configured:

1. **HighMemoryUsage** - Triggered when memory usage is above 500MB for 5 minutes
2. **HighCPUUsage** - Triggered when CPU usage is above 80% for 5 minutes
3. **HighHTTPErrorRate** - Triggered when HTTP error rate is above 5% for 5 minutes
4. **HighDBQueryTime** - Triggered when average database query time is above 1 second for 5 minutes

## Slack Notification Channels

The following Slack channels are used for notifications:

- **#deployments** - Deployment notifications
- **#alerts** - System alerts from Prometheus/AlertManager
- **#monitoring** - Monitoring system updates
- **#code-quality** - Code quality reports from SonarQube

## Troubleshooting

### SonarQube Analysis Issues

If SonarQube analysis fails, check the following:

1. Verify that the `SONAR_TOKEN` and `SONAR_HOST_URL` secrets are correctly configured
2. Check the SonarQube server logs for any errors
3. Ensure that the SonarQube server is accessible from GitHub Actions

### Prometheus and AlertManager Issues

If Prometheus or AlertManager setup fails, check the following:

1. Verify that the AWS credentials are correctly configured
2. Check the AWS ECS service logs for any errors
3. Ensure that the ECS cluster and services exist

### Slack Notification Issues

If Slack notifications fail, check the following:

1. Verify that the `SLACK_WEBHOOK` secret is correctly configured
2. Check that the Slack webhook URL is valid and points to the correct workspace
3. Ensure that the Slack app has the necessary permissions

## Additional Resources

- [Prometheus Documentation](https://prometheus.io/docs/introduction/overview/)
- [AlertManager Documentation](https://prometheus.io/docs/alerting/latest/alertmanager/)
- [SonarQube Documentation](https://docs.sonarqube.org/latest/)
- [Slack API Documentation](https://api.slack.com/messaging/webhooks)