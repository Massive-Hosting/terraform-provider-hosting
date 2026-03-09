terraform {
  required_providers {
    hosting = {
      source = "massive-hosting/hosting"
    }
  }
}

provider "hosting" {
  api_url     = "https://home.massive-hosting.com"
  token       = var.hosting_token
  customer_id = var.customer_id
}

variable "hosting_token" {
  type      = string
  sensitive = true
}

variable "customer_id" {
  type = string
}

variable "tenant_id" {
  type = string
}

# ─── Web Application ───────────────────────────────────────────────

resource "hosting_webapp" "myapp" {
  tenant_id       = var.tenant_id
  runtime         = "php"
  runtime_version = "8.4"
  public_folder   = "public"

  # WAF protection (ModSecurity + OWASP CRS)
  waf_enabled    = true
  waf_mode       = "block"
  waf_exclusions = [942100]

  # Rate limiting (per source IP)
  rate_limit_enabled = true
  rate_limit_rps     = 100
  rate_limit_burst   = 200
}

resource "hosting_webapp_env_vars" "myapp" {
  webapp_id = hosting_webapp.myapp.id

  vars = {
    APP_ENV   = "production"
    APP_DEBUG = "false"
    APP_URL   = "https://myapp.example.com"
  }

  secret_vars = {
    APP_KEY     = var.app_key
    DB_PASSWORD = hosting_database_user.myuser.password
  }
}

resource "hosting_webapp_daemon" "worker" {
  webapp_id     = hosting_webapp.myapp.id
  command       = "php artisan queue:work --tries=3"
  num_procs     = 2
  max_memory_mb = 256
}

resource "hosting_webapp_cron_job" "scheduler" {
  webapp_id       = hosting_webapp.myapp.id
  schedule        = "* * * * *"
  command         = "php artisan schedule:run"
  timeout_seconds = 60
}

# ─── Database ──────────────────────────────────────────────────────

resource "hosting_database" "mydb" {
  tenant_id = var.tenant_id
}

resource "hosting_database_user" "myuser" {
  database_id = hosting_database.mydb.id
  privileges  = ["ALL"]
}

# ─── Custom Domain & SSL ──────────────────────────────────────────

resource "hosting_fqdn" "main" {
  fqdn        = "myapp.example.com"
  webapp_id   = hosting_webapp.myapp.id
  ssl_enabled = true
}

# ─── DNS ───────────────────────────────────────────────────────────

resource "hosting_dns_zone" "main" {
  domain = "example.com"
}

resource "hosting_dns_record" "root" {
  zone_id = hosting_dns_zone.main.id
  type    = "A"
  name    = "@"
  content = "10.10.10.70"
  ttl     = 3600
}

resource "hosting_dns_record" "www" {
  zone_id = hosting_dns_zone.main.id
  type    = "CNAME"
  name    = "www"
  content = "myapp.example.com"
  ttl     = 3600
}

# ─── S3 Storage ────────────────────────────────────────────────────

resource "hosting_s3_bucket" "assets" {
  tenant_id   = var.tenant_id
  public      = true
  quota_bytes = 10737418240 # 10 GB
}

resource "hosting_s3_access_key" "app" {
  s3_bucket_id = hosting_s3_bucket.assets.id
  permissions  = ["read", "write"]
}

# ─── Valkey (Redis) ───────────────────────────────────────────────

resource "hosting_valkey" "cache" {
  tenant_id     = var.tenant_id
  max_memory_mb = 128
}

resource "hosting_valkey_user" "app" {
  valkey_instance_id = hosting_valkey.cache.id
  privileges         = ["allcommands"]
  key_pattern        = "*"
}

# ─── Email ─────────────────────────────────────────────────────────

resource "hosting_email_account" "info" {
  fqdn_id  = hosting_fqdn.main.id
  address  = "info"
  password = var.email_password
}

resource "hosting_email_alias" "contact" {
  email_account_id = hosting_email_account.info.id
  address          = "contact"
}

resource "hosting_email_forward" "backup" {
  email_account_id = hosting_email_account.info.id
  destination      = "backup@gmail.com"
  keep_copy        = true
}

# ─── Preview Environments ──────────────────────────────────────────

resource "hosting_preview_config" "myapp" {
  webapp_id              = hosting_webapp.myapp.id
  github_repo_owner      = "my-org"
  github_repo_name       = "my-app"
  github_installation_id = 12345678
  database_mode          = "clone"
  source_database_id     = hosting_database.main.id
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

# ─── Container ─────────────────────────────────────────────────────

resource "hosting_container" "meilisearch" {
  tenant_id     = var.tenant_id
  name          = "meilisearch"
  image         = "getmeili/meilisearch:v1.6"
  max_memory_mb = 512
}

resource "hosting_container_env_vars" "meilisearch" {
  container_id = hosting_container.meilisearch.id

  secret_vars = {
    MEILI_MASTER_KEY = var.meili_key
  }
}

# ─── Uptime Monitoring ─────────────────────────────────────────────

resource "hosting_uptime_monitor" "main" {
  tenant_id        = var.tenant_id
  url              = "https://myapp.example.com/healthz"
  interval_seconds = 60
  timeout_seconds  = 10
  expected_status  = 200
}

# ─── Webhooks ──────────────────────────────────────────────────────

resource "hosting_webhook_endpoint" "deploy_notifications" {
  tenant_id   = var.tenant_id
  url         = "https://myapp.example.com/webhooks/hosting"
  description = "Deploy notifications"
  events      = ["deploy.success", "deploy.failed"]
}

# ─── Networking ────────────────────────────────────────────────────

resource "hosting_wireguard_peer" "dev" {
  tenant_id = var.tenant_id
  name      = "dev-laptop"
}

resource "hosting_ssh_key" "deploy" {
  tenant_id  = var.tenant_id
  name       = "deploy-key"
  public_key = file("~/.ssh/id_ed25519.pub")
}

resource "hosting_egress_rule" "api" {
  tenant_id   = var.tenant_id
  cidr        = "203.0.113.0/24"
  description = "Allow access to external API"
}

# ─── Variables ─────────────────────────────────────────────────────

variable "app_key" {
  type      = string
  sensitive = true
}

variable "email_password" {
  type      = string
  sensitive = true
}

variable "meili_key" {
  type      = string
  sensitive = true
}

# ─── Outputs ───────────────────────────────────────────────────────

output "webapp_status" {
  value = hosting_webapp.myapp.status
}

output "database_user" {
  value     = hosting_database_user.myuser.username
  sensitive = false
}

output "s3_credentials" {
  value = {
    access_key = hosting_s3_access_key.app.access_key_id
    secret_key = hosting_s3_access_key.app.secret_access_key
  }
  sensitive = true
}

output "wireguard_config" {
  value     = hosting_wireguard_peer.dev.client_config
  sensitive = true
}
