---
page_title: "hosting_webapp_daemon Resource - terraform-provider-hosting"
subcategory: ""
description: |-
  Manages a daemon process for a webapp.
---

# hosting_webapp_daemon (Resource)

Manages a long-running daemon process for a webapp. Daemons can optionally proxy HTTP traffic via a path prefix.

## Example Usage

```hcl
resource "hosting_webapp_daemon" "worker" {
  webapp_id      = hosting_webapp.myapp.id
  command        = "php artisan queue:work --tries=3"
  max_memory_mb  = 256
  stop_signal    = "TERM"
  stop_wait_secs = 30
}

resource "hosting_webapp_daemon" "api" {
  webapp_id  = hosting_webapp.myapp.id
  command    = "node server.js"
  proxy_path = "/api"
  proxy_port = 3000
}
```

## Schema

### Required

- `webapp_id` (String) Webapp ID. Changing this forces a new resource.
- `command` (String) Command to run.

### Optional

- `customer_id` (String) Customer ID. Defaults to provider `customer_id`.
- `proxy_path` (String) URL path prefix to proxy to this daemon (e.g. `/api`). Default: `""`.
- `proxy_port` (Number) Port the daemon listens on for proxied requests. Default: `0`.
- `stop_signal` (String) Signal to send when stopping (`TERM`, `INT`, `QUIT`, `KILL`). Default: `"TERM"`.
- `stop_wait_secs` (Number) Seconds to wait after stop signal before killing. Default: `10`.
- `max_memory_mb` (Number) Memory limit in MB (0 = unlimited). Default: `0`.

### Read-Only

- `id` (String) Daemon ID.
- `enabled` (Boolean) Whether the daemon is enabled.
- `status` (String) Current status.
- `status_message` (String) Status message (set on failure).

## Import

```shell
terraform import hosting_webapp_daemon.worker dm_abc123
```
