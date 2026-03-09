---
page_title: "hosting_webapp Resource - terraform-provider-hosting"
subcategory: ""
description: |-
  Manages a web application.
---

# hosting_webapp (Resource)

Manages a web application. Supports PHP, Node.js, Python, Ruby, Go, Java, and static sites.

## Example Usage

```hcl
resource "hosting_webapp" "myapp" {
  tenant_id       = var.tenant_id
  runtime         = "php"
  runtime_version = "8.4"
  public_folder   = "public"

  # WAF protection
  waf_enabled   = true
  waf_mode      = "block"
  waf_exclusions = [942100, 920350]

  # Rate limiting
  rate_limit_enabled = true
  rate_limit_rps     = 100
  rate_limit_burst   = 200
}
```

## Schema

### Required

- `tenant_id` (String) Tenant ID. Changing this forces a new resource.
- `runtime` (String) Runtime type (e.g. php, nodejs, python, ruby, static).
- `runtime_version` (String) Runtime version (e.g. 8.4, 22, 3.13).

### Optional

- `customer_id` (String) Customer ID. Defaults to provider `customer_id`. Changing this forces a new resource.
- `public_folder` (String) Public folder relative to app root (e.g. `public`). Default: `""`.
- `service_hostname_enabled` (Boolean) Whether the built-in service hostname is enabled. Default: `true`.
- `waf_enabled` (Boolean) Enable ModSecurity WAF with OWASP CRS. Default: `false`.
- `waf_mode` (String) WAF mode: `block` (reject malicious requests) or `detect` (log only). Default: `"block"`.
- `waf_exclusions` (List of Number) OWASP CRS rule IDs to exclude. Default: `[]`.
- `rate_limit_enabled` (Boolean) Enable per-IP rate limiting. Default: `false`.
- `rate_limit_rps` (Number) Requests per second per source IP (0-100000). Default: `0`.
- `rate_limit_burst` (Number) Burst size — requests allowed above rate limit. Default: `0`.

### Read-Only

- `id` (String) Webapp ID.
- `env_file_name` (String) Environment file name.
- `status` (String) Current status.

## Import

```shell
terraform import hosting_webapp.myapp wp_abc123
```
