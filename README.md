# Terraform Provider: Hosting Platform

Terraform provider for managing resources on the [Hosting Platform](https://massive-hosting.com). Supports webapps, databases, DNS, S3 storage, email, containers, VPN, and more.

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- A Personal Access Token (PAT) from the control panel

## Installation

```hcl
terraform {
  required_providers {
    hosting = {
      source = "massive-hosting/hosting"
    }
  }
}
```

## Configuration

```hcl
provider "hosting" {
  api_url     = "https://home.massive-hosting.com"  # or HOSTING_API_URL env var
  token       = var.hosting_token                   # or HOSTING_TOKEN env var
  customer_id = var.customer_id                     # or HOSTING_CUSTOMER_ID env var
}
```

## Quick Start

```hcl
# Create a PHP webapp
resource "hosting_webapp" "myapp" {
  tenant_id       = var.tenant_id
  runtime         = "php"
  runtime_version = "8.4"
  public_folder   = "public"
}

# Create a database
resource "hosting_database" "mydb" {
  tenant_id = var.tenant_id
}

resource "hosting_database_user" "myuser" {
  database_id = hosting_database.mydb.id
  privileges  = ["ALL"]
}

# Set environment variables
resource "hosting_webapp_env_vars" "myapp" {
  webapp_id = hosting_webapp.myapp.id

  vars = {
    APP_ENV = "production"
    APP_URL = "https://myapp.example.com"
  }

  secret_vars = {
    DB_PASSWORD = hosting_database_user.myuser.password
  }
}

# Attach a custom domain with SSL
resource "hosting_fqdn" "main" {
  fqdn        = "myapp.example.com"
  webapp_id   = hosting_webapp.myapp.id
  ssl_enabled = true
}
```

## Resources

| Resource | Description |
|---|---|
| `hosting_webapp` | Web application (PHP, Node.js, Python, Ruby, Go, static) |
| `hosting_webapp_env_vars` | Webapp environment variables (bulk) |
| `hosting_webapp_daemon` | Long-running daemon process |
| `hosting_webapp_cron_job` | Scheduled cron job |
| `hosting_database` | MySQL database |
| `hosting_database_user` | MySQL database user |
| `hosting_valkey` | Valkey (Redis) instance |
| `hosting_valkey_user` | Valkey user |
| `hosting_fqdn` | Custom hostname / domain binding |
| `hosting_dns_zone` | DNS zone (with domain verification) |
| `hosting_dns_record` | DNS record |
| `hosting_s3_bucket` | S3 storage bucket |
| `hosting_s3_access_key` | S3 access key |
| `hosting_email_account` | Email account |
| `hosting_email_alias` | Email alias |
| `hosting_email_forward` | Email forward rule |
| `hosting_container` | OCI container |
| `hosting_container_env_vars` | Container environment variables (bulk) |
| `hosting_wireguard_peer` | WireGuard VPN peer |
| `hosting_ssh_key` | SSH public key |
| `hosting_egress_rule` | Egress firewall rule |
| `hosting_preview_config` | Preview environment configuration |
| `hosting_uptime_monitor` | HTTP uptime health check |
| `hosting_webhook_endpoint` | Webhook endpoint for event notifications |

See the [docs/](docs/) directory for full documentation on each resource.

## Import

Most resources support `terraform import`:

```bash
terraform import hosting_webapp.myapp wp_abc123
terraform import hosting_database.mydb db_xyz789
terraform import hosting_database_user.myuser db_xyz789/dbu_123456
```

## Terraform Export

The control panel can generate a complete `.tf` file from your existing resources. Use the "Export Terraform" button on the subscription detail page, or call the API directly:

```
GET /api/v1/customers/{cid}/terraform/export?tenant_id=...
```

## Development

```bash
go build ./...
go vet ./...
```

For local testing, add a `replace` directive in `go.mod`:

```
replace github.com/massive-hosting/go-hosting => ../go-hosting
```

## License

MIT
