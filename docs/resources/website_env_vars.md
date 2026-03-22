---
page_title: "hosting_website_env_vars Resource - terraform-provider-hosting"
subcategory: ""
description: |-
  Manages environment variables for a website.
---

# hosting_website_env_vars (Resource)

Manages environment variables for a website. This resource manages ALL env vars for the website -- any vars not included will be removed.

~> **Note:** This resource replaces all environment variables on the website. Variables not specified in `vars` or `secret_vars` will be deleted.

## Example Usage

```hcl
resource "hosting_website_env_vars" "myapp" {
  website_id = hosting_website.myapp.id

  vars = {
    APP_ENV   = "production"
    APP_DEBUG = "false"
    APP_URL   = "https://myapp.example.com"
  }

  secret_vars = {
    APP_KEY    = var.app_key
    DB_PASSWORD = var.db_password
  }
}
```

## Schema

### Required

- `website_id` (String) Website ID. Changing this forces a new resource.

### Optional

- `customer_id` (String) Customer ID. Defaults to provider `customer_id`.
- `vars` (Map of String) Non-secret environment variables (name -> value).
- `secret_vars` (Map of String, Sensitive) Secret environment variables (name -> value). Values are encrypted server-side and cannot be read back.

### Read-Only

- `id` (String) Resource ID (same as website_id).
