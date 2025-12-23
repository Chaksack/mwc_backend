# Webhooks & Events

The Montessori World Connect API uses webhooks to notify your application about events that happen in your account.

## Stripe Webhooks

The platform integrates with Stripe for payment processing and uses webhooks to handle subscription events.

### Webhook Endpoint

```
POST /api/v1/subscription/webhook
```

This endpoint receives webhook events from Stripe. It's already configured in the API and doesn't require authentication (Stripe signature verification is used instead).

### Supported Events

#### checkout.session.completed
Triggered when a user successfully completes a checkout session.

**Actions:**
- Creates or updates subscription record
- Sends confirmation email
- Creates in-app notification

**Example Payload:**
```json
{
  "type": "checkout.session.completed",
  "data": {
    "object": {
      "id": "cs_test_...",
      "customer": "cus_...",
      "subscription": "sub_...",
      "metadata": {
        "user_id": "123",
        "plan": "monthly"
      }
    }
  }
}
```

#### customer.subscription.updated
Triggered when a subscription is updated (e.g., plan change, status change).

**Actions:**
- Updates subscription status in database
- Sends notification if status changed
- Logs the update

#### customer.subscription.deleted
Triggered when a subscription is cancelled or expires.

**Actions:**
- Updates subscription status to "canceled"
- Sends cancellation notification
- Records cancellation date

#### invoice.payment_succeeded
Triggered when an invoice payment succeeds.

**Actions:**
- Updates payment status
- Extends subscription period
- Sends payment confirmation

#### invoice.payment_failed
Triggered when an invoice payment fails.

**Actions:**
- Updates subscription status to "past_due"
- Sends payment failure notification
- May trigger retry logic

### Setting Up Webhooks

If you're integrating Stripe webhooks with your own application:

1. **Create webhook endpoint** in Stripe Dashboard
2. **Add endpoint URL**: `https://api.montessoriworldconnect.com/api/v1/subscription/webhook`
3. **Select events** to listen to:
   - `checkout.session.completed`
   - `customer.subscription.updated`
   - `customer.subscription.deleted`
   - `invoice.payment_succeeded`
   - `invoice.payment_failed`
4. **Copy webhook signing secret** for verification

## WebSocket Events

The API provides real-time updates via WebSocket connections.

### Connecting to WebSocket

```javascript
const token = 'your-jwt-token';
const ws = new WebSocket(`wss://api.montessoriworldconnect.com/wss?token=${token}`);

ws.onopen = () => {
  console.log('WebSocket connected');
};

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  handleMessage(message);
};
```

### Event Types

#### new_message
Sent when you receive a new message.

```json
{
  "type": "new_message",
  "data": {
    "id": 123,
    "sender_id": 456,
    "sender_name": "John Doe",
    "content": "Hello!",
    "created_at": "2025-12-23T10:00:00Z"
  }
}
```

#### notification
Sent when you receive a new notification.

```json
{
  "type": "notification",
  "data": {
    "id": 789,
    "title": "New Job Application",
    "message": "You have a new application for Lead Teacher position",
    "created_at": "2025-12-23T10:00:00Z"
  }
}
```

#### job_application
Sent to institutions when someone applies for their job.

```json
{
  "type": "job_application",
  "data": {
    "job_id": 321,
    "job_title": "Lead Teacher",
    "applicant_id": 654,
    "applicant_name": "Jane Smith",
    "applied_at": "2025-12-23T10:00:00Z"
  }
}
```

#### subscription_updated
Sent when subscription status changes.

```json
{
  "type": "subscription_updated",
  "data": {
    "status": "active",
    "plan": "monthly",
    "end_date": "2025-12-23T10:00:00Z"
  }
}
```

### WebSocket Best Practices

1. **Implement reconnection logic**
```javascript
let reconnectAttempts = 0;
const maxReconnectAttempts = 5;

function connect() {
  const ws = new WebSocket(`wss://api.montessoriworldconnect.com/wss?token=${token}`);
  
  ws.onclose = () => {
    if (reconnectAttempts < maxReconnectAttempts) {
      reconnectAttempts++;
      setTimeout(connect, Math.pow(2, reconnectAttempts) * 1000);
    }
  };
  
  ws.onopen = () => {
    reconnectAttempts = 0;
  };
}
```

2. **Handle connection errors gracefully**
3. **Implement message queuing** for offline scenarios
4. **Use heartbeat/ping** to keep connection alive
5. **Parse and validate** all incoming messages

## Custom Webhooks (Future)

In future versions, we plan to support custom webhooks where you can register your own endpoints to receive events:

- User registration
- Profile updates
- Job applications
- Review submissions
- Subscription changes

Stay tuned for updates!

## Webhook Security

### Stripe Webhook Verification

The API automatically verifies Stripe webhook signatures using the webhook signing secret. This ensures:
- Events are from Stripe
- Events haven't been tampered with
- Events are processed exactly once

### WebSocket Authentication

WebSocket connections require a valid JWT token in the connection URL:
```
wss://api.montessoriworldconnect.com/wss?token=<your-jwt-token>
```

Connections without valid tokens are immediately rejected.

## Event Logs

All webhook events and important actions are logged in the system. Admins can view these logs at:

```
GET /api/v1/admin/action-logs
Authorization: <admin-token>
```

This helps with debugging, auditing, and monitoring system activity.
