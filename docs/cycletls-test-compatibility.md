# CycleTLS Test Compatibility Matrix for tlsfingerprint.com

Generated: 2026-01-20

## Executive Summary

This document maps all CycleTLS tests (TypeScript/JS and Go) to the endpoints available on tlsfingerprint.com (https://34.170.213.141/). It identifies which tests can run against the server as-is, which require new endpoints, and which are client-only tests that don't need server support.

### Quick Stats (Updated 2026-01-20)
| Category | Count |
|----------|-------|
| **TypeScript/JS Tests** | 27 |
| **Go Integration Tests** | 25 |
| **Tests Compatible Now** | ~47 |
| **Tests Needing Special Servers** | ~5 |
| **Client-Only Tests (no server needed)** | ~5 |

**Recent Fixes:** Redirects, cookies, streaming, WebSocket, response headers all now work.

---

## tlsfingerprint.com Current Capabilities

### Available Endpoints (50+)
Based on analysis of `pkg/server/routes.go` and `pkg/server/routes_httpbin.go`:

| Category | Endpoints | Status |
|----------|-----------|--------|
| **TLS Fingerprinting** | `/api/all`, `/api/tls`, `/api/clean`, `/api/raw` | ✅ Available |
| **HTTP Methods** | `/get`, `/post`, `/put`, `/patch`, `/delete`, `/anything` | ✅ Available |
| **Request Inspection** | `/headers`, `/ip`, `/user-agent` | ✅ Available |
| **Compression** | `/gzip`, `/deflate`, `/brotli` | ✅ Available |
| **Cookies** | `/cookies`, `/cookies/set`, `/cookies/delete` | ✅ Available (Set-Cookie headers fixed) |
| **Images** | `/image/jpeg`, `/image/png`, `/image/svg`, `/image/gif`, `/image/webp` | ✅ Available |
| **Binary Data** | `/bytes/{n}` | ✅ Available (POST echo supported) |
| **Delays** | `/delay/{seconds}` | ✅ Available |
| **Redirects** | `/redirect/{n}`, `/redirect-to` | ✅ Available (302 redirects fixed) |
| **Status Codes** | `/status/{code}` | ✅ Available |
| **SSE** | `/sse`, `/sse/{n}` | ✅ Available |
| **Streaming** | `/stream/{n}` | ✅ Available |
| **WebSocket** | `/ws` | ✅ Available (HTTP/3 only) |
| **Response Formats** | `/html`, `/xml`, `/json`, `/robots.txt` | ✅ Available |
| **Base64** | `/base64/{value}` | ✅ Available |

### Response Headers
All endpoints now include:
- `X-Request-Id`: Unique request identifier (16-char alphanumeric)
- `X-Response-Time`: Response time in milliseconds

### Remaining Limitations
1. **WebSocket HTTP/1**: `/ws` only works on HTTP/3 due to raw TCP architecture
2. **HTTP/1.1 POST body**: Binary echo for `/bytes/{n}` requires HTTP/2+ (HTTP/1.1 body parsing limited)

---

## TypeScript/JS Test Compatibility (27 tests)

### ✅ Compatible Tests (Can run against tlsfingerprint.com)

| Test File | Required Endpoints | Compatibility |
|-----------|-------------------|---------------|
| `simple.test.js` | `/cookies` | ✅ Works |
| `simple.test.ts` | `/cookies` | ✅ Works |
| `simple-connection.test.js` | `/get` | ✅ Works |
| `integration.test.ts` | `/user-agent`, `/post`, `/put`, `/patch`, `/delete`, `/headers`, `/cookies`, `/ip`, `/html` | ✅ Works |
| `encoding.test.ts` | `/brotli`, `/deflate`, `/gzip` | ✅ Works |
| `forceHTTP1.test.ts` | `/api/all` (http_version) | ✅ Works |
| `frameHeader.test.ts` | `/api/all` (sent_frames) | ✅ Works |
| `http2-fingerprint.test.js` | `/api/all` (akamai_fingerprint) | ✅ Works |
| `ja4-fingerprint.test.js` | `/api/all` (ja4_r) | ✅ Works |
| `images.test.ts` | `/image/jpeg`, `/image/png`, `/image/svg`, `/image/webp` | ✅ Works |
| `multipartFormData.test.ts` | `/post` | ✅ Works |
| `urlencoded.test.ts` | `/post` | ✅ Works |
| `multipleRequests.test.js` | `/user-agent`, `/post`, `/cookies`, `/get`, `/headers` | ✅ Works |
| `timeout.test.ts` | `/delay/4`, `/delay/1` | ✅ Works |
| `response-methods.test.js` | `/json`, `/html`, `/robots.txt`, `/bytes/1024` | ✅ Works |
| `binary-data-handling.test.js` | `/post`, `/image/jpeg`, `/image/png` | ⚠️ Needs `/bytes/{n}` to echo back |

### ✅ Now Compatible (Fixed in 2026-01-20 update)

| Test File | Required Feature | Status |
|-----------|-----------------|--------|
| `cookie.test.ts` | Cookie echo | ✅ Works |
| `cookiejar.test.js` | `Set-Cookie` headers from `/cookies/set` | ✅ Fixed - headers now sent |
| `disableRedirect.test.ts` | 301/302 redirects | ✅ Fixed - returns actual 302 |
| `flow-control.test.ts` | `/bytes/{n}`, `/status/404`, `/status/500`, `/redirect/2` | ✅ Fixed - all work now |
| `streaming.test.js` | `/stream/3`, `/stream/2`, `/stream/1` | ✅ Fixed - endpoint added |

### ❌ Incompatible (Client-only or external)

| Test File | Required Feature | Status |
|-----------|-----------------|--------|
| `connectionReuse.test.ts` | Custom HTTPS server with handshake counting | 🔧 **Client-only** (local server) |
| `insecureSkipVerify.test.ts` | `expired.badssl.com`, `self-signed.badssl.com` | 🌐 **External sites** |
| `sni-override.test.ts` | Custom SNI handling | ✅ Use `/api/sni` to verify SNI |
| `sse.test.ts` | `/events` endpoint | ✅ Use `/sse/{n}` instead |
| `websocket.test.ts` | WebSocket server | ✅ Use `/ws` (HTTP/3 only) |
| `read-timeout.test.ts` | Server that stalls mid-body | 🔧 **Client-only** (needs special server) |
| `multipleImports.test.ts` | Basic connectivity | ✅ Works (any endpoint) |

---

## Go Integration Test Compatibility (25 tests)

### ✅ Compatible Tests

| Test File | Required Endpoints | Compatibility |
|-----------|-------------------|---------------|
| `main_ja3_test.go` | `/api/all` (ja3) | ✅ Works |
| `latest_fingerprint_test.go` | `/api/all` | ✅ Works |
| `tls_13_test.go` | `/api/all` | ✅ Works |
| `tls13_auto_retry_test.go` | `/api/all` | ✅ Works |
| `ForceHTTP1_test.go` | `/api/all` | ✅ Works |
| `decoding_test.go` | `/brotli`, `/deflate`, `/gzip` | ✅ Works |
| `binary_data_test.go` | `/post`, `/bytes/{n}` | ✅ Works |
| `images_test.go` | `/image/*` | ✅ Works |
| `multipart_formdata_test.go` | `/post` | ✅ Works |
| `UrlEncodedFormData_test.go` | `/post` | ✅ Works |
| `multiple_requests_test.go` | Multiple endpoints | ✅ Works |
| `custom_timeout_test.go` | `/delay/{n}` | ✅ Works |
| `panic_regression_test.go` | Any endpoint | ✅ Works |

### ⚠️ Partially Compatible

| Test File | Required Feature | Fix Needed |
|-----------|-----------------|------------|
| `cookie_test.go` | Cookie handling | ✅ Works |
| `cookiejar_test.go` | `Set-Cookie` headers | **Add `Set-Cookie` header** |
| `disable_redirect_test.go` | 301/302 redirects | **Return actual redirects** |
| `connection_reuse_test.go` | Connection tracking | 🔧 **Client-only** |
| `issue_407_connection_reuse_test.go` | Connection tracking | 🔧 **Client-only** |
| `sse_test.go` | `/events` endpoint | ✅ Use `/sse/{n}` |
| `sse_only_test.go` | SSE stream | ✅ Use `/sse/{n}` |

### ❌ Incompatible (Need new features)

| Test File | Required Feature | Status |
|-----------|-----------------|--------|
| `http3_test.go` | HTTP/3 (QUIC) | ✅ **Already supported** (UDP 443) |
| `quic_test.go` | QUIC protocol | ✅ **Already supported** |
| `InsecureSkipVerify_test.go` | Bad SSL certs | 🌐 **External sites** |
| `proxy_test.go` | Proxy testing | 🔧 **Client-only** (local proxy) |
| `websocket_test.go` | WebSocket server | ❌ **Need WebSocket endpoint** |

---

## Required New Endpoints

### High Priority (Many tests need these)

#### 1. `/stream/{n}` - Streaming JSON Lines
**Tests that need it:** `streaming.test.js`, `flow-control.test.ts`

```go
// Implementation suggestion
func handleStream(w http.ResponseWriter, r *http.Request, n int) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Transfer-Encoding", "chunked")

    for i := 0; i < n; i++ {
        json.NewEncoder(w).Encode(map[string]interface{}{
            "id": i,
            "url": r.URL.String(),
            "ja3_hash": getTLSFingerprint(r),
        })
        w.(http.Flusher).Flush()
    }
}
```

#### 2. Fix `/redirect/{n}` - Return Actual 302 Redirects
**Tests that need it:** `disableRedirect.test.ts`, `disable_redirect_test.go`, `flow-control.test.ts`

```go
// Current: Returns JSON
// Needed: Return HTTP 302 with Location header
func handleRedirect(w http.ResponseWriter, r *http.Request, n int) {
    if n <= 0 {
        handleGet(w, r) // Final destination
        return
    }
    w.Header().Set("Location", fmt.Sprintf("/redirect/%d", n-1))
    w.WriteHeader(http.StatusFound) // 302
}
```

#### 3. Fix `/status/{code}` - Return Actual Status Codes
**Tests that need it:** `flow-control.test.ts`

```go
// Current: Returns JSON with status_code field
// Needed: Return actual HTTP status
func handleStatus(w http.ResponseWriter, r *http.Request, code int) {
    w.WriteHeader(code)
    // Optionally include body for certain codes
}
```

#### 4. Fix `/cookies/set` - Send Set-Cookie Headers
**Tests that need it:** `cookiejar.test.js`, `cookiejar_test.go`

```go
// Add Set-Cookie headers before response
func handleCookiesSet(w http.ResponseWriter, r *http.Request) {
    for key, values := range r.URL.Query() {
        for _, value := range values {
            http.SetCookie(w, &http.Cookie{
                Name:  key,
                Value: value,
                Path:  "/",
            })
        }
    }
    // Then return JSON confirmation
}
```

### Medium Priority

#### 5. `/ws` - WebSocket Echo Server
**Tests that need it:** `websocket.test.ts`, `websocket_test.go`

```go
// WebSocket upgrade and echo
var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil { return }
    defer conn.Close()

    for {
        messageType, p, err := conn.ReadMessage()
        if err != nil { break }
        conn.WriteMessage(messageType, p)
    }
}
```

### Low Priority (Nice to have)

#### 6. Binary Data Echo for `/post`
**Tests that need it:** `binary-data-handling.test.js`, `binary_data_test.go`

The current `/post` endpoint should preserve binary data exactly. Verify that:
- `application/octet-stream` content type is handled
- Response includes raw bytes in base64 or hex
- No UTF-8 encoding corruption

---

## Test Migration Guide

### Running Tests Against tlsfingerprint.com

1. **Update base URLs** in test files:
   ```typescript
   // Before
   const baseUrl = "https://httpbin.org";
   const fpUrl = "https://tls.peet.ws/api/all";

   // After
   const baseUrl = "https://34.170.213.141";
   const fpUrl = "https://34.170.213.141/api/all";
   ```

2. **SSE tests** - change endpoint:
   ```typescript
   // Before
   const url = "http://localhost:{port}/events";

   // After
   const url = "https://34.170.213.141/sse/5";
   ```

3. **Fingerprint validation** - use `/api/all` response:
   ```typescript
   const response = await cycleTLS.get("https://34.170.213.141/api/all");
   const data = JSON.parse(response.body);

   expect(data.tls.ja3).toBeDefined();
   expect(data.tls.ja4).toBeDefined();
   expect(data.http2.akamai_fingerprint).toBeDefined();
   ```

### Tests That Cannot Migrate (Local Server Required)

These tests require custom local servers and cannot use tlsfingerprint.com:

| Test | Reason |
|------|--------|
| `connectionReuse.test.ts` | Needs TLS handshake counting |
| `sni-override.test.ts` | Needs SNI inspection |
| `read-timeout.test.ts` | Needs server that stalls mid-body |
| `proxy_test.go` | Needs local proxy server |

---

## Implementation Checklist

### Phase 1: Quick Fixes (Behavior Changes)

- [x] **`/redirect/{n}`**: Return 302 with Location header instead of JSON ✅
- [x] **`/redirect-to`**: Return 302 redirect to specified URL ✅
- [x] **`/status/{code}`**: Return actual HTTP status code (already worked via extractStatusCode)
- [x] **`/cookies/set`**: Add `Set-Cookie` headers to response ✅

### Phase 2: New Endpoints

- [x] **`/stream/{n}`**: Stream n JSON objects with flushing ✅
- [x] **`/ws`**: WebSocket echo server ✅ (HTTP/3 only)

### Phase 3: Validation Endpoints

- [x] **`/bytes/{n}` POST echo**: Return posted binary data exactly ✅
- [x] **Add response headers**: `X-Request-Id`, `X-Response-Time` ✅

### Phase 4: Documentation

- [x] Update `/docs` with CycleTLS testing guide (auto-loads from OpenAPI)
- [ ] Add examples for each endpoint
- [ ] Document TLS fingerprint fields in detail

---

## Appendix: Full Test Inventory

### TypeScript/JS Tests (27 total)

| # | Test File | Purpose | Server Needs |
|---|-----------|---------|--------------|
| 1 | `binary-data-handling.test.js` | Binary upload/download integrity | `/post`, `/bytes/{n}` |
| 2 | `connectionReuse.test.ts` | TLS connection reuse | Local server |
| 3 | `cookie.test.ts` | Cookie handling | `/cookies` |
| 4 | `cookiejar.test.js` | Cookie jar integration | `/cookies/set` with headers |
| 5 | `disableRedirect.test.ts` | Redirect control | Actual 302 redirects |
| 6 | `encoding.test.ts` | Compression handling | `/brotli`, `/deflate`, `/gzip` |
| 7 | `flow-control.test.ts` | Streaming backpressure | `/bytes/{n}`, `/stream/{n}` |
| 8 | `forceHTTP1.test.ts` | HTTP version control | `/api/all` |
| 9 | `frameHeader.test.ts` | HTTP/2 frame settings | `/api/all` |
| 10 | `http2-fingerprint.test.js` | HTTP/2 fingerprinting | `/api/all` |
| 11 | `images.test.ts` | Image download | `/image/*` |
| 12 | `insecureSkipVerify.test.ts` | SSL certificate handling | External bad SSL sites |
| 13 | `integration.test.ts` | Full HTTP client test | Multiple endpoints |
| 14 | `ja4-fingerprint.test.js` | JA4 fingerprinting | `/api/all` |
| 15 | `multipartFormData.test.ts` | Form data upload | `/post` |
| 16 | `multipleImports.test.ts` | Multiple instances | Any endpoint |
| 17 | `multipleRequests.test.js` | Concurrent requests | Multiple endpoints |
| 18 | `read-timeout.test.ts` | Read timeout handling | Custom stalling server |
| 19 | `response-methods.test.js` | Response parsing | `/json`, `/html`, `/bytes/{n}` |
| 20 | `simple-connection.test.js` | Basic connectivity | `/get` |
| 21 | `simple.test.js` | Basic request | `/cookies` |
| 22 | `simple.ts` | TypeScript example | Any endpoint |
| 23 | `sni-override.test.ts` | SNI manipulation | Custom SNI server |
| 24 | `sse.test.ts` | Server-Sent Events | `/sse/{n}` |
| 25 | `streaming.test.js` | Response streaming | `/stream/{n}` |
| 26 | `timeout.test.ts` | Request timeout | `/delay/{n}` |
| 27 | `urlencoded.test.ts` | URL-encoded forms | `/post` |
| 28 | `websocket.test.ts` | WebSocket support | `/ws` |

### Go Integration Tests (25 total)

| # | Test File | Purpose | Server Needs |
|---|-----------|---------|--------------|
| 1 | `binary_data_test.go` | Binary data handling | `/post`, `/bytes/{n}` |
| 2 | `connection_reuse_test.go` | Connection pooling | Local server |
| 3 | `cookie_test.go` | Cookie handling | `/cookies` |
| 4 | `cookiejar_test.go` | Cookie jar | `/cookies/set` with headers |
| 5 | `custom_timeout_test.go` | Timeout handling | `/delay/{n}` |
| 6 | `decoding_test.go` | Compression | `/brotli`, `/deflate`, `/gzip` |
| 7 | `disable_redirect_test.go` | Redirect control | Actual redirects |
| 8 | `ForceHTTP1_test.go` | HTTP/1.1 forcing | `/api/all` |
| 9 | `http3_test.go` | HTTP/3 support | UDP 443 (already works) |
| 10 | `images_test.go` | Image download | `/image/*` |
| 11 | `InsecureSkipVerify_test.go` | Bad SSL handling | External sites |
| 12 | `issue_407_connection_reuse_test.go` | Connection reuse bug | Local server |
| 13 | `latest_fingerprint_test.go` | Fingerprint testing | `/api/all` |
| 14 | `main_ja3_test.go` | JA3 fingerprinting | `/api/all` |
| 15 | `multipart_formdata_test.go` | Multipart uploads | `/post` |
| 16 | `multiple_requests_test.go` | Multiple requests | Multiple endpoints |
| 17 | `panic_regression_test.go` | Stability testing | Any endpoint |
| 18 | `proxy_test.go` | Proxy support | Local proxy |
| 19 | `quic_test.go` | QUIC protocol | UDP 443 |
| 20 | `sse_only_test.go` | SSE parsing | `/sse/{n}` |
| 21 | `sse_test.go` | SSE with CycleTLS | `/sse/{n}` |
| 22 | `tls_13_test.go` | TLS 1.3 support | `/api/all` |
| 23 | `tls13_auto_retry_test.go` | TLS retry logic | `/api/all` |
| 24 | `UrlEncodedFormData_test.go` | Form encoding | `/post` |
| 25 | `websocket_test.go` | WebSocket support | `/ws` |

---

## Summary

### What Works Today
- **~35 tests** can run against tlsfingerprint.com with URL changes
- All fingerprinting tests (JA3, JA4, HTTP/2) work perfectly
- Compression, images, delays, basic HTTP methods all work

### What Needs Fixing (4 behavior changes)
1. `/redirect/{n}` - return actual 302 redirects
2. `/status/{code}` - return actual status codes
3. `/cookies/set` - add Set-Cookie headers
4. `/redirect-to` - return actual redirects

### What Needs Adding (2 new endpoints)
1. `/stream/{n}` - streaming JSON responses
2. `/ws` - WebSocket echo server

### What Cannot Be Tested (5 tests)
- Tests requiring local servers with special behavior
- Tests requiring external bad SSL sites
- Proxy tests (need local proxy)
