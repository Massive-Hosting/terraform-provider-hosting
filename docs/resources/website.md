---
page_title: "hosting_website Resource - terraform-provider-hosting"
subcategory: ""
description: |-
  Manages a web application.
---

# hosting_website (Resource)

Manages a website. Supports PHP, Node.js, Python, Ruby and static sites, and
OCI containers — a container is a website whose `runtime` is `container`.

## Example Usage

```hcl
resource "hosting_website" "myapp" {
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

### A container

A container is the same resource with a different runtime. Its image, limits
and ports go in the `container` block, which is stored as the website's
runtime_config; everything else — domains, env vars, status — works exactly as
it does for any other runtime.

```hcl
resource "hosting_website" "api" {
  tenant_id = var.tenant_id
  folder    = "metrics-api"
  runtime   = "container"

  container {
    image          = "ghcr.io/you/metrics-api:1.8.2"
    command        = "npm start"
    restart_policy = "always"
    max_memory_mb  = 512
    max_cpu_cores  = 1

    ports = [
      { container_port = 8080, protocol = "tcp" },
    ]

    volumes = [
      { host_path = "data", container_path = "/app/data", read_only = false },
    ]
  }
}

resource "hosting_website_env_vars" "api" {
  website_id = hosting_website.api.id
  vars       = { LOG_LEVEL = "info" }
  secret_vars = { API_TOKEN = var.api_token }
}
```

## Schema

### Required

- `tenant_id` (String) Tenant ID. Changing this forces a new resource.
- `runtime` (String) Runtime type: `php`, `node`, `python`, `ruby`, `static` or `container`.

### Optional

- `runtime_version` (String) Runtime version (e.g. `8.4`, `22`, `3.13`). Not used by the `container` runtime, which is versioned by its image tag. Default: `""`.
- `container` (Attributes) Container settings. Required when `runtime` is `container`, ignored otherwise. Stored as the website's runtime_config.
  - `image` (String, Required) Image reference including the tag.
  - `command` (String) Overrides the image ENTRYPOINT.
  - `image_pull_secret_id` (String) Registry credential for a private image.
  - `restart_policy` (String) `always`, `on-failure`, `unless-stopped` or `no`. Default: `"always"`.
  - `max_memory_mb` (Number) Hard memory ceiling. A container that exceeds it is killed with exit code 137. Default: `512`.
  - `max_cpu_cores` (Number) CPU ceiling in cores. Default: `1.0`.
  - `proxy_path` (String) Serve the container under this path on the node, for a container with no domain of its own.
  - `proxy_port` (Number, Read-Only) The host port the platform derives.
  - `ports` (Attributes List) `container_port` (Required), `host_port` (Optional, defaults to `container_port`) and `protocol` (`tcp`/`udp`). `host_port` is the port published on the workload's internal address — what a connected website reaches the service on, and what the generated `_PORT` variable and the health check use.
  - `volumes` (Attributes List) `host_path`, `container_path` (both Required) and `read_only`.
- `enabled` (Boolean) Whether the workload should be running. Only the container runtime acts on it. Default: `true`.

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

- `id` (String) Website ID.
- `status` (String) Current status.

## Import

```shell
terraform import hosting_website.myapp wp_abc123
```
