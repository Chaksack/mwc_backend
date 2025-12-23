# Subscription System

Montessori World Connect offers flexible subscription plans to access premium features.

## Free Trial

All new users automatically receive a **60-day free trial** upon registration with full access to premium features.

### Trial Benefits Include:
- ✅ Advanced school search with filters
- ✅ Direct messaging with institutions
- ✅ Priority job listings and applications
- ✅ Detailed school information access
- ✅ Exclusive educational resources
- ✅ Community forums and networking
- ✅ Unlimited saved schools
- ✅ Full notification system

### Trial Details:
- **Duration**: 60 days
- **Status**: Active immediately upon registration
- **Auto-renewal**: Disabled (no automatic charge)
- **Upgrade**: Available anytime during trial

## Subscription Plans

After your free trial, choose from these plans:

### Monthly Plan
- **Price**: $9.99/month
- **Billing**: Monthly
- **Features**: All premium features
- **Cancellation**: Cancel anytime

### Annual Plan
- **Price**: $99.99/year
- **Billing**: Yearly
- **Savings**: Save 17% compared to monthly
- **Features**: All premium features
- **Cancellation**: Cancel anytime

## Premium Features

With an active subscription, you get:

### For All Users:
- Advanced search capabilities
- Detailed institution profiles
- Direct messaging system
- Priority support
- Ad-free experience

### For Montessori Professionals:
- Unlimited job applications
- Enhanced profile visibility
- Job alerts and notifications
- Resume/CV builder tools

### For Parents:
- Full school details and reviews
- Connect with other parents
- School comparison tools
- Educational resources library

### For Institutions:
- Unlimited job postings
- Applicant tracking system
- Enhanced profile features
- Analytics dashboard

## Managing Subscriptions

### View Current Subscription

```http
GET /api/v1/subscription
Authorization: <your-token>
```

**Response:**
```json
{
  "id": 123,
  "user_id": 456,
  "plan": "monthly",
  "status": "active",
  "start_date": "2025-01-01T00:00:00Z",
  "end_date": "2025-02-01T00:00:00Z",
  "auto_renew": true,
  "stripe_customer_id": "cus_...",
  "stripe_subscription_id": "sub_..."
}
```

### Create Checkout Session

```http
POST /api/v1/subscription/checkout
Authorization: <your-token>
Content-Type: application/json

{
  "plan": "monthly",
  "success_url": "https://yourapp.com/success",
  "cancel_url": "https://yourapp.com/cancel"
}
```

**Response:**
```json
{
  "session_id": "cs_...",
  "url": "https://checkout.stripe.com/..."
}
```

Redirect users to the `url` to complete payment via Stripe Checkout.

### Cancel Subscription

```http
POST /api/v1/subscription/cancel
Authorization: <your-token>
Content-Type: application/json

{
  "reason": "Optional cancellation reason"
}
```

**Note:** Subscription remains active until the end of the current billing period.

### Create Billing Portal Session

```http
POST /api/v1/subscription/portal
Authorization: <your-token>
```

Returns a URL to Stripe's billing portal where users can:
- Update payment methods
- View invoices
- Change subscription plans
- Cancel subscription

## Subscription Status

### Status Types:

| Status | Description |
|--------|-------------|
| `active` | Subscription is active and user has access |
| `canceled` | Subscription cancelled, access until period end |
| `expired` | Subscription expired, no access |
| `past_due` | Payment failed, limited access |
| `trialing` | In free trial period |

### Checking Subscription Status

The API automatically checks subscription status for protected endpoints. Users without active subscriptions receive a `403 Forbidden` response with:

```json
{
  "error": "Active subscription required to access this resource"
}
```

## Webhooks

The API handles Stripe webhooks for automatic subscription updates:

```http
POST /api/v1/subscription/webhook
```

**Supported Events:**
- `checkout.session.completed` - New subscription created
- `customer.subscription.updated` - Subscription updated
- `customer.subscription.deleted` - Subscription cancelled
- `invoice.payment_succeeded` - Payment successful
- `invoice.payment_failed` - Payment failed

## Notifications

Users receive in-app notifications for:
- ✉️ Subscription activated
- ✉️ Subscription status changes
- ✉️ Subscription cancelled
- ✉️ Payment successful
- ✉️ Payment failed
- ✉️ Trial ending soon

View notifications at:
```http
GET /api/v1/notifications
Authorization: <your-token>
```

## FAQ

### Can I switch plans?
Yes, you can upgrade or downgrade at any time through the billing portal.

### What happens when my trial ends?
Your account remains active but premium features are locked until you subscribe.

### Can I get a refund?
Contact support@montessoriworldconnect.com for refund requests.

### Is my payment information secure?
Yes, all payments are processed securely through Stripe. We never store your credit card information.
