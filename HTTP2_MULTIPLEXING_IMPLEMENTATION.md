# HTTP/2 Multiplexing Implementation Plan

## Problem Statement

The current tlsfingerprint.com server implementation has a fundamental architectural issue: **it does not support HTTP/2 multiplexing**. Each HTTP/2 connection is treated as single-request-and-close, which causes "unexpected EOF" errors when clients attempt operations like:

- Following redirect chains (`/redirect/3` → `/redirect/2` → `/redirect/1` → `/get`)
- Making multiple requests on a single connection
- Using HTTP/2's primary efficiency feature (stream multiplexing)

### Current Symptoms

1. CycleTLS tests fail with "unexpected EOF" when following redirects
2. Non-redirect endpoints (`/status/200`, `/status/404`) work because they complete in one request
3. Redirects fail mid-chain (e.g., `/redirect/3` fails at `/redirect/2`)

### Root Cause Analysis

**Location:** `pkg/server/connection_handler.go`

#### Issue 1: HTTP/1 Immediate Connection Close (lines 351-360)
```go
_, err := conn.Write([]byte(res1))
// ...
time.Sleep(50 * time.Millisecond)  // Current bandaid fix
err = conn.Close()  // <-- Always closes after single request
```

#### Issue 2: HTTP/2 Single-Request-Per-Connection (lines 367-597)
The `handleHTTP2` function:
1. Reads frames until it sees `EndStream`
2. Processes ONE request
3. Sends response
4. Sends `GOAWAY` frame
5. Closes connection

This defeats HTTP/2's core purpose - multiplexing multiple streams over a single connection.

#### Issue 3: HTTP/2 Eager GOAWAY (lines 586-596)
```go
if statusCode < 300 || statusCode >= 400 {
    fr.WriteGoAway(headerFrame.Stream, http2.ErrCodeNo, []byte{})
    time.Sleep(time.Millisecond * 500)
    conn.Close()
} else {
    // For redirects, give more time - but still closes!
    time.Sleep(time.Millisecond * 100)
    conn.Close()
}
```

Even the "fix" for redirects still closes the connection immediately after.

---

## Proposed Solution

### Architecture Overview

Transform from **single-request-per-connection** to **proper HTTP/2 multiplexing**:

```
CURRENT (broken):
┌─────────────────────────────────────────┐
│ Connection                              │
│  └─ Stream 1 (request) → response → CLOSE
└─────────────────────────────────────────┘

PROPOSED (correct):
┌─────────────────────────────────────────┐
│ Connection (persistent)                 │
│  ├─ Stream 1 (request) → response       │
│  ├─ Stream 3 (request) → response       │
│  ├─ Stream 5 (request) → response       │
│  └─ ... (until idle timeout or GOAWAY)  │
└─────────────────────────────────────────┘
```

### Implementation Phases

---

## Phase 1: Connection State Management

### 1.1 Create Connection Manager

Create `pkg/server/connection_state.go`:

```go
package server

import (
    "net"
    "sync"
    "time"

    "golang.org/x/net/http2"
)

type HTTP2Connection struct {
    conn           net.Conn
    framer         *http2.Framer
    tlsFingerprint *types.TLSDetails

    // Stream management
    streams        map[uint32]*HTTP2Stream
    streamsMu      sync.RWMutex
    lastStreamID   uint32

    // Connection lifecycle
    maxStreams     uint32
    idleTimeout    time.Duration
    lastActivity   time.Time
    closing        bool
    closeMu        sync.Mutex

    // Server reference
    srv            *Server
}

type HTTP2Stream struct {
    streamID       uint32
    state          StreamState
    headerFrame    types.ParsedFrame
    dataFrames     []types.ParsedFrame
    response       chan []byte
}

type StreamState int

const (
    StreamOpen StreamState = iota
    StreamHalfClosedRemote
    StreamHalfClosedLocal
    StreamClosed
)

func NewHTTP2Connection(conn net.Conn, framer *http2.Framer, tlsDetails *types.TLSDetails, srv *Server) *HTTP2Connection {
    return &HTTP2Connection{
        conn:           conn,
        framer:         framer,
        tlsFingerprint: tlsDetails,
        streams:        make(map[uint32]*HTTP2Stream),
        maxStreams:     100,  // Match SETTINGS_MAX_CONCURRENT_STREAMS
        idleTimeout:    30 * time.Second,
        lastActivity:   time.Now(),
        srv:            srv,
    }
}
```

### 1.2 Stream Lifecycle Methods

```go
func (c *HTTP2Connection) GetOrCreateStream(streamID uint32) *HTTP2Stream {
    c.streamsMu.Lock()
    defer c.streamsMu.Unlock()

    if stream, exists := c.streams[streamID]; exists {
        return stream
    }

    stream := &HTTP2Stream{
        streamID: streamID,
        state:    StreamOpen,
        response: make(chan []byte, 1),
    }
    c.streams[streamID] = stream

    if streamID > c.lastStreamID {
        c.lastStreamID = streamID
    }

    return stream
}

func (c *HTTP2Connection) CloseStream(streamID uint32) {
    c.streamsMu.Lock()
    defer c.streamsMu.Unlock()

    if stream, exists := c.streams[streamID]; exists {
        stream.state = StreamClosed
        close(stream.response)
        delete(c.streams, streamID)
    }
}

func (c *HTTP2Connection) ActiveStreamCount() int {
    c.streamsMu.RLock()
    defer c.streamsMu.RUnlock()
    return len(c.streams)
}
```

---

## Phase 2: Frame Processing Loop

### 2.1 Replace Single-Request Handler

Replace the current `handleHTTP2` function with a continuous frame processing loop:

```go
func (srv *Server) handleHTTP2(conn net.Conn, tlsFingerprint *types.TLSDetails) {
    fr := http2.NewFramer(conn, conn)
    h2conn := NewHTTP2Connection(conn, fr, tlsFingerprint, srv)

    // Send initial SETTINGS
    err := fr.WriteSettings(
        http2.Setting{ID: http2.SettingInitialWindowSize, Val: 1048576},
        http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 100},
        http2.Setting{ID: http2.SettingMaxHeaderListSize, Val: 65536},
    )
    if err != nil {
        log.Println("Failed to write settings:", err)
        return
    }

    // Start idle timeout goroutine
    go h2conn.idleTimeoutLoop()

    // Main frame processing loop
    h2conn.processFrames()
}

func (c *HTTP2Connection) processFrames() {
    defer c.gracefulShutdown()

    for {
        frame, err := c.framer.ReadFrame()
        if err != nil {
            if err == io.EOF || isConnectionClosed(err) {
                return
            }
            log.Println("Error reading frame:", err)
            return
        }

        c.lastActivity = time.Now()

        switch f := frame.(type) {
        case *http2.SettingsFrame:
            if !f.IsAck() {
                c.framer.WriteSettingsAck()
            }

        case *http2.HeadersFrame:
            go c.handleRequest(f)

        case *http2.DataFrame:
            c.handleData(f)

        case *http2.WindowUpdateFrame:
            // Handle flow control (can be expanded later)

        case *http2.PingFrame:
            if !f.IsAck() {
                c.framer.WritePing(true, f.Data)
            }

        case *http2.GoAwayFrame:
            // Client is closing, respect it
            return

        case *http2.RSTStreamFrame:
            c.CloseStream(f.StreamID)
        }
    }
}
```

### 2.2 Per-Stream Request Handler

```go
func (c *HTTP2Connection) handleRequest(headerFrame *http2.HeadersFrame) {
    streamID := headerFrame.StreamID
    stream := c.GetOrCreateStream(streamID)

    // Decode headers
    d := hpack.NewDecoder(4096, func(hf hpack.HeaderField) {})
    d.SetEmitEnabled(true)
    headers, err := d.DecodeFull(headerFrame.HeaderBlockFragment())
    if err != nil {
        log.Println("Error decoding headers:", err)
        c.sendRSTStream(streamID, http2.ErrCodeProtocol)
        return
    }

    // Parse request details
    var path, method, userAgent string
    var parsedHeaders []string
    for _, h := range headers {
        switch h.Name {
        case ":method":
            method = h.Value
        case ":path":
            path = h.Value
        case "user-agent":
            userAgent = h.Value
        }
        parsedHeaders = append(parsedHeaders, fmt.Sprintf("%s: %s", h.Name, h.Value))
    }

    // Wait for body if not EndStream
    var bodyData []byte
    if !headerFrame.StreamEnded() {
        bodyData = c.waitForStreamBody(streamID)
    }

    // Build response object (existing logic)
    resp := c.buildResponse(path, method, userAgent, parsedHeaders, bodyData)

    // Route and send response
    c.sendResponse(streamID, resp, path, method)
}

func (c *HTTP2Connection) waitForStreamBody(streamID uint32) []byte {
    stream := c.streams[streamID]
    if stream == nil {
        return nil
    }

    // Wait for data frames with timeout
    timeout := time.After(5 * time.Second)
    var body []byte

    for {
        select {
        case data, ok := <-stream.response:
            if !ok {
                return body
            }
            body = append(body, data...)
        case <-timeout:
            return body
        }
    }
}
```

---

## Phase 3: Response Writing

### 3.1 Stream-Aware Response

```go
func (c *HTTP2Connection) sendResponse(streamID uint32, resp types.Response, path, method string) {
    res, ctype := Router(path, resp, c.srv)

    // Handle redirects
    statusCode, extraHeaders := c.parseContentType(ctype, path)
    if res == nil {
        res = []byte{}
    }

    // Build HEADERS frame
    hbuf := bytes.NewBuffer([]byte{})
    encoder := hpack.NewEncoder(hbuf)
    encoder.WriteField(hpack.HeaderField{Name: ":status", Value: strconv.Itoa(statusCode)})
    encoder.WriteField(hpack.HeaderField{Name: "server", Value: "TrackMe.peet.ws"})
    encoder.WriteField(hpack.HeaderField{Name: "content-length", Value: strconv.Itoa(len(res))})
    encoder.WriteField(hpack.HeaderField{Name: "content-type", Value: ctype})

    for _, h := range extraHeaders {
        encoder.WriteField(h)
    }

    // Write HEADERS
    err := c.framer.WriteHeaders(http2.HeadersFrameParam{
        StreamID:      streamID,
        BlockFragment: hbuf.Bytes(),
        EndHeaders:    true,
        EndStream:     len(res) == 0,  // EndStream if no body
    })
    if err != nil {
        log.Println("Error writing headers:", err)
        return
    }

    // Write DATA frames if body exists
    if len(res) > 0 {
        chunks := utils.SplitBytesIntoChunks(res, 16384)  // 16KB chunks
        for i, chunk := range chunks {
            endStream := i == len(chunks)-1
            c.framer.WriteData(streamID, endStream, chunk)
        }
    }

    // Close this stream, but NOT the connection
    c.CloseStream(streamID)

    // DO NOT send GOAWAY or close connection here!
    // The connection stays open for more streams
}
```

---

## Phase 4: Connection Lifecycle

### 4.1 Idle Timeout

```go
func (c *HTTP2Connection) idleTimeoutLoop() {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        c.closeMu.Lock()
        if c.closing {
            c.closeMu.Unlock()
            return
        }

        // Check if idle
        if time.Since(c.lastActivity) > c.idleTimeout && c.ActiveStreamCount() == 0 {
            c.closing = true
            c.closeMu.Unlock()
            c.initiateGracefulShutdown()
            return
        }
        c.closeMu.Unlock()
    }
}
```

### 4.2 Graceful Shutdown

```go
func (c *HTTP2Connection) initiateGracefulShutdown() {
    // Send GOAWAY with last processed stream ID
    c.framer.WriteGoAway(c.lastStreamID, http2.ErrCodeNo, []byte("idle timeout"))

    // Wait for in-flight streams to complete (with timeout)
    deadline := time.Now().Add(5 * time.Second)
    for c.ActiveStreamCount() > 0 && time.Now().Before(deadline) {
        time.Sleep(100 * time.Millisecond)
    }

    // Close connection
    c.conn.Close()
}

func (c *HTTP2Connection) gracefulShutdown() {
    c.closeMu.Lock()
    if !c.closing {
        c.closing = true
        c.closeMu.Unlock()
        c.initiateGracefulShutdown()
        return
    }
    c.closeMu.Unlock()
    c.conn.Close()
}
```

---

## Phase 5: HTTP/1 Keep-Alive (Optional Enhancement)

### 5.1 HTTP/1.1 Keep-Alive Support

For HTTP/1, implement keep-alive to support redirect chains:

```go
func (srv *Server) respondToHTTP1(conn net.Conn, resp types.Response) {
    // ... existing header building ...

    // Add keep-alive headers
    res1 += "Connection: keep-alive\r\n"
    res1 += "Keep-Alive: timeout=30, max=100\r\n"

    // Write response
    _, err := conn.Write([]byte(res1))
    if err != nil {
        log.Println("Error writing HTTP/1 data", err)
        return
    }

    // DON'T close - loop for more requests
    srv.http1KeepAliveLoop(conn)
}

func (srv *Server) http1KeepAliveLoop(conn net.Conn) {
    maxRequests := 100
    idleTimeout := 30 * time.Second

    for i := 0; i < maxRequests; i++ {
        conn.SetReadDeadline(time.Now().Add(idleTimeout))

        request := make([]byte, 1024)
        n, err := conn.Read(request)
        if err != nil {
            // Timeout or client closed - exit gracefully
            conn.Close()
            return
        }

        // Parse and handle the next request
        details := parseHTTP1(request[:n])
        details.IP = conn.RemoteAddr().String()
        // ... handle request ...
    }

    conn.Close()
}
```

---

## Testing Plan

### Unit Tests

Create `pkg/server/connection_handler_test.go`:

```go
func TestHTTP2Multiplexing(t *testing.T) {
    // Test multiple streams on single connection
}

func TestHTTP2RedirectChain(t *testing.T) {
    // Test /redirect/3 works without "unexpected EOF"
}

func TestHTTP2IdleTimeout(t *testing.T) {
    // Test connection closes after idle period
}

func TestHTTP2GracefulShutdown(t *testing.T) {
    // Test GOAWAY + in-flight stream completion
}
```

### Integration Tests

Use `curl` with HTTP/2:

```bash
# Test multiple requests on same connection
curl -v --http2 https://localhost:8443/get https://localhost:8443/headers

# Test redirect chain (most important!)
curl -vL --http2 https://localhost:8443/redirect/3

# Test with connection reuse explicitly
curl -v --http2 --keepalive-time 30 \
  https://localhost:8443/redirect/3 \
  https://localhost:8443/get
```

### CycleTLS Tests

After implementation, the following tests should pass:

```bash
cd /Users/dannydasilva/Documents/personal/CycleTLS/cycletls
npx vitest run tests/tlsfingerprint/redirect.test.ts
```

---

## Migration Strategy

### Step 1: Add Connection State (Low Risk)
- Add new files without modifying existing code
- Test in isolation

### Step 2: Replace HTTP/2 Handler (Medium Risk)
- Keep old handler as `handleHTTP2Legacy`
- Add feature flag to switch between implementations
- Test extensively

### Step 3: Remove Legacy Code (After Verification)
- Remove feature flag
- Delete legacy handler
- Update documentation

### Feature Flag Example

```go
var UseMultiplexedHTTP2 = os.Getenv("USE_MULTIPLEXED_HTTP2") == "true"

func (srv *Server) HandleTLSConnection(conn net.Conn) bool {
    // ... TLS fingerprinting ...

    if string(request) == HTTP2_PREAMBLE {
        if UseMultiplexedHTTP2 {
            srv.handleHTTP2Multiplexed(conn, &tlsDetails)
        } else {
            srv.handleHTTP2(conn, &tlsDetails)  // Legacy
        }
    }
    // ...
}
```

---

## Summary of Changes

| File | Changes |
|------|---------|
| `pkg/server/connection_state.go` | NEW - Connection and stream state management |
| `pkg/server/connection_handler.go` | MODIFY - Replace single-request handler with frame loop |
| `pkg/server/connection_handler_test.go` | NEW - Unit tests for multiplexing |

### Current Bandaid Fixes to Remove

Once multiplexing is implemented, remove these temporary fixes:

1. **Line 358**: `time.Sleep(50 * time.Millisecond)` before HTTP/1 close
2. **Lines 586-596**: Conditional GOAWAY logic for redirects

These were necessary workarounds for the single-request architecture but will be obsolete with proper multiplexing.

---

## References

- [RFC 9113: HTTP/2](https://datatracker.ietf.org/doc/html/rfc9113)
- [golang.org/x/net/http2](https://pkg.go.dev/golang.org/x/net/http2)
- [HTTP/2 Stream States](https://datatracker.ietf.org/doc/html/rfc9113#section-5.1)
- [GOAWAY Frame](https://datatracker.ietf.org/doc/html/rfc9113#section-6.8)
