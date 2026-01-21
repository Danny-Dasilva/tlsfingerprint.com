package server

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	trackmehttp "github.com/pagpeter/trackme/pkg/http"
	"github.com/pagpeter/trackme/pkg/types"
	"github.com/pagpeter/trackme/pkg/utils"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

type HTTP2Connection struct {
	conn           net.Conn
	framer         *http2.Framer
	tlsFingerprint *types.TLSDetails

	// Stream management
	streams      map[uint32]*HTTP2Stream
	streamsMu    sync.RWMutex
	lastStreamID uint32

	// Connection lifecycle
	maxStreams   uint32
	idleTimeout  time.Duration
	lastActivity time.Time
	closing      bool
	closeMu      sync.Mutex
	writeMu      sync.Mutex

	// Server reference
	srv *Server

	// HPACK Decoder for the connection
	hpackDecoder *hpack.Decoder

	// Connection level frames for fingerprinting (SETTINGS, etc.)
	connectionFrames []types.ParsedFrame
}

type HTTP2Stream struct {
	streamID uint32
	state    StreamState
	// We store parsed frames for fingerprinting
	frames     []types.ParsedFrame
	response   chan []byte
	bodyClosed bool
	mu         sync.Mutex
}

type StreamState int

const (
	StreamOpen StreamState = iota
	StreamHalfClosedRemote
	StreamHalfClosedLocal
	StreamClosed
)

func NewHTTP2Connection(conn net.Conn, framer *http2.Framer, tlsDetails *types.TLSDetails, srv *Server) *HTTP2Connection {
	decoder := hpack.NewDecoder(4096, func(hf hpack.HeaderField) {})
	decoder.SetEmitEnabled(true)

	return &HTTP2Connection{
		conn:             conn,
		framer:           framer,
		tlsFingerprint:   tlsDetails,
		streams:          make(map[uint32]*HTTP2Stream),
		maxStreams:       100, // Match SETTINGS_MAX_CONCURRENT_STREAMS
		idleTimeout:      30 * time.Second,
		lastActivity:     time.Now(),
		srv:              srv,
		hpackDecoder:     decoder,
		connectionFrames: []types.ParsedFrame{},
	}
}

func (c *HTTP2Connection) GetOrCreateStream(streamID uint32) *HTTP2Stream {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()

	if stream, exists := c.streams[streamID]; exists {
		return stream
	}

	stream := &HTTP2Stream{
		streamID: streamID,
		state:    StreamOpen,
		response: make(chan []byte, 10), // Buffered channel for body chunks
		frames:   []types.ParsedFrame{},
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
		stream.mu.Lock()
		if !stream.bodyClosed {
			close(stream.response)
			stream.bodyClosed = true
		}
		stream.mu.Unlock()
		delete(c.streams, streamID)
	}
}

func (c *HTTP2Connection) ActiveStreamCount() int {
	c.streamsMu.RLock()
	defer c.streamsMu.RUnlock()
	return len(c.streams)
}

func (c *HTTP2Connection) processFrames() {
	defer c.gracefulShutdown()

	for {
		frame, err := c.framer.ReadFrame()
		if err != nil {
			if err == io.EOF || isConnectionClosed(err) {
				return
			}
			// log.Println("Error reading frame:", err)
			return
		}

		c.lastActivity = time.Now()

		// Convert to ParsedFrame for fingerprinting
		parsedFrame := c.convertFrame(frame)

		// Store connection-level frames
		if frame.Header().StreamID == 0 {
			c.connectionFrames = append(c.connectionFrames, parsedFrame)
		}

		switch f := frame.(type) {
		case *http2.SettingsFrame:
			if !f.IsAck() {
				c.writeMu.Lock()
				c.framer.WriteSettingsAck()
				c.writeMu.Unlock()
			}

		case *http2.HeadersFrame:
			// Add frame to stream
			stream := c.GetOrCreateStream(f.StreamID)
			stream.frames = append(stream.frames, parsedFrame)

			// Decode headers synchronously using persistent decoder
			headers, err := c.hpackDecoder.DecodeFull(f.HeaderBlockFragment())
			if err != nil {
				log.Println("Error decoding headers:", err)
				c.sendRSTStream(f.StreamID, http2.ErrCodeProtocol)
				continue
			}

			go c.handleRequest(f.StreamID, headers, f.StreamEnded(), stream)

		case *http2.DataFrame:
			stream := c.GetOrCreateStream(f.StreamID)
			stream.frames = append(stream.frames, parsedFrame)
			c.handleData(f)

		case *http2.WindowUpdateFrame:
			if f.StreamID != 0 {
				stream := c.GetOrCreateStream(f.StreamID)
				stream.frames = append(stream.frames, parsedFrame)
			}
			// Handle flow control (can be expanded later)

		case *http2.PriorityFrame:
			if f.StreamID != 0 {
				stream := c.GetOrCreateStream(f.StreamID)
				stream.frames = append(stream.frames, parsedFrame)
			}

		case *http2.PingFrame:
			if !f.IsAck() {
				c.writeMu.Lock()
				c.framer.WritePing(true, f.Data)
				c.writeMu.Unlock()
			}

		case *http2.GoAwayFrame:
			// Client is closing, respect it
			return

		case *http2.RSTStreamFrame:
			c.CloseStream(f.StreamID)
		}
	}
}

func (c *HTTP2Connection) handleRequest(streamID uint32, headers []hpack.HeaderField, endStream bool, stream *HTTP2Stream) {
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
	if !endStream {
		_ = c.waitForStreamBody(streamID)
	}

	// Combine connection frames and stream frames for fingerprinting
	allFrames := make([]types.ParsedFrame, len(c.connectionFrames)+len(stream.frames))
	copy(allFrames, c.connectionFrames)
	copy(allFrames[len(c.connectionFrames):], stream.frames)

	// Build response object
	resp := types.Response{
		IP:          c.conn.RemoteAddr().String(),
		HTTPVersion: "h2",
		Path:        path,
		Method:      method,
		UserAgent:   userAgent,
		Http2: &types.Http2Details{
			SendFrames:            allFrames,
			AkamaiFingerprint:     trackmehttp.GetAkamaiFingerprint(allFrames),
			AkamaiFingerprintHash: utils.GetMD5Hash(trackmehttp.GetAkamaiFingerprint(allFrames)),
		},
		TLS: c.tlsFingerprint,
	}

	// Calculate JA4H for HTTP/2
	if resp.Http2 != nil && resp.TLS != nil {
		// Extract headers from HTTP/2 frames
		h2Headers := []string{}
		for _, frame := range allFrames {
			if frame.Type == "HEADERS" {
				h2Headers = append(h2Headers, frame.Headers...)
			}
		}
		resp.TLS.JA4H = trackmehttp.CalculateJA4H(resp.Method, resp.HTTPVersion, h2Headers)
		resp.TLS.JA4H_r = trackmehttp.CalculateJA4H_r(resp.Method, resp.HTTPVersion, h2Headers)
	}

	// Route and send response
	c.sendResponse(streamID, resp, path, method)
}

func (c *HTTP2Connection) handleData(f *http2.DataFrame) {
	c.streamsMu.RLock()
	stream, exists := c.streams[f.StreamID]
	c.streamsMu.RUnlock()

	if !exists {
		// Stream might be closed or not created yet (shouldn't happen for DATA usually without HEADERS)
		return
	}

	if f.Data() != nil {
		select {
		case stream.response <- f.Data():
		default:
			// Buffer full or channel closed
		}
	}

	if f.StreamEnded() {
		stream.mu.Lock()
		if !stream.bodyClosed {
			close(stream.response)
			stream.bodyClosed = true
		}
		stream.mu.Unlock()
	}
}

func (c *HTTP2Connection) waitForStreamBody(streamID uint32) []byte {
	c.streamsMu.RLock()
	stream := c.streams[streamID]
	c.streamsMu.RUnlock()

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

func (c *HTTP2Connection) sendResponse(streamID uint32, resp types.Response, path, method string) {
	// Track request timing
	startTime := time.Now()
	requestID := generateRequestID()

	res, ctype := Router(path, resp, c.srv)

	var isAdmin bool
	key, isKeySet := c.srv.GetAdmin()
	if isKeySet && method == "OPTIONS" {
		isAdmin = true
	} else if isKeySet {
		// Headers in HTTP/2 are in resp.Http2.SendFrames, but easier to check if we parsed them
		// For simplicity, we assume Router handled auth checks or we check parsed headers if needed
		// But here we need to know if we should send CORS headers.
		// Let's check parsed headers from the frame logic if available?
		// Actually, Router already ran.
		// Let's just check if we need to add admin headers.
		// In legacy code:
		/*
		   if isKeySet {
		       for _, a := range resp.Http1.Headers { ... }
		   }
		*/
		// We don't have easy access to headers map here without reparsing.
		// But `resp.Http2.SendFrames` has HEADERS frames.
		for _, f := range resp.Http2.SendFrames {
			if f.Type == "HEADERS" {
				for _, h := range f.Headers {
					if strings.HasPrefix(h, key) {
						isAdmin = true
					}
				}
			}
		}
	}

	// Handle redirects
	statusCode := extractStatusCode(path)
	var extraHeaders []hpack.HeaderField

	// Handle redirect responses: "redirect:STATUS:LOCATION"
	if strings.HasPrefix(ctype, "redirect:") {
		parts := strings.SplitN(ctype, ":", 3)
		if len(parts) >= 3 {
			if code, err := strconv.Atoi(parts[1]); err == nil {
				statusCode = code
			}
			location := parts[2]
			extraHeaders = append(extraHeaders, hpack.HeaderField{Name: "location", Value: location})
			ctype = "text/html; charset=utf-8"
			res = []byte{}
		}
	}

	// Handle Set-Cookie responses
	if strings.HasPrefix(ctype, "set-cookies:") {
		parts := strings.SplitN(ctype, ":", 3)
		if len(parts) >= 3 {
			cookies := strings.Split(parts[1], "|")
			for _, cookie := range cookies {
				extraHeaders = append(extraHeaders, hpack.HeaderField{Name: "set-cookie", Value: cookie})
			}
			ctype = parts[2]
		}
	}

	if res == nil {
		res = []byte{}
	}

	responseTime := time.Since(startTime).Milliseconds()

	// Build HEADERS frame
	hbuf := bytes.NewBuffer([]byte{})
	encoder := hpack.NewEncoder(hbuf)
	encoder.WriteField(hpack.HeaderField{Name: ":status", Value: strconv.Itoa(statusCode)})
	encoder.WriteField(hpack.HeaderField{Name: "server", Value: "TrackMe.peet.ws"})
	encoder.WriteField(hpack.HeaderField{Name: "content-length", Value: strconv.Itoa(len(res))})
	encoder.WriteField(hpack.HeaderField{Name: "content-type", Value: ctype})

	// Add request tracking headers
	encoder.WriteField(hpack.HeaderField{Name: "x-request-id", Value: requestID})
	encoder.WriteField(hpack.HeaderField{Name: "x-response-time", Value: strconv.FormatInt(responseTime, 10)})

	for _, h := range extraHeaders {
		encoder.WriteField(h)
	}

	// Add Content-Encoding header
	if strings.HasPrefix(path, "/gzip") {
		encoder.WriteField(hpack.HeaderField{Name: "content-encoding", Value: "gzip"})
	} else if strings.HasPrefix(path, "/deflate") {
		encoder.WriteField(hpack.HeaderField{Name: "content-encoding", Value: "deflate"})
	} else if strings.HasPrefix(path, "/brotli") {
		encoder.WriteField(hpack.HeaderField{Name: "content-encoding", Value: "br"})
	}

	encoder.WriteField(hpack.HeaderField{Name: "alt-svc", Value: "h3=\":443\"; ma=86400"})

	if isAdmin {
		encoder.WriteField(hpack.HeaderField{Name: "access-control-allow-origin", Value: "*"})
		encoder.WriteField(hpack.HeaderField{Name: "access-control-allow-methods", Value: "*"})
		encoder.WriteField(hpack.HeaderField{Name: "access-control-allow-headers", Value: "*"})
	}

	// Write HEADERS
	c.writeMu.Lock()
	err := c.framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: hbuf.Bytes(),
		EndHeaders:    true,
		EndStream:     len(res) == 0, // EndStream if no body
	})
	c.writeMu.Unlock()
	if err != nil {
		log.Println("Error writing headers:", err)
		return
	}

	// Write DATA frames if body exists
	if len(res) > 0 {
		chunks := utils.SplitBytesIntoChunks(res, 16384) // 16KB chunks
		for i, chunk := range chunks {
			endStream := i == len(chunks)-1
			c.writeMu.Lock()
			c.framer.WriteData(streamID, endStream, chunk)
			c.writeMu.Unlock()
		}
	}

	// Close this stream in our map
	c.CloseStream(streamID)

	// DO NOT send GOAWAY or close connection here!
}

func (c *HTTP2Connection) sendRSTStream(streamID uint32, code http2.ErrCode) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.framer.WriteRSTStream(streamID, code)
}

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

func (c *HTTP2Connection) initiateGracefulShutdown() {
	// Send GOAWAY with last processed stream ID
	c.writeMu.Lock()
	c.framer.WriteGoAway(c.lastStreamID, http2.ErrCodeNo, []byte("idle timeout"))
	c.writeMu.Unlock()

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

func (c *HTTP2Connection) convertFrame(frame http2.Frame) types.ParsedFrame {
	p := types.ParsedFrame{}
	p.Type = frame.Header().Type.String()
	p.Stream = frame.Header().StreamID
	p.Length = frame.Header().Length
	p.Flags = utils.GetAllFlags(frame)

	switch frame := frame.(type) {
	case *http2.SettingsFrame:
		p.Settings = []string{}
		frame.ForeachSetting(func(s http2.Setting) error {
			setting := fmt.Sprintf("%q", s)
			setting = strings.Replace(setting, "\"", "", -1)
			setting = strings.Replace(setting, "[", "", -1)
			setting = strings.Replace(setting, "]", "", -1)

			if strings.HasPrefix(setting, "UNKNOWN_SETTING_9 = ") {
				setting = strings.ReplaceAll(setting, "UNKNOWN_SETTING_9", "NO_RFC7540_PRIORITIES")
			}

			p.Settings = append(p.Settings, setting)
			return nil
		})
	case *http2.HeadersFrame:
		d := hpack.NewDecoder(4096, func(hf hpack.HeaderField) {})
		d.SetEmitEnabled(true)
		h2Headers, err := d.DecodeFull(frame.HeaderBlockFragment())
		if err != nil {
			return p
		}

		for _, h := range h2Headers {
			h := fmt.Sprintf("%q: %q", h.Name, h.Value)
			h = strings.Trim(h, "\"")
			h = strings.Replace(h, "\": \"", ": ", -1)
			p.Headers = append(p.Headers, h)
		}
		if frame.HasPriority() {
			prio := types.Priority{}
			p.Priority = &prio
			p.Priority.Weight = int(frame.Priority.Weight) + 1
			p.Priority.DependsOn = int(frame.Priority.StreamDep)
			if frame.Priority.Exclusive {
				p.Priority.Exclusive = 1
			}
		}
	case *http2.DataFrame:
		p.Payload = frame.Data()
	case *http2.WindowUpdateFrame:
		p.Increment = frame.Increment
	case *http2.PriorityFrame:
		prio := types.Priority{}
		p.Priority = &prio
		p.Priority.Weight = int(frame.PriorityParam.Weight) + 1
		p.Priority.DependsOn = int(frame.PriorityParam.StreamDep)
		if frame.PriorityParam.Exclusive {
			p.Priority.Exclusive = 1
		}
	case *http2.GoAwayFrame:
		p.GoAway = &types.GoAway{}
		p.GoAway.LastStreamID = frame.LastStreamID
		p.GoAway.ErrCode = uint32(frame.ErrCode)
		p.GoAway.DebugData = frame.DebugData()
	}

	return p
}

// Helper to check for closed connection
func isConnectionClosed(err error) bool {
	if err == nil {
		return false
	}
	// Check for various "closed connection" errors
	s := err.Error()
	return strings.Contains(s, "use of closed network connection") ||
		strings.Contains(s, "connection reset by peer") ||
		strings.Contains(s, "EOF")
}
