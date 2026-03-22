---
page_title: "Hosting Provider"
subcategory: ""
description: |-
  Manage hosting resources (websites, databases, DNS, containers, etc.) via the control panel API.
---

# Hosting Provider

The Hosting provider manages infrastructure on the Massive Hosting platform. It supports websites, databases, DNS zones, S3 storage, email, containers, VPN peers, and more.

## Authentication

The provider authenticates using a Personal Access Token (PAT). Create one from your profile page in the control panel.

## Example Usage

```hcl
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
```

## Configuration

### Required

- `api_url` (String) Base URL of the control panel API. Can also be set via `HOSTING_API_URL` environment variable.
- `token` (String, Sensitive) Personal Access Token for API authentication. Can also be set via `HOSTING_TOKEN` environment variable.

### Optional

- `customer_id` (String) Default customer ID for resource creation. Can also be set via `HOSTING_CUSTOMER_ID` environment variable. If set, individual resources don't need to specify `customer_id`.

## Async Resources

Many resources provision asynchronously. The provider automatically polls until the resource reaches `active` status (default timeout: 5 minutes).

## Import

Most resources support `terraform import`. See individual resource docs for the import ID format.

## Export

The control panel includes a Terraform export feature that generates a `.tf` file from existing resources:

```
GET /api/v1/customers/{cid}/terraform/export?tenant_id=...
```

This produces a complete configuration with cross-references and import commands as comments.
