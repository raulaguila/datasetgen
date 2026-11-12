# Surface: products-crud

<!-- keel:surface products-crud -->

Accepted shape: REST CRUD on `app.product` behind Bearer + Require CRUD bits (option A).

## Decisions

- Physical DELETE
- List hard-capped at 100 (no cursor v1)
- Roles API **out of scope** for this slice
- Routes: GET list/id, POST, PATCH, DELETE with `products:{read,create,update,delete}`

## Ownership

- Domain/use case in `internal/core`
- Repo `adapter/outbound/persistence/postgres/product`
- Handlers under `adapter/inbound/http/rest/product`
