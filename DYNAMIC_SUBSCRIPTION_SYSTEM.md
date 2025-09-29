# Dynamic Subscription Management System

## Overview

The Montessori World Connect platform now supports a dynamic subscription management system that allows administrators to create, edit, and assign subscription plans to different user roles. This system replaces the previous static subscription model with a flexible, role-based subscription management approach.

## Key Features

### 🎯 For Administrators
- **Create Custom Subscription Plans**: Define subscription plans with custom pricing, features, and billing cycles
- **Role-Based Access Control**: Assign specific subscription plans to different user roles (parent, montessori_professional, institution, etc.)
- **Dynamic Plan Management**: Edit existing plans, activate/deactivate plans, and delete unused plans
- **User Subscription Assignment**: Directly assign subscription plans to specific users with custom durations
- **Comprehensive Management**: View all plans, role mappings, and subscription assignments

### 🔧 For the System
- **Backward Compatibility**: Existing static subscriptions continue to work alongside dynamic plans
- **Flexible Data Model**: Support for both legacy static plans and new dynamic plans
- **Role Validation**: Automatic validation that users can only be assigned plans allowed for their role
- **Usage Protection**: Prevents deletion of subscription plans that are currently in use

## Database Schema Changes

### New Models Added

#### `DynamicSubscriptionPlan`
- `ID`: Unique identifier
- `Name`: Plan name (e.g., "Premium Parent Plan")
- `Description`: Detailed plan description
- `Price`: Plan price in specified currency
- `Currency`: Currency code (default: USD)
- `BillingCycle`: monthly or annual
- `Features`: JSON array of feature strings
- `IsActive`: Boolean flag to enable/disable plan
- `StripePriceID`: Integration with Stripe pricing
- `AllowedRoles`: JSON array of roles that can use this plan
- `CreatedByUserID`: Admin who created the plan
- `CreatedAt/UpdatedAt`: Timestamp tracking

#### `RoleSubscriptionMapping`
- `ID`: Unique identifier
- `Role`: User role (parent, montessori_professional, etc.)
- `SubscriptionPlanID`: Reference to DynamicSubscriptionPlan
- `IsDefault`: Flag to mark default plan for role
- `CreatedAt/UpdatedAt`: Timestamp tracking

#### Updated `Subscription` Model
- Added `DynamicPlanID`: Optional reference to dynamic subscription plan
- Added `DynamicPlan`: Relationship to DynamicSubscriptionPlan
- Maintains backward compatibility with existing `Plan` field

## API Endpoints

All endpoints require admin authentication (`Admin` or `SuperAdmin` role).

### Create Subscription Plan
```http
POST /api/v1/admin/subscription-plans
```

**Request Body:**
```json
{
  "name": "Premium Parent Plan",
  "description": "Advanced features for parents",
  "price": 29.99,
  "currency": "USD",
  "billing_cycle": "monthly",
  "features": [
    "Advanced Search",
    "Direct Messaging",
    "Priority Support",
    "Premium Content"
  ],
  "allowed_roles": ["parent", "montessori_professional"],
  "stripe_price_id": "price_1234567890"
}
```

### Get All Subscription Plans
```http
GET /api/v1/admin/subscription-plans
```

### Update Subscription Plan
```http
PUT /api/v1/admin/subscription-plans/{id}
```

**Request Body:**
```json
{
  "name": "Premium Parent Plan Updated",
  "description": "Updated premium features",
  "price": 39.99,
  "currency": "USD",
  "billing_cycle": "monthly",
  "features": [
    "Advanced Search",
    "Direct Messaging",
    "Priority Support",
    "Premium Content",
    "Analytics Dashboard"
  ],
  "allowed_roles": ["parent", "montessori_professional"],
  "is_active": true
}
```

### Delete Subscription Plan
```http
DELETE /api/v1/admin/subscription-plans/{id}
```

### Get Role Subscription Mappings
```http
GET /api/v1/admin/role-subscriptions?role=parent
```

### Assign Subscription to User
```http
POST /api/v1/admin/assign-subscription
```

**Request Body:**
```json
{
  "user_id": 123,
  "subscription_plan_id": 456,
  "duration_months": 12
}
```

## Usage Examples

### Creating a Role-Specific Plan

```bash
# Create a plan specifically for parents
curl -X POST http://localhost:8080/api/v1/admin/subscription-plans \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Parent Premium",
    "description": "Premium features for parents seeking the best schools",
    "price": 19.99,
    "currency": "USD",
    "billing_cycle": "monthly",
    "features": [
      "Advanced School Search",
      "Direct School Communication",
      "Priority Support",
      "Exclusive Parent Resources"
    ],
    "allowed_roles": ["parent"]
  }'
```

### Creating a Multi-Role Plan

```bash
# Create a plan for multiple roles
curl -X POST http://localhost:8080/api/v1/admin/subscription-plans \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Professional Plus",
    "description": "Advanced features for education professionals",
    "price": 49.99,
    "currency": "USD",
    "billing_cycle": "monthly",
    "features": [
      "Professional Networking",
      "Advanced Job Matching",
      "Institution Analytics",
      "Premium Content Library",
      "Priority Support"
    ],
    "allowed_roles": ["montessori_professional", "institution", "training_center"]
  }'
```

### Assigning a Subscription to a User

```bash
# Assign a 6-month subscription to user
curl -X POST http://localhost:8080/api/v1/admin/assign-subscription \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": 123,
    "subscription_plan_id": 456,
    "duration_months": 6
  }'
```

## Integration with Existing Systems

### Stripe Integration
- Dynamic subscription plans can include `stripe_price_id` for seamless Stripe integration
- Existing Stripe webhook handling remains unchanged
- New plans can be created in Stripe and referenced in dynamic plans

### User Authentication & Authorization
- All existing authentication middleware continues to work
- Subscription middleware now checks both static and dynamic subscriptions
- Role-based access control validates user roles against plan allowed roles

### Backward Compatibility
- Existing users with static subscriptions (free, monthly, annual) continue to work
- System supports both static and dynamic subscription plans simultaneously
- Migration path available for converting static subscriptions to dynamic plans

## Security Considerations

### Admin Authorization
- All subscription management endpoints require admin or superadmin roles
- Role validation prevents unauthorized subscription assignments
- Audit logging tracks all subscription management actions

### Data Validation
- Price validation ensures positive values
- Role validation against defined UserRole constants
- Subscription plan usage protection prevents deletion of active plans

### User Role Validation
- System validates that users can only receive subscriptions for their role
- Prevents privilege escalation through subscription assignment
- Maintains role-based access control integrity

## Testing

A comprehensive test suite has been created (`tests/test_dynamic_subscriptions.go`) that validates:

1. **Admin Authentication**: Proper admin token handling
2. **Plan Creation**: Creating subscription plans with role restrictions
3. **Plan Management**: Retrieving, updating, and deleting plans
4. **Role Mappings**: Validating role-to-subscription relationships
5. **User Assignment**: Assigning subscriptions with role validation
6. **Data Protection**: Preventing deletion of in-use plans

To run the tests:

```bash
# Start the server first
go run main.go

# In another terminal, run the tests
cd tests
go run test_dynamic_subscriptions.go
```

## Migration Guide

### From Static to Dynamic Subscriptions

1. **Create Dynamic Plans**: Use the admin API to create dynamic plans that match your current static plans
2. **Map User Roles**: Assign appropriate roles to each dynamic plan
3. **Migrate Users**: Gradually move users from static to dynamic subscriptions
4. **Update Frontend**: Modify admin interfaces to use the new dynamic subscription APIs
5. **Update Billing**: Integrate new dynamic plans with Stripe or your payment processor

### Example Migration Script

```go
// Migrate existing "monthly" static subscriptions to dynamic plan
func migrateToDynamicPlan(db *gorm.DB, staticPlan string, dynamicPlanID uint) error {
    return db.Model(&models.Subscription{}).
        Where("plan = ? AND dynamic_plan_id IS NULL", staticPlan).
        Update("dynamic_plan_id", dynamicPlanID).Error
}
```

## Admin Interface Integration

### Recommended UI Features

1. **Subscription Plan Dashboard**
   - List all dynamic subscription plans
   - Create/Edit/Delete plan functionality
   - Plan usage statistics

2. **Role Management Interface**
   - View role-to-plan mappings
   - Assign/remove roles from plans
   - Default plan settings per role

3. **User Subscription Management**
   - Search and assign subscriptions to users
   - View user subscription history
   - Bulk subscription operations

4. **Analytics Dashboard**
   - Subscription plan performance metrics
   - Role-based subscription analytics
   - Revenue tracking by plan

## Future Enhancements

### Planned Features
- **Plan Templates**: Pre-defined plan templates for common use cases
- **Bulk Operations**: Bulk assignment/removal of subscriptions
- **Subscription Scheduling**: Schedule subscription changes for future dates
- **Usage-Based Billing**: Support for usage-based pricing models
- **Plan Versioning**: Version control for subscription plan changes
- **Advanced Analytics**: Detailed subscription and revenue analytics

### API Versioning
- Current implementation is v1-compatible
- Future versions will maintain backward compatibility
- Deprecation notices will be provided for any breaking changes

## Support and Troubleshooting

### Common Issues

1. **Role Validation Errors**: Ensure user roles match the allowed roles in subscription plans
2. **Plan Deletion Failures**: Check if plans are in use before attempting deletion
3. **Authentication Issues**: Verify admin tokens are valid and have appropriate permissions

### Debug Mode
Enable debug logging to troubleshoot subscription management issues:

```go
// Add to your config for detailed logging
cfg.Debug = true
```

This dynamic subscription system provides the flexibility needed to manage complex subscription scenarios while maintaining backward compatibility and security best practices.