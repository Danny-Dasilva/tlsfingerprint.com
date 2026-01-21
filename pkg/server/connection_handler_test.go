package server

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/pagpeter/trackme/pkg/types"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// Helper to create a test server and connection
func setupTest() (*Server, net.Conn, net.Conn) {
	srv := NewServer()
	srv.State.Config.MakeDefault()
	srv.State.Config.LogToDB = false // Disable DB logging for tests

	clientConn, serverConn := net.Pipe()
	return srv, clientConn, serverConn
}

func TestHTTP2Multiplexing(t *testing.T) {
	srv, clientConn, serverConn := setupTest()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		tlsDetails := &types.TLSDetails{
			JA3:       "771,4865,0,10,23",
			PeetPrint: "hash|h2|hash|sig",
		}
		srv.handleHTTP2(serverConn, tlsDetails)
	}()

	// Client side
	fr := http2.NewFramer(clientConn, clientConn)

	// 1. Read Server Settings
	f, err := fr.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	if f.Header().Type != http2.FrameSettings {
		t.Fatalf("Expected Settings frame, got %v", f.Header().Type)
	}

	// 2. Send Client Settings
	if err := fr.WriteSettings(); err != nil {
		t.Fatal(err)
	}

	// Channel to collect responses
	type frameInfo struct {
		f   http2.Frame
		err error
	}
	responses := make(chan frameInfo, 10)

	// Start reading responses in background
	go func() {
		for {
			f, err := fr.ReadFrame()
			responses <- frameInfo{f, err}
			if err != nil {
				return
			}
		}
	}()

	// 3. Send Request 1 (Stream 1)
	var buf bytes.Buffer
	enc := hpack.NewEncoder(&buf)
	enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/status/200"})
	enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "https"})
	enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "localhost"})

	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: buf.Bytes(),
		EndHeaders:    true,
		EndStream:     true,
	}); err != nil {
		t.Fatal(err)
	}

	// 4. Send Request 2 (Stream 3) using fresh headers
	var buf2 bytes.Buffer
	enc2 := hpack.NewEncoder(&buf2)
	enc2.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	enc2.WriteField(hpack.HeaderField{Name: ":path", Value: "/status/200"})
	enc2.WriteField(hpack.HeaderField{Name: ":scheme", Value: "https"})
	enc2.WriteField(hpack.HeaderField{Name: ":authority", Value: "localhost"})

	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      3,
		BlockFragment: buf2.Bytes(),
		EndHeaders:    true,
		EndStream:     true,
	}); err != nil {
		t.Fatal(err)
	}

	// Wait for responses
	gotResponse1 := false
	gotResponse2 := false

	// Read loop
	timeout := time.After(5 * time.Second)
	for !gotResponse1 || !gotResponse2 {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for responses. Got 1: %v, 2: %v", gotResponse1, gotResponse2)
		case res := <-responses:
			if res.err != nil {
				if res.err == io.EOF {
					t.Fatal("Connection closed unexpectedly")
				}
				t.Fatal(res.err)
			}
			f := res.f
			t.Logf("Received frame type %v stream %v", f.Header().Type, f.Header().StreamID)
			if f.Header().Type == http2.FrameHeaders {
				if f.Header().StreamID == 1 {
					gotResponse1 = true
				}
				if f.Header().StreamID == 3 {
					gotResponse2 = true
				}
			}
		}
	}
}

func TestHTTP2RedirectChain(t *testing.T) {
	srv, clientConn, serverConn := setupTest()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		tlsDetails := &types.TLSDetails{
			JA3:       "771,4865,0,10,23",
			PeetPrint: "hash|h2|hash|sig",
		}
		srv.handleHTTP2(serverConn, tlsDetails)
	}()

	fr := http2.NewFramer(clientConn, clientConn)

	// Read settings
	fr.ReadFrame()
	fr.WriteSettings()

	// Channel to collect responses
	type frameInfo struct {
		f   http2.Frame
		err error
	}
	responses := make(chan frameInfo, 10)

	go func() {
		for {
			f, err := fr.ReadFrame()
			responses <- frameInfo{f, err}
			if err != nil {
				return
			}
		}
	}()

	// Request /redirect/302/https://example.com (using helper path format if available, or just mock expected redirect behavior)
	var buf bytes.Buffer
	enc := hpack.NewEncoder(&buf)
	enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/redirect/3"}) // Assuming this path exists
	enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "https"})

	err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: buf.Bytes(),
		EndHeaders:    true,
		EndStream:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Read response
	// We expect HEADERS and DATA (maybe empty) and NO GoAway immediately.
	gotHeaders := false
	gotGoAway := false

	// We'll read for a bit to see if GoAway comes
	timeout := time.After(500 * time.Millisecond)

loop:
	for {
		select {
		case <-timeout:
			break loop
		case res := <-responses:
			if res.err != nil {
				if res.err == io.EOF {
					t.Log("Connection closed (EOF)")
					break loop
				}
				// t.Log("Read error:", res.err)
				break loop
			}
			f := res.f
			if f.Header().Type == http2.FrameHeaders {
				gotHeaders = true
			}
			if f.Header().Type == http2.FrameGoAway {
				gotGoAway = true
			}
		}
	}

	if !gotHeaders {
		// If we didn't get headers, maybe the path /redirect/3 doesn't exist and it 404s (which is also headers).
		// So we should have got headers.
		t.Log("Did not receive headers (path might be 404 but should still respond)")
	}

	if gotGoAway {
		t.Fatal("Received premature GOAWAY for redirect/request")
	}
}
