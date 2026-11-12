# postgres adapter

Outbound de persistência: conexão, health, `Store` (Tx/UoW) e repos de identidade.

- `Connect` / `MustConnect`: GORM, `PrepareStmt`, pool 50/50, lifetime 1h, logger Silent.
- `HealthChecker.Ping`: `sql.DB.PingContext`.
- `NewStore`: UnitOfWork + `WithinTx`.
- Repos: `user`, `credential`, `session`, `token`, `attempt`, `role`, `outbox`, `rbac` (effective permissions).
- `shared.WrapError`: `ErrRecordNotFound` → `NOT_FOUND`; unique → `ALREADY_EXISTS`; FK → `RESOURCE_IN_USE`; check → `VALIDATION_FAILED`; resto → `DATABASE_ERROR`.
