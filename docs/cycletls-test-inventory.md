# CycleTLS Test Inventory

Complete inventory of all CycleTLS tests grouped by functionality.

---

## TLS Fingerprinting

Tests that verify TLS fingerprint generation and accuracy.

| Test File | Language | Description |
|-----------|----------|-------------|
| `ja4-fingerprint.test.js` | TS/JS | JA4_r fingerprint validation for Firefox, Chrome 138/139, TLS 1.2 |
| `http2-fingerprint.test.js` | TS/JS | HTTP/2 Akamai fingerprint for Firefox and Chrome |
| `frameHeader.test.ts` | TS/JS | HTTP/2 SETTINGS and WINDOW_UPDATE frame validation |
| `main_ja3_test.go` | Go | JA3 fingerprint generation and validation |
| `latest_fingerprint_test.go` | Go | Latest browser fingerprint accuracy |
| `tls_13_test.go` | Go | TLS 1.3 specific fingerprint testing |
| `tls13_auto_retry_test.go` | Go | TLS 1.3 retry logic with fingerprint preservation |

---

## HTTP Version Control

Tests for forcing specific HTTP protocol versions.

| Test File | Language | Description |
|-----------|----------|-------------|
| `forceHTTP1.test.ts` | TS/JS | Force HTTP/1.1 instead of HTTP/2 |
| `ForceHTTP1_test.go` | Go | Force HTTP/1.1 protocol |
| `http3_test.go` | Go | HTTP/3 (QUIC) protocol support |
| `quic_test.go` | Go | QUIC protocol testing |

---

## HTTP Methods

Tests for different HTTP request methods.

| Test File | Language | Description |
|-----------|----------|-------------|
| `integration.test.ts` | TS/JS | GET, POST, PUT, PATCH, DELETE methods |
| `multipleRequests.test.js` | TS/JS | Concurrent requests with multiple methods |
| `multiple_requests_test.go` | Go | Multiple sequential and concurrent requests |

---

## Cookies

Tests for cookie handling and cookie jar integration.

| Test File | Language | Description |
|-----------|----------|-------------|
| `cookie.test.ts` | TS/JS | Simple and complex cookie formats |
| `cookiejar.test.js` | TS/JS | Cookie jar with tough-cookie library |
| `cookie_test.go` | Go | Cookie sending and receiving |
| `cookiejar_test.go` | Go | Cookie jar persistence across requests |

---

## Compression / Encoding

Tests for handling compressed responses.

| Test File | Language | Description |
|-----------|----------|-------------|
| `encoding.test.ts` | TS/JS | Brotli, deflate, gzip decompression |
| `decoding_test.go` | Go | Automatic decompression of responses |

---

## Binary Data

Tests for binary data upload/download integrity.

| Test File | Language | Description |
|-----------|----------|-------------|
| `binary-data-handling.test.js` | TS/JS | Binary upload/download, hash verification, UTF-8 edge cases |
| `images.test.ts` | TS/JS | Image download (JPEG, PNG, SVG, WebP) |
| `binary_data_test.go` | Go | Binary data preservation |
| `images_test.go` | Go | Image download integrity |

---

## Form Data

Tests for form submissions (multipart and URL-encoded).

| Test File | Language | Description |
|-----------|----------|-------------|
| `multipartFormData.test.ts` | TS/JS | Multipart form data with file uploads |
| `urlencoded.test.ts` | TS/JS | URL-encoded form data (application/x-www-form-urlencoded) |
| `multipart_formdata_test.go` | Go | Multipart form uploads |
| `UrlEncodedFormData_test.go` | Go | URL-encoded form submissions |

---

## Streaming

Tests for streaming response handling.

| Test File | Language | Description |
|-----------|----------|-------------|
| `streaming.test.js` | TS/JS | Node.js stream events, chunk counting, JSON line parsing |
| `flow-control.test.ts` | TS/JS | Backpressure, memory-efficient large downloads (1MB) |

---

## Server-Sent Events (SSE)

Tests for SSE streaming support.

| Test File | Language | Description |
|-----------|----------|-------------|
| `sse.test.ts` | TS/JS | SSE connection, event parsing, multiple events |
| `sse_test.go` | Go | SSE with CycleTLS client |
| `sse_only_test.go` | Go | SSE-specific parsing |

---

## WebSocket

Tests for WebSocket protocol support.

| Test File | Language | Description |
|-----------|----------|-------------|
| `websocket.test.ts` | TS/JS | WebSocket echo, ping/pong handling |
| `websocket_test.go` | Go | WebSocket client with TLS fingerprinting |

---

## Timeouts

Tests for request and read timeout handling.

| Test File | Language | Description |
|-----------|----------|-------------|
| `timeout.test.ts` | TS/JS | Request timeout with `/delay` endpoint |
| `read-timeout.test.ts` | TS/JS | Read timeout during body streaming (requires custom server) |
| `custom_timeout_test.go` | Go | Custom timeout configuration |

---

## Redirects

Tests for redirect following and control.

| Test File | Language | Description |
|-----------|----------|-------------|
| `disableRedirect.test.ts` | TS/JS | Disable redirect following, `finalUrl` tracking |
| `disable_redirect_test.go` | Go | Redirect control with `disableRedirect` option |

---

## SSL/TLS Certificate Handling

Tests for certificate validation and bypass.

| Test File | Language | Description |
|-----------|----------|-------------|
| `insecureSkipVerify.test.ts` | TS/JS | Skip certificate verification, expired/self-signed certs |
| `InsecureSkipVerify_test.go` | Go | `insecureSkipVerify` option |
| `sni-override.test.ts` | TS/JS | SNI override for domain fronting (requires custom server) |

---

## Connection Management

Tests for connection reuse and pooling.

| Test File | Language | Description |
|-----------|----------|-------------|
| `connectionReuse.test.ts` | TS/JS | TLS handshake counting, connection reuse (requires custom server) |
| `connection_reuse_test.go` | Go | Connection pooling verification |
| `issue_407_connection_reuse_test.go` | Go | Bug fix regression test for connection reuse |

---

## Response Parsing

Tests for response body parsing methods.

| Test File | Language | Description |
|-----------|----------|-------------|
| `response-methods.test.js` | TS/JS | `json()`, `text()`, `arrayBuffer()`, `blob()` methods |

---

## Client Architecture

Tests for client instantiation and multi-instance scenarios.

| Test File | Language | Description |
|-----------|----------|-------------|
| `multipleImports.test.ts` | TS/JS | Multiple CycleTLS instances on same port |
| `simple-connection.test.js` | TS/JS | Basic connectivity validation |
| `simple.test.js` | TS/JS | Basic GET request |
| `panic_regression_test.go` | Go | Stability and panic prevention |

---

## Proxy Support

Tests for HTTP/SOCKS proxy functionality.

| Test File | Language | Description |
|-----------|----------|-------------|
| `proxy_test.go` | Go | Proxy support testing (requires local proxy) |

---

## Summary by Language

### TypeScript/JavaScript Tests (27)

| Category | Tests |
|----------|-------|
| TLS Fingerprinting | 3 |
| HTTP Version | 1 |
| HTTP Methods | 2 |
| Cookies | 2 |
| Compression | 1 |
| Binary Data | 2 |
| Form Data | 2 |
| Streaming | 2 |
| SSE | 1 |
| WebSocket | 1 |
| Timeouts | 2 |
| Redirects | 1 |
| SSL/TLS | 2 |
| Connection | 1 |
| Response Parsing | 1 |
| Client Architecture | 3 |

### Go Integration Tests (25)

| Category | Tests |
|----------|-------|
| TLS Fingerprinting | 4 |
| HTTP Version | 3 |
| HTTP Methods | 1 |
| Cookies | 2 |
| Compression | 1 |
| Binary Data | 2 |
| Form Data | 2 |
| SSE | 2 |
| WebSocket | 1 |
| Timeouts | 1 |
| Redirects | 1 |
| SSL/TLS | 1 |
| Connection | 2 |
| Client Architecture | 1 |
| Proxy | 1 |

---

## Test Requirements Summary

### Can Run Against tlsfingerprint.com

| Category | TS/JS | Go | Total |
|----------|-------|-----|-------|
| TLS Fingerprinting | 3 | 4 | 7 |
| HTTP Version | 1 | 3 | 4 |
| HTTP Methods | 2 | 1 | 3 |
| Cookies | 2 | 2 | 4 |
| Compression | 1 | 1 | 2 |
| Binary Data | 2 | 2 | 4 |
| Form Data | 2 | 2 | 4 |
| Streaming | 2 | 0 | 2 |
| SSE | 1 | 2 | 3 |
| WebSocket | 1 | 1 | 2 |
| Timeouts | 1 | 1 | 2 |
| Redirects | 1 | 1 | 2 |
| Response Parsing | 1 | 0 | 1 |
| Client Architecture | 3 | 1 | 4 |
| **Total** | **23** | **21** | **44** |

### Require External Services or Local Servers

| Test | Reason |
|------|--------|
| `insecureSkipVerify.test.ts` | Needs badssl.com |
| `InsecureSkipVerify_test.go` | Needs badssl.com |
| `connectionReuse.test.ts` | Needs custom server with handshake counting |
| `sni-override.test.ts` | Needs custom server with SNI inspection |
| `read-timeout.test.ts` | Needs server that stalls mid-body |
| `connection_reuse_test.go` | Needs custom server |
| `issue_407_connection_reuse_test.go` | Needs custom server |
| `proxy_test.go` | Needs local proxy server |
