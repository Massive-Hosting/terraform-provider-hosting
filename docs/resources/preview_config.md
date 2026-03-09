---
page_title: "hosting_preview_config Resource - terraform-provider-hosting"
subcategory: ""
description: |-
  Manages a preview environment configuration for a webapp.
---

# hosting_preview_config (Resource)

Manages a preview environment configuration for a webapp. When configured with a GitHub App, pull requests automatically get ephemeral preview environments with their own webapp, optional database, Valkey, S3, and customized environment variables.

## Example Usage

```hcl
resource "hosting_preview_config" "app" {
  webapp_id              = hosting_webapp.app.id
  github_repo_owner      = "my-org"
  github_repo_name       = "my-app"
  github_installation_id = 12345678
  database_mode          = "clone"
  source_database_id     = hosting_database.main.id
  valkey_mode            = "empty"
  auto_destroy_hours     = 48

  env_var_overrides {
    name  = "APP_URL"
    value = "{{PREVIEW_URL}}"
  }
  env_var_overrides {
    name  = "DATABASE_URL"
    value = "mysql://{{DB_USERNAME}}:{{DB_PASSWORD}}@{{DB_HOST}}/{{DB_NAME}}"
  }
}
```

## Schema

### Required

- `webapp_id` (String) Webapp ID. Changing this forces a new resource.
- `github_repo_owner` (String) GitHub repository owner (organization or user).
- `github_repo_name` (String) GitHub repository name.
- `github_installation_id` (Number) GitHub App installation ID.

### Optional

- `enabled` (Boolean) Whether preview environments are enabled. Default: `true`.
- `workflow_filename` (String) Deploy workflow filename. Default: `deploy.yml`.
- `database_mode` (String) Database provisioning mode: `none`, `empty`, or `clone`. Default: `none`.
- `source_database_id` (String) Source database ID for clone mode.
- `valkey_mode` (String) Valkey provisioning mode: `none` or `empty`. Default: `none`.
- `source_valkey_id` (String) Source Valkey instance ID for config reference.
- `s3_mode` (String) S3 provisioning mode: `none` or `empty`. Default: `none`.
- `env_var_overrides` (Block List) Environment variable overrides with template variable support.
  - `name` (String, Required) Environment variable name.
  - `value` (String, Required) Value. Supports template variables: `{{PREVIEW_URL}}`, `{{PR_NUMBER}}`, `{{PR_BRANCH}}`, `{{DB_HOST}}`, `{{DB_NAME}}`, `{{DB_USERNAME}}`, `{{DB_PASSWORD}}`, `{{VALKEY_HOST}}`, `{{VALKEY_PORT}}`, `{{VALKEY_PASSWORD}}`, `{{S3_BUCKET}}`, `{{S3_ACCESS_KEY_ID}}`, `{{S3_SECRET_ACCESS_KEY}}`.
- `auto_destroy_hours` (Number) Hours before preview environments are auto-destroyed. Default: `72`.

### Read-Only

- `id` (String) Preview config ID.
- `status` (String) Current status.

## Import

Import by webapp ID:

```shell
terraform import hosting_preview_config.app <webapp-id>
```
