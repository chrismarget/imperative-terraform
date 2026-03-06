# Imperative Terraform Design Document

## Overview

The imperative-terraform service allows Terraform provider resource CRUD operations and data source Read() operations to be accessed **without Terraform**. This enables thin wrappers (like Ansible collections) to leverage existing Terraform provider logic.

## Architecture

### Startup Flow

1. Provider binary launched with a CLI argument (e.g. `--imperative-server-mode`) or environment variable to indicate that the provider binary should run in imperative server mode.
2. Bootstrap client (the "patron") sends configuration via stdin:
   - Provider configuration (JSON)
   - Shared secret (for client authentication)
   - Discovery file path (for daemon socket location)
3. Server configures the Terraform provider.
4. Server creates a Unix domain socket where clients will connect.
5. Server announces startup to the bootstrap client via stdout.
6. Server enters accept loop, handling multiple clients.
7. After idle timeout, server initiates shutdown sequence:
   - Delete discover file to prevent new clients from connecting.
   - Continue accepting new client connections for a brief grace period.
   - Close the listening socket to prevent new connections.
   - Wait for existing clients to disconnect.
   - Exit.

### Client Discovery & Authentication

**Discovery Mechanism:**
- The process begins with no server running and no discovery file present.
- One or more clients require service. The clients:
  - May spin up concurrently (parallel exection in a CI job, perhaps), creating a race to start the server.
  - Share a unique identifier (e.g. `ansible_run_id`) that they use to coordinate.
  - Have knowledge of a shared secret for authentication (if enabled).
- The race to start the server begins with a race to create an exclusive lock file (the discovery file) with name based on the shared identifier.
- The race winner, known as the Bootstrap Client creates an empty file at the well-known location. The file serves three purposes:
  - It acts as a lock to ensure only one client attempts to start the service.
  - It serves as a discovery mechanism for other clients to find the path to the server's socket listener.
  - It coordinates graceful shutdown of the service: The server deletes this file during shutdown, signaling to clients that the no service is currently available. Any clients which attempt to discover the service begin a new race, leading to election of a new Bootstrap Client and startup of a new server instance.
- After creating the file, the Bootstrap Client starts the server and passes configuration via stdin.
- The server creates a Unix domain socket listener and writes the socket path to the discovery file. <- TODO
- Other clients (race losers) watch for the socket path to appear in the discovery file, then connect to the server using that path.
- All clients connect to the same daemon socket

**Authentication:**
- HMAC-based challenge-response (if secret provided during startup)
- This is **authorization without identity** - proves caller is part of same workflow.
- Prevents unauthorized access to server empowered to act with external API credentials.

### Shutdown Sequence

The server uses a `shutdown.Controller` to manage graceful shutdown:

1. **Idle Timer (30s)**: After continuous inactivity, delete the discovery file to prevent new clients from connecting.
2. **Grace Period (5s)**: Allow stragglers who read the file just before deletion to connect.
3. **Wait for Clients**: After grace period expires, stop accepting new connections while continuing to serve current clients.
4. **Clean Exit**: When last client disconnects, server exits.

The controller tracks active clients with a WaitGroup and custom counter. Shutdown can also be triggered by:
- SIGTERM / SIGINT (Ctrl-C)
- Context cancellation
- Explicit shutdown signal

**Note**: The discovery file deletion is deliberate - clients arriving after shutdown find no file and become the new "race winner," starting a fresh server instance.

## Protocol

### Wire Format

All messages use JSON with this envelope structure:

```json
{
  "type": "message_type_string",
  "protocol_version": 1,
  "payload": { /* type-specific data */ }
}
```

**Protocol Version**: Currently 1 (hard-coded, validated on all messages)

### Message Types

**Server → Bootstrap Client (stdout):**
- `listening`: Announces socket path and auth requirement

**Client → Server:**
- `challenge_response`: HMAC of server's nonce (if auth enabled)
- `configuration`: Provider config + secret + discovery file (bootstrap client only, via stdin)
- `goodbye`: Clean disconnect signal
- `data_source_read_request`: Data source name + config
- `resource_create_request`: Resource type + config (planned)
- `resource_read_request`: Resource type + state (planned)
- `resource_update_request`: Resource type + config + state (planned)
- `resource_delete_request`: Resource type + state (planned)

**Server → Client:**
- `server_hello`: Lists available resources and data sources
- `challenge`: HMAC challenge nonce (if auth enabled)
- `error`: Error message
- `goodbye`: Acknowledgment of client goodbye
- `data_source_read_response`: Data source state (planned)
- `resource_*_response`: Resource state/ID (planned)

### Connection Lifecycle

1. Client connects to Unix socket
2. [If auth enabled] Server sends `challenge`, client responds with `challenge_response`
3. Server sends `server_hello` with capabilities
4. Client sends request messages, server responds
5. Client sends `goodbye` or closes connection
6. Server detects EOF and cleans up

## Implementation Details

### Buffered Connection Wrapper

`BufferedConn` wraps `net.Conn` with a `bufio.Reader` to prevent `json.Decoder` from greedy-reading. This allows multiple decoders to be created safely without consuming extra bytes from the stream.

### Message Packing/Unpacking

**Two-stage parsing:**
- Stage 1: Decode envelope (type + version) into `message.Message`
- Stage 2: Decode payload into type-specific struct

`message.Read()` handles both:
- If target is `*Message`, only stage 1 is performed
- Otherwise, validates message type matches target struct, then unpacks payload

`message.Write()` auto-detects payload type via reflection and sets the correct message type string.

### Resource/Data Source Filtering

**Allowlists** (hard-coded for now):
```go
var allowedResources = map[string]bool{
    "apstra_datacenter_routing_zone": true,
}

var allowedDataSources = map[string]bool{
    "apstra_datacenter_routing_zone": true,
}
```

Started with routing zone because it's simple. Expect to accommodate most resources/data sources eventually. The allowlist may not be required long term. It was initially added to limit scope and reduce risk while building the first few handlers, but may be removed once we have confidence in the system.

### Error Handling

- **EOF on read**: Client disconnected gracefully - exit handler
- **Other errors**: Log and return (triggers deferred cleanup)
- **Client errors**: Send `error` message, continue connection
- **Auth failures**: Log and close connection

## Known Issues & TODOs

### Current State
- Only data source Read() is partially implemented
- Resource CRUD methods are stubbed
- No tests exist yet
- Bootstrap client (Ansible modules) not yet implemented

### Deliberate Decisions
- Shutdown controller goroutine "leak" is intentional (for now)
- Discovery file is not restored on exit (race winner check handles stale files)
- Umask is set to maximally restrictive without restoration

### Future Work
- [ ] Complete data source Read() implementation
- [ ] Implement resource CRUD handlers
- [ ] Add unit tests for message pack/unpack
- [ ] Add integration tests for connection lifecycle
- [ ] Implement Ansible collection wrapper
- [ ] Consider making allowlists configurable
- [ ] Add metrics/observability
- [ ] Handle provider-specific diagnostics better

## File Structure

```
internal/imperative_server/
├── auth.go                  # HMAC challenge-response
├── buffered_conn.go         # Buffered connection wrapper
├── connection.go            # Connection handler & message loop
├── init.go                  # Provider configuration & startup
├── new.go                   # Server constructor
├── schema.go                # Schema extraction helpers
├── server.go                # Main accept loop & shutdown
├── todo.go                  # TODO markers
├── diags/                   # Diagnostic handling
├── message/
│   ├── message.go           # Wire message envelope
│   ├── messages.go          # Message type definitions
│   ├── read.go              # Inbound message parsing
│   └── write.go             # Outbound message serialization
└── shutdown/
    ├── controller.go        # Shutdown orchestration
    └── new.go               # Controller constructor
```

## Security Considerations

1. **Socket Permissions**: Unix socket created with umask 0077 (owner-only access)
2. **HMAC Authentication**: Optional but recommended - prevents unauthorized local access
3. **No Identity**: Authentication proves workflow membership, not caller identity
4. **Credential Protection**: Server holds API credentials - must not serve unauthorized clients
5. **File-based Discovery**: Lock file must be protected by OS permissions

## Ansible Integration Plan

Each Ansible module corresponds to a Terraform resource type.

**Module behavior:**
1. Race to create exclusive lock file (named with `ansible_run_id`)
2. Winner starts imperative server, passes config via stdin
3. Winner writes socket path (from server's stdout) to lock file
4. Losers read socket path from lock file
5. All modules connect and perform CRUD operations
6. Server shuts down after idle timeout

**Authentication:**
If the Bootstrap client provides a secret, all clients must demonstrate knowledge of that secret via SHA255 HMAC challenge/response immediately upon connection to the server. This ensures only authorized members of the playbook run can interact with the server.
