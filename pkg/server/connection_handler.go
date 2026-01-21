package server

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pagpeter/quic-go/http3"
	trackmehttp "github.com/pagpeter/trackme/pkg/http"
	"github.com/pagpeter/trackme/pkg/tls"
	"github.com/pagpeter/trackme/pkg/types"
	utls "github.com/wwhtrbbtt/utls"
	"golang.org/x/net/http2"
)

const HTTP2_PREAMBLE = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

// generateRequestID generates a simple random ID for request tracking
func generateRequestID() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

// extractStatusCode extracts HTTP status code from /status/{code} paths
func extractStatusCode(path string) int {
	if !strings.HasPrefix(path, "/status/") {
		return 200
	}
	parts := strings.Split(path, "/")
	if len(parts) >= 3 {
		if code, err := strconv.Atoi(parts[2]); err == nil && code >= 100 && code < 600 {
			return code
		}
	}
	return 200
}

func parseHTTP1(request []byte) types.Response {
	// Split the request into lines
	lines := strings.Split(string(request), "\r\n")

	// Split the first line into the method, path and http version
	firstLine := strings.Split(lines[0], " ")

	// Split the headers into an array
	var headers []string
	var userAgent string
	for _, line := range lines {
		if strings.Contains(line, ":") {
			headers = append(headers, line)
			if strings.HasPrefix(strings.ToLower(line), "user-agent") {
				userAgent = strings.TrimSpace(strings.Split(line, ":")[1])
			}
		}
	}

	if len(firstLine) != 3 {
		return types.Response{
			HTTPVersion: "--",
			Method:      "--",
			Path:        "--",
		}
	}
	return types.Response{
		HTTPVersion: firstLine[2],
		Path:        firstLine[1],
		Method:      firstLine[0],
		UserAgent:   userAgent,
		Http1: &types.Http1Details{
			Headers: headers,
		},
	}
}

func (srv *Server) HandleTLSConnection(conn net.Conn) bool {
	// Read the first line of the request
	// We only read the first line to determine if the connection is HTTP1 or HTTP2
	// If we know that it isnt HTTP2, we can read the rest of the request and then start processing it
	// If we know that it is HTTP2, we start the HTTP2 handler

	l := len([]byte(HTTP2_PREAMBLE))
	request := make([]byte, l)

	_, err := conn.Read(request)
	if err != nil {
		//log.Println("Error reading request", err)
		if strings.HasSuffix(err.Error(), "unknown certificate") && srv.IsLocal() {
			log.Println("Local error (probably developement) - not closing conn")
			return true
		}
		return false
	}

	hs := conn.(*utls.Conn).ClientHello

	parsedClientHello := tls.ParseClientHello(hs)
	JA3Data := tls.CalculateJA3(parsedClientHello)
	peetfp, peetprintHash := tls.CalculatePeetPrint(parsedClientHello, JA3Data)

	// Calculate JA4 directly from ClientHello (improved method)
	negotiatedVersion := fmt.Sprintf("%v", conn.(*utls.Conn).ConnectionState().Version)
	ja4 := tls.CalculateJa4Direct(parsedClientHello, negotiatedVersion)
	ja4_r := tls.CalculateJa4Direct_r(parsedClientHello, negotiatedVersion)

	// Convert raw bytes to hex and base64
	rawBytes, _ := hex.DecodeString(hs)
	rawB64 := base64.StdEncoding.EncodeToString(rawBytes)

	tlsDetails := types.TLSDetails{
		Ciphers:          JA3Data.ReadableCiphers,
		Extensions:       parsedClientHello.Extensions,
		RecordVersion:    JA3Data.Version,
		NegotiatedVesion: negotiatedVersion,
		JA3:              JA3Data.JA3,
		JA3Hash:          JA3Data.JA3Hash,
		JA4:              ja4,
		JA4_r:            ja4_r,
		PeetPrint:        peetfp,
		PeetPrintHash:    peetprintHash,
		SessionID:        parsedClientHello.SessionID,
		ClientRandom:     parsedClientHello.ClientRandom,
		RawBytes:         hs,
		RawB64:           rawB64,
	}

	// Check if the first line is HTTP/2
	if string(request) == HTTP2_PREAMBLE {
		srv.handleHTTP2(conn, &tlsDetails)
	} else {
		// Read the rest of the request
		r2 := make([]byte, 1024-l)
		_, err := conn.Read(r2)
		if err != nil {
			log.Println(err)
			return true
		}
		// Append it to the first line
		request = append(request, r2...)

		// Parse and handle the request
		details := parseHTTP1(request)
		details.IP = conn.RemoteAddr().String()
		details.TLS = &tlsDetails

		// Calculate JA4H for HTTP/1
		if details.Http1 != nil && details.TLS != nil {
			details.TLS.JA4H = trackmehttp.CalculateJA4H(details.Method, details.HTTPVersion, details.Http1.Headers)
			details.TLS.JA4H_r = trackmehttp.CalculateJA4H_r(details.Method, details.HTTPVersion, details.Http1.Headers)
		}

		srv.respondToHTTP1(conn, details)
	}
	return true
}

func (srv *Server) respondToHTTP1(conn net.Conn, resp types.Response) {
	// log.Println("Request:", resp.ToJson())
	// log.Println(len(resp.ToJson()))

	// Track request timing
	startTime := time.Now()
	requestID := generateRequestID()

	var isAdmin bool
	var res []byte
	var ctype = "text/plain"
	if resp.Method != "OPTIONS" {
		res, ctype = Router(resp.Path, resp, srv)
	} else {
		isAdmin = true
	}

	key, isKeySet := srv.GetAdmin()
	if isKeySet {
		for _, a := range resp.Http1.Headers {
			if strings.HasPrefix(a, key) {
				isAdmin = true
			}
		}
	}

	// Parse special content-type directives
	var extraHeaders []string
	statusCode := extractStatusCode(resp.Path)

	// Handle redirect responses: "redirect:STATUS:LOCATION"
	if strings.HasPrefix(ctype, "redirect:") {
		parts := strings.SplitN(ctype, ":", 3)
		if len(parts) >= 3 {
			if code, err := strconv.Atoi(parts[1]); err == nil {
				statusCode = code
			}
			location := parts[2]
			extraHeaders = append(extraHeaders, "Location: "+location)
			ctype = "text/html; charset=utf-8"
			res = []byte{}
		}
	}

	// Handle Set-Cookie responses: "set-cookies:COOKIE1|COOKIE2:ACTUAL_CONTENT_TYPE"
	if strings.HasPrefix(ctype, "set-cookies:") {
		parts := strings.SplitN(ctype, ":", 3)
		if len(parts) >= 3 {
			cookies := strings.Split(parts[1], "|")
			for _, cookie := range cookies {
				extraHeaders = append(extraHeaders, "Set-Cookie: "+cookie)
			}
			ctype = parts[2]
		}
	}

	// Calculate response time
	responseTime := time.Since(startTime).Milliseconds()

	res1 := fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, http.StatusText(statusCode))
	res1 += "Content-Length: " + fmt.Sprintf("%v\r\n", len(res))
	res1 += "Content-Type: " + ctype + "; charset=utf-8\r\n"

	// Add request tracking headers
	res1 += "X-Request-Id: " + requestID + "\r\n"
	res1 += "X-Response-Time: " + fmt.Sprintf("%d\r\n", responseTime)

	// Add extra headers (redirects, cookies)
	for _, h := range extraHeaders {
		res1 += h + "\r\n"
	}

	// Add Content-Encoding header for compression endpoints
	if strings.HasPrefix(resp.Path, "/gzip") {
		res1 += "Content-Encoding: gzip\r\n"
	} else if strings.HasPrefix(resp.Path, "/deflate") {
		res1 += "Content-Encoding: deflate\r\n"
	} else if strings.HasPrefix(resp.Path, "/brotli") {
		res1 += "Content-Encoding: br\r\n"
	}

	if isAdmin {
		res1 += "Access-Control-Allow-Origin: *\r\n"
		res1 += "Access-Control-Allow-Methods: *\r\n"
		res1 += "Access-Control-Allow-Headers: *\r\n"
	}
	res1 += "Server: TrackMe\r\n"
	res1 += "Alt-Svc: h3=\":443\"; ma=86400\r\n"
	res1 += "\r\n"
	res1 += string(res)
	res1 += "\r\n\r\n"

	_, err := conn.Write([]byte(res1))
	if err != nil {
		log.Println("Error writing HTTP/1 data", err)
		return
	}

	err = conn.Close()
	if err != nil {
		log.Println("Error closing HTTP/1 connection", err)
		return
	}
}

// https://stackoverflow.com/questions/52002623/golang-tcp-server-how-to-write-http2-data
func (srv *Server) handleHTTP2(conn net.Conn, tlsFingerprint *types.TLSDetails) {
	fr := http2.NewFramer(conn, conn)
	h2conn := NewHTTP2Connection(conn, fr, tlsFingerprint, srv)

	// Send initial SETTINGS
	// Same settings that google uses
	err := fr.WriteSettings(
		http2.Setting{
			ID: http2.SettingInitialWindowSize, Val: 1048576,
		},
		http2.Setting{
			ID: http2.SettingMaxConcurrentStreams, Val: 100,
		},
		http2.Setting{
			ID: http2.SettingMaxHeaderListSize, Val: 65536,
		},
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

// HandleHTTP3 handles HTTP/3 requests and returns a simple "Hello, World!" response
func (srv *Server) HandleHTTP3() http.Handler {
	mux := http.NewServeMux()

	// WebSocket endpoint - must be registered before the catch-all "/" handler
	mux.HandleFunc("/ws", HandleWebSocket)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Track request timing
		startTime := time.Now()
		requestID := generateRequestID()

		if h3w, ok := w.(*http3.ResponseWriter); ok {
			h3c := h3w.Connection()
			h3state := h3c.ConnectionState()

			h3c.Settings()

			// Safely extract connection state and settings
			var used0RTT, supportsDatagrams, supportsStreamResetPartialDelivery bool
			var version uint32
			var gso bool
			var settings types.Http3Settings

			used0RTT = h3state.Used0RTT
			supportsDatagrams = h3state.SupportsDatagrams
			supportsStreamResetPartialDelivery = h3state.SupportsStreamResetPartialDelivery
			version = uint32(h3state.Version)
			gso = h3state.GSO
			if h3c != nil && h3c.Settings() != nil {
				settings = types.Http3Settings(*h3c.Settings())
			}

			resp := types.Response{
				IP:          r.RemoteAddr,
				HTTPVersion: "h3",
				Path:        r.URL.Path,
				Method:      r.Method,
				UserAgent:   r.Header.Get("User-Agent"),
				Http3: &types.Http3Details{
					Information:                        "HTTP/3 support is work-in-progress. Use https://fp.impersonate.pro/api/http3 in the meantime.",
					Used0RTT:                           used0RTT,
					SupportsDatagrams:                  supportsDatagrams,
					SupportsStreamResetPartialDelivery: supportsStreamResetPartialDelivery,
					Version:                            version,
					GSO:                                gso,
					Settings:                           settings,
				},
			}

			res, ctype := Router(r.URL.Path, resp, srv)

			// Calculate response time
			responseTime := time.Since(startTime).Milliseconds()

			// Handle redirect responses: "redirect:STATUS:LOCATION"
			if strings.HasPrefix(ctype, "redirect:") {
				parts := strings.SplitN(ctype, ":", 3)
				if len(parts) >= 3 {
					statusCode := 302
					if code, err := strconv.Atoi(parts[1]); err == nil {
						statusCode = code
					}
					location := parts[2]
					w.Header().Set("X-Request-Id", requestID)
					w.Header().Set("X-Response-Time", strconv.FormatInt(responseTime, 10))
					w.Header().Set("Location", location)
					w.WriteHeader(statusCode)
					return
				}
			}

			// Handle Set-Cookie responses: "set-cookies:COOKIE1|COOKIE2:ACTUAL_CONTENT_TYPE"
			if strings.HasPrefix(ctype, "set-cookies:") {
				parts := strings.SplitN(ctype, ":", 3)
				if len(parts) >= 3 {
					cookies := strings.Split(parts[1], "|")
					for _, cookie := range cookies {
						w.Header().Add("Set-Cookie", cookie)
					}
					ctype = parts[2]
				}
			}

			w.Header().Set("Content-Type", ctype)
			w.Header().Set("Server", "TrackMe")
			w.Header().Set("X-Request-Id", requestID)
			w.Header().Set("X-Response-Time", strconv.FormatInt(responseTime, 10))
			w.Write([]byte(res))
		}
	})

	return mux
}
