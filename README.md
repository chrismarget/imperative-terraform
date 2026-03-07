# Imperative Terraform

Imperative Terraform exposes Terraform provider resource CRUD and data source Read operations over a local Unix socket so other tools (e.g., Ansible) can call provider logic without running Terraform itself.

This module is strictly a Go library -- it needs to be embedded in a build of a Terraform provider and triggered at startup via a command-line flag or environment variable.

## Status

Work in progress. Data source reads are wired up, but interactive message handling and resource CRUD are not complete yet. See `DESIGN.md` and `IMPLEMENTATION_NOTES.md` for details.

## How It Works (High-Level)

- A bootstrap client starts the provider in server mode and sends configuration via stdin.
- The server announces its Unix socket path via stdout.
- Clients connect to the socket, authenticate (optional HMAC), and send JSON messages.
- The server dispatches requests to provider data sources/resources.

## Protocol

Messages use a JSON envelope:

```json
{
  "type": "message_type",
  "protocol_version": 1,
  "payload": {}
}
```

See `DESIGN.md` for full message types and lifecycle.

## Repository Layout

- `server/` — Core server, connection handling, provider wiring.
- `message/` — Message envelopes and payload types.
- `shutdown/` — Graceful shutdown controller.
- `internal/` — Small shared utilities.

## Next Steps

- Complete data source responses and resource CRUD handlers.
- Implement client (e.g., Ansible collection) to manage bootstrap/discovery.
- Add unit and integration tests.
