# ADR-001 Hexagonal + Fiber

Status: aceito (intenção). Implementação: parcial.

API REST em arquitetura hexagonal. HTTP em Fiber v3 (timeouts em `APIConfig`), Bearer em `/v1`.

Core isolado de framework. Handlers só falam com input ports.

Ainda não há Fiber no `go.mod` nem listen no `cmd/backend`.
