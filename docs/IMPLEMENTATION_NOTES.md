# Implementation Notes & Decisions

This document captures specific implementation choices, gotchas, and learnings from the development process.

## Architecture Decisions

### Why BufferedConn?

**Problem**: `json.Decoder` can greedy-read from the underlying reader, consuming more bytes than needed for a single message. This breaks when you want to create a new decoder for each message in a loop.

**Solution**: Wrap `net.Conn` with `BufferedConn` that embeds a `bufio.Reader`. The buffered reader preserves unprocessed data between decoder creations.

```go
type BufferedConn struct {
    net.Conn
    r *bufio.Reader
}
```

**Trade-off**: Extra allocation, but necessary for safe multi-message parsing.

---

### Two-Stage Message Parsing

**Problem**: The message handler loop doesn't know what message type to expect - several different types could arrive. But we want type-safe unmarshaling into concrete structs.

**Solution**: Split parsing into two stages:
1. **Envelope decode**: Parse `type` and `protocol_version`, leave `payload` as `json.RawMessage`
2. **Payload decode**: Based on `type`, unmarshal `payload` into the correct struct

**Implementation**:
- `message.Read(*Message)` - stage 1 only (when target is `*Message`)
- `message.Read(concreteType)` - both stages (validates type matches expected)
- `message.UnpackPayload(target, raw)` - stage 2 only (for manual dispatch)

**Why this works**: 
- Known-type flows (like reading Config from stdin) pass the concrete type and get full validation
- Generic handler loops read the envelope first, dispatch on `msg.Type`, then unpack the payload

---

### Shutdown Controller Design

**Requirements**:
- Idle timeout (30s of no clients)
- Grace period (5s for stragglers)
- Track active client count
- Delete discovery file at idle timeout
- Call stop function when truly idle (after grace + all clients gone)

**Key insight**: This isn't a general-purpose activity controller. It's specifically a shutdown controller, so we can bake the logic in rather than being abstract.

**Components**:
- `sync.WaitGroup` for client tracking (`NewClient()` adds, `ClientDone()` decrements)
- Custom counter that fires callback on decrement-to-zero (after grace timer)
- Two timers: idle and grace
- Lock file deletion at idle timeout

**Sequence**:
1. Server starts, idle timer (30s) starts
2. Client connects → idle timer resets
3. Client disconnects → idle timer restarts
4. 30s pass with no clients → delete discovery file, start grace timer (5s)
5. Grace timer expires → close shutdown channel (signals accept loop to stop)
6. Accept loop stops accepting new connections
7. When last client disconnects → `wg.Wait()` completes → `stopFunc()` called → main exits

**Race condition handled**: Client reads lock file → we delete it → client connects during grace period. This is fine - grace period exists exactly for this scenario. The client gets served normally.

**Deliberate choice**: Clients who arrive *after* the lock file is deleted will not find the socket path. They become the new "race winner" and start a fresh server instance. This is the intended behavior.

---

### Authentication: Identity-less Authorization

**Question**: Is HMAC challenge-response authentication or authorization?

**Answer**: It's **authorization without identity**. The server doesn't know *who* the client is, only that they possess the shared secret, proving they're part of the same workflow.

**Real-world analogies**: 
- WiFi password (you're on the network, but the router doesn't know your name)
- Game lobby codes (you can join, but you're just "Player 3")
- Shared API keys (authorization without user identity)

**Why this matters**: The server has likely been configured with credentials for an external API. It must not perform actions for unauthorized callers. Demonstrating knowledge of the shared secret is sufficient authorization.

**How it works**:
1. Server generates random nonce
2. Server sends `challenge` message with nonce
3. Client computes HMAC(secret, nonce)
4. Client sends `challenge_response` with HMAC
5. Server verifies HMAC matches expected value
6. If match: connection continues; if not: connection closes

---

### Umask Handling

**Problem**: Need to set umask to 0077 (owner-only socket access) without making the system less secure.

**Naive approach**: `syscall.Umask(0077)` - but this discards the old value, which might have been MORE restrictive.

**Better approach**: 
```go
old := syscall.Umask(0o7777)  // Get current (temporarily sets to max paranoid)
syscall.Umask(old | 0o0077)    // Bitwise OR to ensure our bits are set
```

This ensures we're at *least* as restrictive as 0077, but if the system was more restrictive (e.g., 0177), we preserve that.

**Decision**: Don't restore the old umask on exit. This process only creates one file (the socket), and we're setting it MORE restrictive. Restoring a possibly-less-secure value is nonsense. If this process was setting the umask *less* secure, we'd restore it, but we're not.

**Note**: `syscall.Umask()` is Unix-only. On Windows, this code path doesn't execute (Windows doesn't use Unix domain sockets in the same way).

---

### EOF Handling

**Problem**: When a client closes the connection, `json.Decoder.Decode()` returns `io.EOF`. This was causing an infinite loop where the error was logged but the handler continued.

**Solution**:
```go
if err == io.EOF {
    // Client closed the connection gracefully
    return
}
s.logFunc("connection: reading client message: %v", err)
return
```

**Key**: EOF is not an error - it's the expected signal that the client has disconnected. Don't log it, just return. The deferred `ClientDone()` will clean up properly.

---

## Protocol Decisions

### Why Protocol Version in Every Message?

**Reason**: Allows server to detect version mismatches immediately on first message. Client doesn't need a separate handshake - any message will be validated.

**Implementation**: 
- `protocolVersion = 1` (constant)
- Every message (in both directions) includes `"protocol_version": 1`
- Both `Read()` and `decodeEnvelope()` validate the version

**Note**: Version 0 is not valid. Clients must explicitly specify version 1. This catches uninitialized/missing values.

**Future**: When we bump to version 2, the server can decide whether to support both or reject v1 clients.

---

### Message Type Field Name

**Decision**: Use `type` not `message_type` or `msg_type`.

**Rationale**: Shorter, follows JSON conventions (many protocols use `type` as discriminator).

**JSON structure**:
```json
{
  "type": "challenge",
  "protocol_version": 1,
  "payload": {
    "nonce": "base64-encoded-bytes"
  }
}
```

---

### Goodbye Message

**Purpose**: Allows client to signal clean disconnect (vs. just closing the socket).

**Implementation**: Empty struct - no payload needed.

```go
type Goodbye struct{}
```

**Handling**: 
- Client sends goodbye message
- Server can respond with goodbye acknowledgment (optional)
- Server returns from handler (triggers deferred cleanup)

**Note**: EOF handling is separate - handles ungraceful disconnect (client crashes, network issue, etc.).

---

### Message Type Constants & Auto-wiring

**Pattern**: `TypeXxx` constants map to `Xxx` struct types.

```go
const TypeConfig = "configuration"
type Config struct { 
    ProviderConfig    json.RawMessage `json:"provider_config"`
    Secret            []byte          `json:"secret"`
    DiscoveryFilePath string          `json:"discovery_file_path"`
}
```

**Auto-wiring via reflection**: 
- `payloadTypes` map: `string → *StructType` (for Read validation)
- `typeToMessageType` map: `reflect.Type → string` (for Write type detection)

Both maps are built automatically at `init()` time, so adding a new message type requires:
1. Define the constant
2. Define the struct
3. Add to `payloadTypes` map

Everything else is automatic.

---

### Why json.RawMessage for Payload?

**Purpose**: Defer parsing until we know the expected type.

**How it works**:
- `Message.Payload` is `json.RawMessage` (just `[]byte` with special handling)
- Stage 1 decode leaves it as raw bytes
- Stage 2 decode unmarshals it into the concrete type

**Alternative considered**: Use `any` and type assertions. Rejected because:
- Loses type safety
- Requires reflection at runtime
- Can't validate before unmarshaling

---

## Error Handling Patterns

### EOF is Not an Error

```go
if err == io.EOF {
    return  // Client closed gracefully
}
```

**Why**: EOF on read means the client disconnected. This is expected behavior, not an error. Logging it would spam the logs on every normal disconnect.

---

### Listener Close Errors

When closing a listener that might already be closed:

```go
if err := listener.Close(); err != nil && 
   !errors.Is(err, net.ErrClosed) && 
   !errors.Is(err, os.ErrClosed) {
    s.logFunc("closing listener: %v", err)
}
```

**Why**: Multiple code paths can close the listener (shutdown signal, context cancel, grace timeout). `ErrClosed` is expected, but other errors might indicate problems worth logging.

---

### Send Errors During Shutdown

If sending a message fails during connection handling, log but don't panic:

```go
if err := message.Write(conn, &response); err != nil {
    s.logFunc("connection: sending response: %v", err)
    return  // Client probably disconnected
}
```

**Why**: The client might have disconnected while we were processing. This is fine - the deferred cleanup will handle it.

---

## Concurrency Patterns

### Accept Loop with Select

**Problem**: `listener.Accept()` blocks indefinitely. Need to check shutdown channel, context, and signals.

**Solution**: Set short deadline on Accept, use `default` case in select:

```go
for {
    select {
    case <-s.sc.Shutdown():
        s.closeListener(listener)
        s.sc.Wait()
        return nil
    case <-ctx.Done():
        s.closeListener(listener)
        s.sc.Wait()
        return ctx.Err()
    case sig := <-sigCh:
        s.closeListener(listener)
        s.logFunc("server: signal %s received: shutting down...", sig)
        s.sc.Wait()
        return nil
    default:
        listener.SetDeadline(time.Now().Add(100 * time.Millisecond))
        conn, acceptErr := listener.Accept()
        if netErr, ok := acceptErr.(net.Error); ok && netErr.Timeout() {
            continue  // Expected timeout, loop again
        }
        // Handle connection...
    }
}
```

**Trade-off**: Small latency on shutdown (up to 100ms), but avoids complexity of goroutine-per-accept patterns.

**Why 100ms**: Fast enough for responsive shutdown, slow enough to not burn CPU.

---

### Signal Handling

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
defer signal.Stop(sigCh)
```

**Buffer size 1**: Ensures we don't miss a signal if we're not immediately ready to receive.

**defer Stop**: Prevents signal channel leak. If we don't stop, the signal package holds a reference to our channel forever.

**Signals**: 
- `SIGINT` (Ctrl-C): Interactive stop
- `SIGTERM`: Graceful shutdown (systemd, containers, etc.)

**Not handled**: `SIGKILL` (can't be caught), `SIGHUP` (could add for reload in future).

---

### Connection Handler Goroutines

Each connection spawns a goroutine:

```go
s.sc.NewClient()
go s.handleConnection(ctx, conn, s.sc)
```

**Inside handleConnection**:
```go
defer s.sc.ClientDone()
defer conn.Close()
```

**Why this order**: 
1. `ClientDone()` deferred first (executes last) - tells shutdown controller we're done
2. `conn.Close()` deferred second (executes first) - ensures connection is closed

**Goroutine lifetime**: Lives until client disconnects or sends goodbye.

**Leak prevention**: Deferred cleanup ensures we always call `ClientDone()`, even if panic occurs.

---

## Gotchas & Lessons Learned

### json.Decoder Buffering Issue

**Problem**: Creating multiple decoders on the same `io.Reader` causes each to buffer independently, losing data.

**Example**:
```go
// DON'T DO THIS:
json.NewDecoder(conn).Decode(&msg1)
json.NewDecoder(conn).Decode(&msg2)  // Might miss data!
```

**Solution**: Use `BufferedConn` wrapper, or create one decoder and reuse it.

---

### Reflect.Type as Map Key

**Works fine**: `reflect.Type` is comparable, so it can be a map key.

**Used in**: `typeToMessageType` for reverse lookup (struct type → message type string).

```go
typeToMessageType := make(map[reflect.Type]string)
for msgType, payloadPtr := range payloadTypes {
    typeToMessageType[reflect.TypeOf(payloadPtr)] = msgType
}
```

**Note**: We store pointer types in the map (`(*Config)(nil)`), so lookups must also use pointer types.

---

### Pointer vs Value in Reflection

**Problem**: User might pass value or pointer to `Write()`.

**Solution**: Normalize to pointer before lookup:

```go
payloadType := reflect.TypeOf(payload)
if payloadType.Kind() != reflect.Ptr {
    payloadType = reflect.PointerTo(payloadType)
}
msgTypeStr := typeToMessageType[payloadType]
```

This allows both patterns:
```go
message.Write(w, &Config{...})  // Pointer
message.Write(w, Config{...})   // Value
```

---

### Context Timeout vs Cancellation

**Pattern**: Create child context with timeout for provider operations:

```go
// TODO: implement when we add resource CRUD
opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
resource.Read(opCtx, req, &resp)
```

**Why**: Provider operations might hang (network issue, API timeout, etc.). Timeout ensures we don't leak goroutines or block forever.

**Note**: Not implemented yet, but will be needed for resource operations.

---

### Stdin is One-Shot

**Gotcha**: The bootstrap client sends config on stdin, then the server must not read from stdin again.

**Why**: After reading the config message, stdin might be closed or redirected. The server shouldn't try to read more.

**Current**: We only read once in `configureProvider()`, then never touch stdin again.

---

### Stdout for Announcements Only

**Pattern**: Server announces startup on stdout (sending `listening` message), then stdout is not used again.

**Why**: Bootstrap client (patron) needs to learn the socket path. After that, all communication happens over the socket.

**Important**: Regular clients (non-bootstrap) connect directly to the socket - they don't use stdin/stdout at all.

---

## Code Organization Decisions

### Why Separate message/ Package?

**Rationale**: Message serialization/deserialization is self-contained and could be tested independently.

**Exports**:
- `Message` struct (envelope)
- Type constants (`TypeConfig`, `TypeHello`, etc.)
- Payload structs (`Config`, `Hello`, etc.)
- `Read()` and `Write()` functions
- `UnpackPayload()` for manual dispatch

---

### Why Separate shutdown/ Package?

**Rationale**: Shutdown controller is complex enough to deserve its own package, and might be reusable in other contexts.

**Exports**:
- `Controller` struct
- `New()` constructor with options
- Option constructors (`WithIdleTimeout()`, etc.)

---

### Why Not Separate auth/ Package?

**Decision**: Auth logic stays in `auth.go` in main package.

**Rationale**: Auth is tightly coupled to connection handling. Separating it would require exposing too many internals.

---

## Testing Gaps (To Address)

### Unit Tests Needed

1. **Message pack/unpack**:
   - All message types
   - Invalid protocol versions
   - Unknown message types
   - Pointer vs value inputs
   - Malformed JSON

2. **Auth challenge-response**:
   - Valid HMAC
   - Invalid HMAC
   - Missing secret
   - Replay attacks (if we add nonce tracking)

3. **Shutdown controller**:
   - Idle timeout fires
   - Grace period fires
   - Client activity resets idle
   - Zero clients after grace calls stopFunc
   - Multiple rapid connect/disconnect

4. **BufferedConn**:
   - Preserves unread data
   - Multiple reads work correctly
   - Write pass-through works
   - Close works

---

### Integration Tests Needed

1. **Server lifecycle**:
   - Start with config on stdin
   - Announce on stdout
   - Accept connection
   - Handle message
   - Graceful shutdown

2. **Multiple clients**:
   - Connect concurrently
   - No crosstalk between connections
   - Disconnect doesn't affect others

3. **Shutdown scenarios**:
   - Idle timeout with no clients
   - Shutdown with active clients (waits for them)
   - SIGTERM handling
   - Context cancellation

---

### E2E Tests Needed (Future)

1. **Bootstrap flow**:
   - Race to create lock file
   - Winner starts server
   - Losers read socket path
   - All connect successfully

2. **Auth flow**:
   - Server requires auth
   - Client without secret rejected
   - Client with wrong secret rejected
   - Client with correct secret accepted

3. **Operations**:
   - Data source read
   - Resource CRUD (when implemented)
   - Error handling
   - Concurrent operations

---

## Performance Considerations

### Connection Pooling

**Current**: Each Ansible module opens a new socket connection for each operation.

**Potential optimization**: Keep connections open between operations.

**Trade-off**: More complex client code, but faster operations.

**Decision**: Start simple, optimize if needed.

---

### Message Serialization

**Current**: Marshal/unmarshal on every message.

**Potential optimization**: Message pooling, pre-allocated buffers.

**When to optimize**: Only if profiling shows this is a bottleneck.

---

### Provider Operation Caching

**Consideration**: Could we cache data source reads or resource state?

**Complexity**: Cache invalidation is hard. Would need to track dependencies.

**Decision**: Don't cache. Let the provider handle it.

---

## Security Considerations

### Socket Permissions

**Implementation**: `umask(0077)` before creating socket.

**Result**: Socket file is mode 0700 (owner-only).

**Why**: Prevents other users on the same system from connecting.

---

### HMAC Prevents Replay

**Current**: Server generates random nonce for each connection.

**Future**: Could add nonce tracking to prevent replay attacks within server lifetime.

**Trade-off**: More complexity, probably not needed for local Unix socket.

---

### Secret Handling

**Current**: Secret passed via stdin in config message.

**Storage**: Held in memory (`s.secret []byte`), never written to disk.

**Transmission**: Over Unix socket (stays on local machine).

**Consideration**: Could use environment variable instead of stdin. Decision: stdin is fine for now.

---

## Future Enhancements

### Metrics

Useful metrics to track:
- Active connection count
- Messages per second by type
- Provider operation latency (Read/Create/Update/Delete)
- Auth success/failure rate
- Shutdown trigger reason (idle/signal/context)

**Implementation**: Could add Prometheus exporter or structured logging.

---

### Connection Draining

**Current**: After grace period, stop accepting but serve existing clients indefinitely.

**Enhancement**: Send "server shutting down" message to all connected clients, give them N seconds to finish, then force-close.

**Trade-off**: More complex, might not be needed. Clients can always reconnect to a new instance.

---

### Configuration File

**Current**: Allowlists hard-coded, timeouts are constants.

**Enhancement**: Load from config file (YAML/TOML).

```yaml
allowed_resources:
  - apstra_datacenter_routing_zone
  - apstra_datacenter_virtual_network
idle_timeout: 30s
grace_timeout: 5s
```

---

### Provider Adapter Pattern

**Current**: Tightly coupled to `terraform-plugin-framework`.

**Enhancement**: Define adapter interface, support multiple providers.

```go
type ProviderAdapter interface {
    Configure(ctx context.Context, config json.RawMessage) error
    GetResource(name string) ResourceAdapter
    GetDataSource(name string) DataSourceAdapter
}
```

This would make the server reusable for any Terraform provider.

---

## References & Similar Projects

- **Terraform Plugin Protocol**: gRPC-based RPC for Terraform ↔ provider communication
- **Terraform Cloud Agents**: Long-running processes that execute Terraform runs
- **Ansible Connection Plugins**: Similar "daemon per workflow" pattern
- **Language Servers (LSP)**: JSON-RPC over stdio, similar message patterns
- **Docker API**: Unix socket for local communication, REST over socket for remote
- **systemd**: Uses D-Bus for IPC, but similar socket activation patterns

---

## Glossary

- **Bootstrap client**: The first client (also called "patron") who starts the server via stdin/stdout
- **Patron**: The bootstrap client who wins the race to create the lock file
- **Discovery file**: Lock file containing the socket path, used by clients to find the server
- **Envelope**: Outer message structure with `type` and `protocol_version`
- **Payload**: Inner message data, specific to each message type
- **Stage 1 decode**: Parse envelope only
- **Stage 2 decode**: Parse payload into concrete type
- **Idle timeout**: Period of inactivity (30s) before shutdown begins
- **Grace period**: Additional time (5s) after idle timeout for stragglers
- **Straggler**: Client who reads discovery file just before/during deletion
