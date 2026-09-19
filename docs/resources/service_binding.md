---
page_title: "hosting_service_binding Resource - terraform-provider-hosting"
subcategory: ""
description: |-
  Connects a website to a container service in the same plan.
---

# hosting_service_binding (Resource)

Connects a website to a container service in the same plan. The service's
address arrives in the website's environment as `PREFIX_HOST`, `PREFIX_PORT`
and `PREFIX_URL`.

The variables are **derived from the binding, not stored**: they are computed
every time the consumer is provisioned, so a service that is renamed, moved to
another node or given a different port stays reachable without a change here.
That is also why the resource has no `value` — there is nothing to update in
place, and every settable attribute forces replacement.

Credentials are deliberately not part of this. The platform does not hold a
container's application secrets; pass those with `hosting_website_env_vars`.

## Example Usage

```hcl
resource "hosting_website" "search" {
  folder  = "meilisearch"
  runtime = "container"

  container {
    image = "getmeili/meilisearch:v1.8"
    ports = [
      { container_port = 7700 },
    ]
    health_check = {
      type = "tcp"
    }
  }
}

resource "hosting_website" "shop" {
  folder          = "shop"
  runtime         = "php"
  runtime_version = "8.3"
}

# shop now has MEILI_HOST=meilisearch, MEILI_PORT=7700 and
# MEILI_URL=http://meilisearch:7700 in its environment.
resource "hosting_service_binding" "shop_search" {
  website_id = hosting_website.shop.id
  target_id  = hosting_website.search.id
  env_prefix = "MEILI"
}
```

## Schema

### Required

- `website_id` (String) The consumer: the website whose environment gains the variables. Changing this forces a new resource.
- `target_id` (String) The service being used — a website with `runtime = "container"`, in the same plan, since a binding produces an address on a per-tenant loopback. Changing this forces a new resource.

### Optional

- `env_prefix` (String) Prefix for the generated variables: `MEILI` gives `MEILI_HOST`, `MEILI_PORT` and `MEILI_URL`. Defaults to the service's own name. Changing this forces a new resource.

### Read-Only

- `id` (String) Binding ID.
- `target_name` (String) The service's internal hostname — the value of `PREFIX_HOST`.

## Notes

- A target with no declared port contributes only `PREFIX_HOST`: the platform
  can say where a service is and declines to guess how to speak to it.
- `PREFIX_PORT` is the **published** port — `host_port` when the mapping sets
  one, `container_port` otherwise — because a consumer connects from outside
  the container.
- A variable the owner sets explicitly with the same name still wins.
- Deleting the target website releases its bindings and re-provisions the
  consumers, so a binding disappearing from state on the next read is an
  ordinary outcome.

## Import

A binding is only addressable through the website that holds it:

```shell
terraform import hosting_service_binding.shop_search <website_id>/<binding_id>
```
