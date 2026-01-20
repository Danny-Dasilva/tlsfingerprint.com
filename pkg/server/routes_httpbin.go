package server

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/pagpeter/trackme/pkg/types"
	"github.com/pagpeter/trackme/pkg/utils"
)

// =============================================================================
// RouteResponse - Extended response type for special handling
// =============================================================================

// RouteResponse encapsulates response data with optional metadata for redirects,
// cookies, and custom status codes
type RouteResponse struct {
	Body        []byte
	ContentType string
	StatusCode  int               // 0 means use default (200 or extracted from path)
	Headers     map[string]string // Additional headers (e.g., Set-Cookie, Location)
	IsRedirect  bool              // True if this is a redirect response
}

// =============================================================================
// Helper Functions
// =============================================================================

// buildTLSFields extracts TLS fingerprint fields from types.Response
func buildTLSFields(res types.Response) map[string]interface{} {
	fields := make(map[string]interface{})

	if res.TLS != nil {
		fields["ja3"] = res.TLS.JA3
		fields["ja3_hash"] = res.TLS.JA3Hash
		fields["ja4"] = res.TLS.JA4
		fields["ja4_r"] = res.TLS.JA4_r
		fields["peetprint"] = res.TLS.PeetPrint
		fields["peetprint_hash"] = res.TLS.PeetPrintHash
	}

	// Akamai fingerprint (HTTP/2 only)
	if res.Http2 != nil {
		fields["akamai"] = res.Http2.AkamaiFingerprint
		fields["akamai_hash"] = res.Http2.AkamaiFingerprintHash
	} else {
		fields["akamai"] = "-"
		fields["akamai_hash"] = "-"
	}

	fields["http_version"] = res.HTTPVersion
	return fields
}

// extractHeaders extracts headers from HTTP/1 or HTTP/2 response
func extractHeaders(res types.Response) map[string]string {
	headers := make(map[string]string)

	if res.Http1 != nil {
		for _, h := range res.Http1.Headers {
			parts := strings.SplitN(h, ": ", 2)
			if len(parts) == 2 {
				headers[parts[0]] = parts[1]
			}
		}
	}

	if res.Http2 != nil {
		for _, frame := range res.Http2.SendFrames {
			if frame.Type == "HEADERS" {
				for _, h := range frame.Headers {
					// Skip pseudo-headers
					if strings.HasPrefix(h, ":") {
						continue
					}
					parts := strings.SplitN(h, ": ", 2)
					if len(parts) == 2 {
						// HTTP/2 headers are lowercase by spec; normalize to title case
						// for compatibility with HTTP/1 style header names
						headerName := normalizeHeaderName(parts[0])
						// Handle multiple headers with same name (e.g., multiple cookie headers)
						// by concatenating them with "; "
						if existing, ok := headers[headerName]; ok {
							headers[headerName] = existing + "; " + parts[1]
						} else {
							headers[headerName] = parts[1]
						}
					}
				}
			}
		}
	}

	return headers
}

// normalizeHeaderName converts lowercase HTTP/2 header names to title case
// e.g., "user-agent" -> "User-Agent", "x-custom-header" -> "X-Custom-Header"
func normalizeHeaderName(name string) string {
	parts := strings.Split(name, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(string(part[0])) + part[1:]
		}
	}
	return strings.Join(parts, "-")
}

// extractBody extracts the request body from HTTP/2 DATA frames
func extractBody(res types.Response) []byte {
	if res.Http2 != nil {
		var body []byte
		for _, frame := range res.Http2.SendFrames {
			if frame.Type == "DATA" {
				body = append(body, frame.Payload...)
			}
		}
		return body
	}
	return nil
}

// buildBaseResponse creates the common response structure with TLS fields
func buildBaseResponse(res types.Response, params url.Values) map[string]interface{} {
	response := buildTLSFields(res)

	// Add common fields
	response["origin"] = cleanIP(res.IP)
	response["method"] = res.Method
	response["url"] = "https://tls.peet.ws" + res.Path

	// Convert params to args
	args := make(map[string]interface{})
	for k, v := range params {
		if len(v) == 1 {
			args[k] = v[0]
		} else {
			args[k] = v
		}
	}
	response["args"] = args

	return response
}

// toJSON marshals response to JSON bytes
func toJSON(data interface{}) []byte {
	j, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return []byte("{\"error\": \"JSON encoding failed\"}")
	}
	return j
}

// =============================================================================
// Echo Endpoints: /get, /post, /put, /patch, /delete, /anything
// =============================================================================

// httpbinGet handles GET /get - echoes request details
func httpbinGet(res types.Response, params url.Values) ([]byte, string) {
	response := buildBaseResponse(res, params)
	response["headers"] = extractHeaders(res)
	return toJSON(response), "application/json"
}

// httpbinPost handles POST /post - echoes POST body and form data
func httpbinPost(res types.Response, params url.Values) ([]byte, string) {
	response := buildBaseResponse(res, params)
	response["headers"] = extractHeaders(res)

	// Extract body from HTTP/2 DATA frames
	body := extractBody(res)
	if len(body) > 0 {
		response["data"] = string(body)
		// Try to parse as JSON
		var jsonData interface{}
		if json.Unmarshal(body, &jsonData) == nil {
			response["json"] = jsonData
		} else {
			response["json"] = nil
		}
	} else {
		response["data"] = ""
		response["json"] = nil
	}
	response["files"] = map[string]interface{}{}
	response["form"] = map[string]interface{}{}

	return toJSON(response), "application/json"
}

// httpbinPut handles PUT /put
func httpbinPut(res types.Response, params url.Values) ([]byte, string) {
	return httpbinPost(res, params)
}

// httpbinPatch handles PATCH /patch
func httpbinPatch(res types.Response, params url.Values) ([]byte, string) {
	return httpbinPost(res, params)
}

// httpbinDelete handles DELETE /delete
func httpbinDelete(res types.Response, params url.Values) ([]byte, string) {
	response := buildBaseResponse(res, params)
	response["headers"] = extractHeaders(res)
	return toJSON(response), "application/json"
}

// httpbinAnything handles any method to /anything
func httpbinAnything(res types.Response, params url.Values) ([]byte, string) {
	response := buildBaseResponse(res, params)
	response["headers"] = extractHeaders(res)

	// Extract body from HTTP/2 DATA frames
	body := extractBody(res)
	if len(body) > 0 {
		response["data"] = string(body)
		// Try to parse as JSON
		var jsonData interface{}
		if json.Unmarshal(body, &jsonData) == nil {
			response["json"] = jsonData
		} else {
			response["json"] = nil
		}
	} else {
		response["data"] = ""
		response["json"] = nil
	}
	response["files"] = map[string]interface{}{}
	response["form"] = map[string]interface{}{}

	return toJSON(response), "application/json"
}

// =============================================================================
// Request Inspection: /headers, /ip, /user-agent
// =============================================================================

// httpbinHeaders handles GET /headers - returns request headers
func httpbinHeaders(res types.Response, params url.Values) ([]byte, string) {
	response := buildTLSFields(res)
	response["headers"] = extractHeaders(res)
	return toJSON(response), "application/json"
}

// httpbinIP handles GET /ip - returns client IP
func httpbinIP(res types.Response, params url.Values) ([]byte, string) {
	response := buildTLSFields(res)
	response["origin"] = cleanIP(res.IP)
	return toJSON(response), "application/json"
}

// httpbinUserAgent handles GET /user-agent - returns User-Agent
func httpbinUserAgent(res types.Response, params url.Values) ([]byte, string) {
	response := buildTLSFields(res)
	response["user-agent"] = res.UserAgent
	return toJSON(response), "application/json"
}

// =============================================================================
// Compression Endpoints: /gzip, /deflate, /brotli
// =============================================================================

// httpbinGzip handles GET /gzip - returns gzip-compressed response
func httpbinGzip(res types.Response, params url.Values) ([]byte, string) {
	response := buildBaseResponse(res, params)
	response["headers"] = extractHeaders(res)
	response["gzipped"] = true

	jsonData := toJSON(response)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write(jsonData)
	gz.Close()

	return buf.Bytes(), "application/json; charset=utf-8"
}

// httpbinDeflate handles GET /deflate - returns deflate-compressed response
// Note: HTTP "deflate" Content-Encoding expects zlib format (RFC 1950), not raw DEFLATE (RFC 1951)
func httpbinDeflate(res types.Response, params url.Values) ([]byte, string) {
	response := buildBaseResponse(res, params)
	response["headers"] = extractHeaders(res)
	response["deflated"] = true

	jsonData := toJSON(response)

	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write(jsonData)
	zw.Close()

	return buf.Bytes(), "application/json; charset=utf-8"
}

// httpbinBrotli handles GET /brotli - returns brotli-compressed response
func httpbinBrotli(res types.Response, params url.Values) ([]byte, string) {
	response := buildBaseResponse(res, params)
	response["headers"] = extractHeaders(res)
	response["brotli"] = true

	jsonData := toJSON(response)

	var buf bytes.Buffer
	bw := brotli.NewWriter(&buf)
	bw.Write(jsonData)
	bw.Close()

	return buf.Bytes(), "application/json; charset=utf-8"
}

// =============================================================================
// Cookie Endpoints: /cookies, /cookies/set, /cookies/delete
// =============================================================================

// httpbinCookies handles GET /cookies - returns cookies from request
func httpbinCookies(res types.Response, params url.Values) ([]byte, string) {
	response := buildTLSFields(res)

	// Extract cookies from headers
	// Headers are normalized to title case, so "Cookie" works for both HTTP/1 and HTTP/2
	cookies := make(map[string]string)
	headers := extractHeaders(res)
	if cookieHeader, ok := headers["Cookie"]; ok {
		parts := strings.Split(cookieHeader, "; ")
		for _, part := range parts {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				cookies[kv[0]] = kv[1]
			}
		}
	}

	response["cookies"] = cookies
	return toJSON(response), "application/json"
}

// httpbinCookiesSet handles GET /cookies/set - sets cookies via query params
// Returns Set-Cookie headers for each query parameter
func httpbinCookiesSet(res types.Response, params url.Values) ([]byte, string) {
	response := buildTLSFields(res)

	// Build cookies and Set-Cookie header list
	cookies := make(map[string]string)
	var setCookies []string
	for k, v := range params {
		if len(v) > 0 {
			cookies[k] = v[0]
			setCookies = append(setCookies, k+"="+v[0]+"; Path=/")
		}
	}

	response["cookies"] = cookies

	// Return special content-type that signals Set-Cookie headers
	// Format: "set-cookies:COOKIE1|COOKIE2|...:application/json"
	// The body follows normal JSON format
	if len(setCookies) > 0 {
		cookieList := strings.Join(setCookies, "|")
		return toJSON(response), "set-cookies:" + cookieList + ":application/json"
	}

	return toJSON(response), "application/json"
}

// httpbinCookiesDelete handles GET /cookies/delete - deletes cookies
func httpbinCookiesDelete(res types.Response, params url.Values) ([]byte, string) {
	response := buildTLSFields(res)
	response["cookies"] = map[string]string{}
	return toJSON(response), "application/json"
}

// =============================================================================
// Binary/Image Endpoints: /image/jpeg, /image/png, /image/svg, /image/gif, /image/webp
// =============================================================================

// Minimal valid JPEG (1x1 red pixel)
var jpegImage = []byte{
	0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
	0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
	0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
	0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
	0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
	0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
	0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
	0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
	0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00,
	0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	0x09, 0x0A, 0x0B, 0xFF, 0xC4, 0x00, 0xB5, 0x10, 0x00, 0x02, 0x01, 0x03,
	0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7D,
	0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
	0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
	0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0, 0x24, 0x33, 0x62, 0x72,
	0x82, 0x09, 0x0A, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x25, 0x26, 0x27, 0x28,
	0x29, 0x2A, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3A, 0x43, 0x44, 0x45,
	0x46, 0x47, 0x48, 0x49, 0x4A, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
	0x5A, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6A, 0x73, 0x74, 0x75,
	0x76, 0x77, 0x78, 0x79, 0x7A, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
	0x8A, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0xA2, 0xA3,
	0xA4, 0xA5, 0xA6, 0xA7, 0xA8, 0xA9, 0xAA, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6,
	0xB7, 0xB8, 0xB9, 0xBA, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9,
	0xCA, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9, 0xDA, 0xE1, 0xE2,
	0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xF1, 0xF2, 0xF3, 0xF4,
	0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01,
	0x00, 0x00, 0x3F, 0x00, 0xFB, 0xD5, 0xDB, 0x20, 0xA8, 0xF1, 0x7E, 0xCA,
	0xB2, 0x2F, 0x1F, 0xFF, 0xD9,
}

// Minimal valid PNG (1x1 red pixel)
var pngImage = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE, 0x00, 0x00, 0x00,
	0x0C, 0x49, 0x44, 0x41, 0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
	0x00, 0x00, 0x03, 0x00, 0x01, 0x00, 0x05, 0xFE, 0xD4, 0xEF, 0x00, 0x00,
	0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
}

// Simple SVG
var svgImage = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100">
  <circle cx="50" cy="50" r="40" fill="blue"/>
</svg>`)

// Minimal valid GIF (1x1 red pixel)
var gifImage = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00,
	0x00, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x21, 0xF9, 0x04, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x2C, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00,
	0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3B,
}

// Minimal valid WebP (1x1 red pixel, lossy)
var webpImage = []byte{
	0x52, 0x49, 0x46, 0x46, 0x1A, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50,
	0x56, 0x50, 0x38, 0x4C, 0x0D, 0x00, 0x00, 0x00, 0x2F, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFE, 0xFB, 0x94, 0x00, 0x00,
}

func httpbinImageJPEG(res types.Response, params url.Values) ([]byte, string) {
	return jpegImage, "image/jpeg"
}

func httpbinImagePNG(res types.Response, params url.Values) ([]byte, string) {
	return pngImage, "image/png"
}

func httpbinImageSVG(res types.Response, params url.Values) ([]byte, string) {
	return svgImage, "image/svg+xml"
}

func httpbinImageGIF(res types.Response, params url.Values) ([]byte, string) {
	return gifImage, "image/gif"
}

func httpbinImageWebP(res types.Response, params url.Values) ([]byte, string) {
	return webpImage, "image/webp"
}

// httpbinBytes handles /bytes/{n}
// GET: returns n random bytes
// POST/PUT: echoes back the request body (for binary data testing)
func httpbinBytes(res types.Response, params url.Values) ([]byte, string) {
	// For POST/PUT requests, echo back the body for binary testing
	if res.Method == "POST" || res.Method == "PUT" {
		body := extractBody(res)
		if len(body) > 0 {
			return body, "application/octet-stream"
		}
	}

	// GET behavior: Extract n from path: /bytes/100
	path := res.Path
	parts := strings.Split(path, "/")
	n := 100 // default
	if len(parts) >= 3 {
		if parsed, err := strconv.Atoi(parts[2]); err == nil && parsed > 0 && parsed <= 102400 {
			n = parsed
		}
	}

	// Generate random-ish bytes (deterministic for testing)
	data := make([]byte, n)
	for i := 0; i < n; i++ {
		data[i] = byte(i % 256)
	}

	return data, "application/octet-stream"
}

// httpbinBase64 handles GET /base64/{value} - decodes base64 and returns
func httpbinBase64(res types.Response, params url.Values) ([]byte, string) {
	// Extract value from path: /base64/SGVsbG8gV29ybGQ=
	path := res.Path
	parts := strings.Split(path, "/")
	if len(parts) >= 3 {
		encoded := parts[2]
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err == nil {
			return decoded, "text/html; charset=utf-8"
		}
	}
	return []byte("Invalid base64"), "text/plain"
}

// =============================================================================
// Redirect Endpoints: /redirect/{n}, /redirect-to, /status/{code}
// =============================================================================

// httpbinRedirect handles GET /redirect/{n} - returns 302 redirect
// Redirects to /redirect/{n-1} until n=1, then redirects to /get
func httpbinRedirect(res types.Response, params url.Values) ([]byte, string) {
	// Extract n from path
	path := res.Path
	parts := strings.Split(path, "/")
	n := 1
	if len(parts) >= 3 {
		if parsed, err := strconv.Atoi(parts[2]); err == nil && parsed > 0 && parsed <= 10 {
			n = parsed
		}
	}

	var location string
	if n > 1 {
		location = "/redirect/" + strconv.Itoa(n-1)
	} else {
		location = "/get"
	}

	// Return special content-type that signals redirect to connection_handler
	// Format: "redirect:STATUS_CODE:LOCATION"
	return []byte{}, "redirect:302:" + location
}

// httpbinRedirectTo handles /redirect-to?url=...
// Returns 302 redirect to the specified URL
func httpbinRedirectTo(res types.Response, params url.Values) ([]byte, string) {
	targetURL := utils.GetParam("url", params)
	if targetURL == "" {
		targetURL = "/get"
	}
	// Also support status_code parameter for different redirect types
	statusCode := 302
	if sc := utils.GetParam("status_code", params); sc != "" {
		if parsed, err := strconv.Atoi(sc); err == nil && parsed >= 300 && parsed < 400 {
			statusCode = parsed
		}
	}

	// Return special content-type that signals redirect to connection_handler
	return []byte{}, "redirect:" + strconv.Itoa(statusCode) + ":" + targetURL
}

// httpbinStatus handles /status/{code}
func httpbinStatus(res types.Response, params url.Values) ([]byte, string) {
	// Extract status code from path
	path := res.Path
	parts := strings.Split(path, "/")
	code := 200
	if len(parts) >= 3 {
		if parsed, err := strconv.Atoi(parts[2]); err == nil && parsed >= 100 && parsed < 600 {
			code = parsed
		}
	}

	response := buildTLSFields(res)
	response["status_code"] = code
	return toJSON(response), "application/json"
}

// =============================================================================
// Delay Endpoint: /delay/{seconds}
// =============================================================================

// httpbinDelay handles /delay/{seconds} - delays response
func httpbinDelay(res types.Response, params url.Values) ([]byte, string) {
	// Extract seconds from path
	path := res.Path
	parts := strings.Split(path, "/")
	seconds := 1
	if len(parts) >= 3 {
		if parsed, err := strconv.Atoi(parts[2]); err == nil && parsed > 0 && parsed <= 10 {
			seconds = parsed
		}
	}

	// Actually delay
	time.Sleep(time.Duration(seconds) * time.Second)

	response := buildBaseResponse(res, params)
	response["headers"] = extractHeaders(res)
	response["delay"] = seconds

	return toJSON(response), "application/json"
}

// =============================================================================
// Response Format Endpoints: /html, /xml, /json
// =============================================================================

func httpbinHTML(res types.Response, params url.Values) ([]byte, string) {
	html := `<!DOCTYPE html>
<html>
<head><title>TLS Fingerprint HTTPBin</title></head>
<body>
<h1>Hello from TLS Fingerprint HTTPBin!</h1>
<p>JA3 Hash: ` + res.TLS.JA3Hash + `</p>
</body>
</html>`
	return []byte(html), "text/html; charset=utf-8"
}

func httpbinXML(res types.Response, params url.Values) ([]byte, string) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<response>
  <ja3_hash>` + res.TLS.JA3Hash + `</ja3_hash>
  <origin>` + cleanIP(res.IP) + `</origin>
</response>`
	return []byte(xml), "application/xml"
}

func httpbinJSON(res types.Response, params url.Values) ([]byte, string) {
	response := buildTLSFields(res)
	response["slideshow"] = map[string]interface{}{
		"author": "TLS Fingerprint",
		"title":  "Sample Slideshow",
	}
	return toJSON(response), "application/json"
}

func httpbinRobots(res types.Response, params url.Values) ([]byte, string) {
	return []byte("User-agent: *\nDisallow: /deny\n"), "text/plain"
}

func httpbinDeny(res types.Response, params url.Values) ([]byte, string) {
	return []byte("YOU SHOULDN'T BE HERE"), "text/plain"
}

// =============================================================================
// SSE Endpoint: /sse, /sse/{n}
// =============================================================================

// httpbinSSE handles /sse - returns SSE-formatted response
// Note: True SSE streaming requires connection_handler modification
// This returns a complete SSE response that CycleTLS can parse
func httpbinSSE(res types.Response, params url.Values) ([]byte, string) {
	// Extract count from path if present: /sse/5
	path := res.Path
	parts := strings.Split(path, "/")
	count := 3 // default
	if len(parts) >= 3 {
		if parsed, err := strconv.Atoi(parts[2]); err == nil && parsed > 0 && parsed <= 100 {
			count = parsed
		}
	}

	ja3Hash := ""
	if res.TLS != nil {
		ja3Hash = res.TLS.JA3Hash
	}

	var buf bytes.Buffer
	for i := 1; i <= count; i++ {
		data := map[string]interface{}{
			"count":    i,
			"ja3_hash": ja3Hash,
		}
		jsonData, _ := json.Marshal(data)
		buf.WriteString("event: message\n")
		buf.WriteString("id: " + strconv.Itoa(i) + "\n")
		buf.WriteString("data: " + string(jsonData) + "\n\n")
	}

	// Final done event
	buf.WriteString("event: done\n")
	buf.WriteString("id: " + strconv.Itoa(count+1) + "\n")
	buf.WriteString("data: {\"total\": " + strconv.Itoa(count) + "}\n\n")

	return buf.Bytes(), "text/event-stream"
}

// =============================================================================
// Stream Endpoint: /stream/{n}
// =============================================================================

// httpbinStream handles /stream/{n} - returns n newline-delimited JSON objects
// This is compatible with HTTPBin's /stream endpoint used by CycleTLS tests
func httpbinStream(res types.Response, params url.Values) ([]byte, string) {
	// Extract n from path: /stream/5
	path := res.Path
	parts := strings.Split(path, "/")
	n := 3 // default
	if len(parts) >= 3 {
		if parsed, err := strconv.Atoi(parts[2]); err == nil && parsed > 0 && parsed <= 100 {
			n = parsed
		}
	}

	ja3Hash := ""
	if res.TLS != nil {
		ja3Hash = res.TLS.JA3Hash
	}

	var buf bytes.Buffer
	for i := 0; i < n; i++ {
		data := map[string]interface{}{
			"id":       i,
			"ja3_hash": ja3Hash,
			"origin":   cleanIP(res.IP),
			"url":      "https://tlsfingerprint.com" + res.Path,
		}
		jsonData, _ := json.Marshal(data)
		buf.Write(jsonData)
		buf.WriteByte('\n')
	}

	return buf.Bytes(), "application/json"
}

// =============================================================================
// Register all HTTPBin routes
// =============================================================================

// getHTTPBinPaths returns all httpbin-compatible routes
func getHTTPBinPaths() map[string]func(types.Response, url.Values) ([]byte, string) {
	return map[string]func(types.Response, url.Values) ([]byte, string){
		// Echo endpoints
		"/get":      httpbinGet,
		"/post":     httpbinPost,
		"/put":      httpbinPut,
		"/patch":    httpbinPatch,
		"/delete":   httpbinDelete,
		"/anything": httpbinAnything,

		// Request inspection
		"/headers":    httpbinHeaders,
		"/ip":         httpbinIP,
		"/user-agent": httpbinUserAgent,

		// Compression
		"/gzip":    httpbinGzip,
		"/deflate": httpbinDeflate,
		"/brotli":  httpbinBrotli,

		// Cookies
		"/cookies":        httpbinCookies,
		"/cookies/set":    httpbinCookiesSet,
		"/cookies/delete": httpbinCookiesDelete,

		// Binary/Images
		"/image/jpeg": httpbinImageJPEG,
		"/image/png":  httpbinImagePNG,
		"/image/svg":  httpbinImageSVG,
		"/image/gif":  httpbinImageGIF,
		"/image/webp": httpbinImageWebP,

		// Response formats
		"/html":       httpbinHTML,
		"/xml":        httpbinXML,
		"/json":       httpbinJSON,
		"/robots.txt": httpbinRobots,
		"/deny":       httpbinDeny,
	}
}

// getDynamicHTTPBinPaths returns handlers for dynamic path patterns
// These need prefix matching in the router
func getDynamicHTTPBinPaths() map[string]func(types.Response, url.Values) ([]byte, string) {
	return map[string]func(types.Response, url.Values) ([]byte, string){
		"/bytes/":      httpbinBytes,
		"/base64/":     httpbinBase64,
		"/redirect/":   httpbinRedirect,
		"/redirect-to": httpbinRedirectTo,
		"/status/":     httpbinStatus,
		"/delay/":      httpbinDelay,
		"/sse":         httpbinSSE,
		"/sse/":        httpbinSSE,
		"/stream/":     httpbinStream,
		"/anything/":   httpbinAnything,
	}
}

// =============================================================================
// OpenAPI Specification Endpoint
// =============================================================================

// httpbinOpenAPI returns the OpenAPI 3.0 specification for all httpbin endpoints
func httpbinOpenAPI(res types.Response, params url.Values) ([]byte, string) {
	spec := map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title":       "TLS Fingerprint HTTPBin API",
			"description": "A simple HTTP Request & Response Service with TLS fingerprinting. All responses include JA3, JA4, PeetPrint, and Akamai fingerprints.",
			"version":     "1.0.0",
			"contact": map[string]string{
				"name": "TLS Fingerprint",
				"url":  "https://tlsfingerprint.com",
			},
		},
		"servers": []map[string]string{
			{"url": "https://tlsfingerprint.com", "description": "Production server"},
			{"url": "https://localhost:8443", "description": "Local development"},
		},
		"tags": []map[string]string{
			{"name": "TLS Fingerprinting", "description": "TLS fingerprint and SNI inspection"},
			{"name": "HTTP Methods", "description": "Testing different HTTP verbs"},
			{"name": "Request Inspection", "description": "Inspect request details"},
			{"name": "Compression", "description": "Compressed responses"},
			{"name": "Cookies", "description": "Cookie operations"},
			{"name": "Images", "description": "Binary image responses"},
			{"name": "Response Formats", "description": "Different response formats"},
			{"name": "Redirects", "description": "Redirect operations"},
			{"name": "Dynamic", "description": "Dynamic response generation"},
			{"name": "WebSocket", "description": "WebSocket echo endpoint (HTTP/3 only)"},
		},
		"paths": buildOpenAPIPaths(),
	}
	return toJSON(spec), "application/json"
}

func buildOpenAPIPaths() map[string]interface{} {
	return map[string]interface{}{
		"/get": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"HTTP Methods"},
				"summary":     "Returns GET request data",
				"description": "Returns the request's query parameters, headers, and TLS fingerprints",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Successful response",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]string{"$ref": "#/components/schemas/EchoResponse"},
							},
						},
					},
				},
			},
		},
		"/post": map[string]interface{}{
			"post": map[string]interface{}{
				"tags":        []string{"HTTP Methods"},
				"summary":     "Returns POST request data",
				"description": "Returns the request's body, form data, headers, and TLS fingerprints",
				"requestBody": map[string]interface{}{
					"content": map[string]interface{}{
						"application/json":                  map[string]interface{}{"schema": map[string]string{"type": "object"}},
						"application/x-www-form-urlencoded": map[string]interface{}{"schema": map[string]string{"type": "object"}},
					},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Successful response",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]string{"$ref": "#/components/schemas/EchoResponse"},
							},
						},
					},
				},
			},
		},
		"/put": map[string]interface{}{
			"put": map[string]interface{}{
				"tags":    []string{"HTTP Methods"},
				"summary": "Returns PUT request data",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Successful response"},
				},
			},
		},
		"/patch": map[string]interface{}{
			"patch": map[string]interface{}{
				"tags":    []string{"HTTP Methods"},
				"summary": "Returns PATCH request data",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Successful response"},
				},
			},
		},
		"/delete": map[string]interface{}{
			"delete": map[string]interface{}{
				"tags":    []string{"HTTP Methods"},
				"summary": "Returns DELETE request data",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Successful response"},
				},
			},
		},
		"/anything": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"HTTP Methods"},
				"summary": "Returns anything passed in request data (accepts any method)",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Successful response"},
				},
			},
		},
		"/headers": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Request Inspection"},
				"summary": "Returns request headers",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Headers in response"},
				},
			},
		},
		"/ip": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Request Inspection"},
				"summary": "Returns the client's IP address",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "IP address"},
				},
			},
		},
		"/user-agent": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Request Inspection"},
				"summary": "Returns the User-Agent header",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "User-Agent string"},
				},
			},
		},
		"/gzip": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Compression"},
				"summary": "Returns gzip-compressed response",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Gzip-encoded response"},
				},
			},
		},
		"/deflate": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Compression"},
				"summary": "Returns deflate-compressed response",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Deflate-encoded response"},
				},
			},
		},
		"/brotli": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Compression"},
				"summary": "Returns brotli-compressed response",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Brotli-encoded response"},
				},
			},
		},
		"/cookies": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Cookies"},
				"summary": "Returns cookies from the request",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Cookies object"},
				},
			},
		},
		"/cookies/set": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Cookies"},
				"summary": "Sets cookies via query parameters",
				"parameters": []map[string]interface{}{
					{"name": "name", "in": "query", "schema": map[string]string{"type": "string"}, "description": "Cookie name=value pairs"},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Set-Cookie headers in response"},
				},
			},
		},
		"/cookies/delete": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Cookies"},
				"summary": "Deletes cookies via query parameters",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Expired Set-Cookie headers"},
				},
			},
		},
		"/image/jpeg": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Images"},
				"summary": "Returns a JPEG image",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "JPEG image", "content": map[string]interface{}{"image/jpeg": map[string]interface{}{}}},
				},
			},
		},
		"/image/png": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Images"},
				"summary": "Returns a PNG image",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "PNG image"},
				},
			},
		},
		"/image/svg": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Images"},
				"summary": "Returns an SVG image",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "SVG image"},
				},
			},
		},
		"/image/gif": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Images"},
				"summary": "Returns a GIF image",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "GIF image"},
				},
			},
		},
		"/image/webp": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Images"},
				"summary": "Returns a WebP image",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "WebP image"},
				},
			},
		},
		"/html": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Response Formats"},
				"summary": "Returns HTML response",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "HTML page"},
				},
			},
		},
		"/xml": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Response Formats"},
				"summary": "Returns XML response",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "XML document"},
				},
			},
		},
		"/json": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Response Formats"},
				"summary": "Returns JSON response",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "JSON object"},
				},
			},
		},
		"/robots.txt": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Response Formats"},
				"summary": "Returns robots.txt",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Robots.txt file"},
				},
			},
		},
		"/deny": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Response Formats"},
				"summary": "Returns denied message",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Access denied text"},
				},
			},
		},
		"/bytes/{n}": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Dynamic"},
				"summary": "Returns n random bytes",
				"parameters": []map[string]interface{}{
					{"name": "n", "in": "path", "required": true, "schema": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 102400}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Random bytes"},
				},
			},
		},
		"/base64/{value}": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Dynamic"},
				"summary": "Decodes base64 string",
				"parameters": []map[string]interface{}{
					{"name": "value", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Decoded value"},
				},
			},
		},
		"/redirect/{n}": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Redirects"},
				"summary": "Redirect chain with n redirects",
				"parameters": []map[string]interface{}{
					{"name": "n", "in": "path", "required": true, "schema": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 10}},
				},
				"responses": map[string]interface{}{
					"302": map[string]interface{}{"description": "Redirect response"},
				},
			},
		},
		"/redirect-to": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Redirects"},
				"summary": "Redirect to specified URL",
				"parameters": []map[string]interface{}{
					{"name": "url", "in": "query", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"302": map[string]interface{}{"description": "Redirect to URL"},
				},
			},
		},
		"/status/{code}": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Dynamic"},
				"summary": "Returns specified HTTP status code",
				"parameters": []map[string]interface{}{
					{"name": "code", "in": "path", "required": true, "schema": map[string]interface{}{"type": "integer", "minimum": 100, "maximum": 599}},
				},
				"responses": map[string]interface{}{
					"default": map[string]interface{}{"description": "Response with specified status"},
				},
			},
		},
		"/delay/{seconds}": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Dynamic"},
				"summary": "Delays response by n seconds",
				"parameters": []map[string]interface{}{
					{"name": "seconds", "in": "path", "required": true, "schema": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 10}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Delayed response"},
				},
			},
		},
		"/sse": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Dynamic"},
				"summary": "Server-Sent Events stream",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "SSE stream"},
				},
			},
		},
		"/stream/{n}": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":    []string{"Dynamic"},
				"summary": "Streams n newline-delimited JSON objects",
				"parameters": []map[string]interface{}{
					{"name": "n", "in": "path", "required": true, "schema": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 100}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Newline-delimited JSON objects"},
				},
			},
		},
		"/ws": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"WebSocket"},
				"summary":     "WebSocket echo endpoint",
				"description": "Upgrades to WebSocket connection and echoes back any message received. Note: WebSocket is only available over HTTP/3.",
				"responses": map[string]interface{}{
					"101": map[string]interface{}{"description": "Switching Protocols - WebSocket connection established"},
				},
			},
		},
		"/api/sni": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"TLS Fingerprinting"},
				"summary":     "Returns the SNI (Server Name Indication) from TLS handshake",
				"description": "Extracts and returns the SNI hostname sent during TLS handshake. Useful for verifying SNI override functionality.",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "SNI information",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"sni":          map[string]string{"type": "string", "description": "Server Name Indication hostname"},
										"ip":           map[string]string{"type": "string", "description": "Client IP address"},
										"http_version": map[string]string{"type": "string", "description": "HTTP version (h1, h2, h3)"},
									},
								},
							},
						},
					},
				},
			},
		},
		"/api/all": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"TLS Fingerprinting"},
				"summary":     "Returns complete TLS fingerprint data",
				"description": "Returns full TLS fingerprint including JA3, JA4, PeetPrint, Akamai fingerprint, and all extensions",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Complete fingerprint response"},
				},
			},
		},
		"/api/tls": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"TLS Fingerprinting"},
				"summary":     "Returns TLS-only fingerprint data",
				"description": "Returns only the TLS fingerprint data (JA3, JA4, extensions) without HTTP details",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "TLS fingerprint response"},
				},
			},
		},
		"/api/clean": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"TLS Fingerprinting"},
				"summary":     "Returns clean fingerprint summary",
				"description": "Returns a minimal fingerprint summary with just the hash values",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Clean fingerprint response"},
				},
			},
		},
	}
}
