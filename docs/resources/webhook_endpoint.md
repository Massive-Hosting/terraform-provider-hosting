# hosting_webhook_endpoint

Manages a webhook endpoint that receives HMAC-signed HTTP notifications for infrastructure events such as deployments, backups, certificate expiry, and cron failures.

## Example Usage

```hcl
resource "hosting_webhook_endpoint" "deploy" {
  tenant_id   = "my-tenant"
  url         = "https://myapp.example.com/webhooks/hosting"
  description = "Deploy notifications"
  events      = ["deploy.success", "deploy.failed"]
}
```

## Argument Reference

- `tenant_id` - (Required, Forces new resource) The tenant ID.
- `url` - (Required) The URL to receive webhook POST requests.
- `description` - (Optional) Human-readable description.
- `events` - (Required) List of event types to subscribe to. Valid values: `deploy.success`, `deploy.failed`, `backup.completed`, `ssl.expiring`, `cron.failed`, `webapp.status_changed`.
- `enabled` - (Optional, Default: `true`) Whether the endpoint is enabled.
- `customer_id` - (Optional) Customer ID. Falls back to provider-level `customer_id`.

## Attribute Reference

- `id` - The webhook endpoint ID.
- `secret` - (Sensitive) The HMAC signing secret. Only available after create; not returned by read.
- `created_at` - Creation timestamp.

## Import

```shell
terraform import hosting_webhook_endpoint.deploy <endpoint-id>
```

Note: The `secret` attribute will not be populated after import since it is not returned by the API on read.
