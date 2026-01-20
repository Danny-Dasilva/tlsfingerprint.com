package server

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/pagpeter/trackme/pkg/types"
	"github.com/pagpeter/trackme/pkg/utils"
)

func staticFile(file string) func(types.Response, url.Values) ([]byte, string) {
	return func(types.Response, url.Values) ([]byte, string) {
		b, _ := utils.ReadFile(file)
		return b, "text/html"
	}
}

func apiAll(res types.Response, _ url.Values) ([]byte, string) {
	return []byte(res.ToJson()), "application/json"
}

func apiTLS(res types.Response, _ url.Values) ([]byte, string) {
	return []byte(types.Response{
		TLS: res.TLS,
	}.ToJson()), "application/json"
}

func apiClean(res types.Response, _ url.Values) ([]byte, string) {
	akamai := "-"
	hash := "-"
	if res.HTTPVersion == "h2" {
		akamai = res.Http2.AkamaiFingerprint
		hash = utils.GetMD5Hash(res.Http2.AkamaiFingerprint)
	}

	smallRes := types.SmallResponse{
		Akamai:     akamai,
		AkamaiHash: hash,
	}

	if res.TLS != nil {
		smallRes.JA3 = res.TLS.JA3
		smallRes.JA3Hash = res.TLS.JA3Hash
		smallRes.JA4 = res.TLS.JA4
		smallRes.JA4_r = res.TLS.JA4_r
		smallRes.JA4H = res.TLS.JA4H
		smallRes.JA4H_r = res.TLS.JA4H_r
		smallRes.PeetPrint = res.TLS.PeetPrint
		smallRes.PeetPrintHash = res.TLS.PeetPrintHash
	}

	return []byte(smallRes.ToJson()), "application/json"
}

func apiRaw(res types.Response, _ url.Values) ([]byte, string) {
	return []byte(fmt.Sprintf(`{"raw": "%s", "raw_b64": "%s"}`, res.TLS.RawBytes, res.TLS.RawB64)), "application/json"
}

// apiSNI extracts and returns the Server Name Indication (SNI) from TLS handshake
// This allows clients to verify their SNI override is working correctly
func apiSNI(res types.Response, _ url.Values) ([]byte, string) {
	sni := ""
	if res.TLS != nil {
		// Extract SNI from extensions array
		for _, ext := range res.TLS.Extensions {
			if m, ok := ext.(map[string]interface{}); ok {
				if serverName, ok := m["server_name"].(string); ok && serverName != "" {
					sni = serverName
					break
				}
			}
		}
	}
	response := map[string]interface{}{
		"sni":         sni,
		"ip":          res.IP,
		"http_version": res.HTTPVersion,
	}
	j, _ := json.Marshal(response)
	return j, "application/json"
}

func apiRequestCount(srv *Server) func(types.Response, url.Values) ([]byte, string) {
	return func(_ types.Response, _ url.Values) ([]byte, string) {
		if !srv.IsConnectedToDB() {
			return []byte("{\"error\": \"Not connected to database.\"}"), "application/json"
		}
		return []byte(fmt.Sprintf(`{"total_requests": %v}`, GetTotalRequestCount(srv))), "application/json"
	}
}

// apiSearchHandler creates a search endpoint handler with common validation logic
func apiSearchHandler(srv *Server, searchFn func(string, *Server) interface{}) func(types.Response, url.Values) ([]byte, string) {
	return func(_ types.Response, u url.Values) ([]byte, string) {
		if !srv.IsConnectedToDB() {
			return []byte("{\"error\": \"Not connected to database.\"}"), "application/json"
		}
		by := utils.GetParam("by", u)
		if by == "" {
			return []byte("{\"error\": \"No 'by' param present\"}"), "application/json"
		}
		res := searchFn(by, srv)
		j, _ := json.MarshalIndent(res, "", "\t")
		return j, "application/json"
	}
}

func apiSearchJA3(srv *Server) func(types.Response, url.Values) ([]byte, string) {
	return apiSearchHandler(srv, func(by string, s *Server) interface{} { return GetByJa3(by, s) })
}

func apiSearchH2(srv *Server) func(types.Response, url.Values) ([]byte, string) {
	return apiSearchHandler(srv, func(by string, s *Server) interface{} { return GetByH2(by, s) })
}

func apiSearchPeetPrint(srv *Server) func(types.Response, url.Values) ([]byte, string) {
	return apiSearchHandler(srv, func(by string, s *Server) interface{} { return GetByPeetPrint(by, s) })
}

func apiSearchUserAgent(srv *Server) func(types.Response, url.Values) ([]byte, string) {
	return apiSearchHandler(srv, func(by string, s *Server) interface{} { return GetByUserAgent(by, s) })
}

func index(r types.Response, v url.Values) ([]byte, string) {
	res, ct := staticFile("static/index.html")(r, v)
	data, _ := json.Marshal(r)
	return []byte(strings.ReplaceAll(string(res), "/*DATA*/", string(data))), ct
}

func apiSearchJA4(srv *Server) func(types.Response, url.Values) ([]byte, string) {
	return apiSearchHandler(srv, func(by string, s *Server) interface{} { return GetByJA4(by, s) })
}

func apiSearchJA4H(srv *Server) func(types.Response, url.Values) ([]byte, string) {
	return apiSearchHandler(srv, func(by string, s *Server) interface{} { return GetByJA4H(by, s) })
}

func getAllPaths(srv *Server) map[string]func(types.Response, url.Values) ([]byte, string) {
	// Start with existing routes
	paths := map[string]func(types.Response, url.Values) ([]byte, string){
		"/":                     index,
		"/explore":              staticFile("static/explore.html"),
		"/docs":                 staticFile("static/docs.html"),
		"/openapi.json":         httpbinOpenAPI,
		"/api/all":              apiAll,
		"/api/tls":              apiTLS,
		"/api/clean":            apiClean,
		"/api/raw":              apiRaw,
		"/api/sni":              apiSNI,
		"/api/request-count":    apiRequestCount(srv),
		"/api/search-ja3":       apiSearchJA3(srv),
		"/api/search-ja4":       apiSearchJA4(srv),
		"/api/search-ja4h":      apiSearchJA4H(srv),
		"/api/search-h2":        apiSearchH2(srv),
		"/api/search-peetprint": apiSearchPeetPrint(srv),
		"/api/search-useragent": apiSearchUserAgent(srv),
	}

	// Add HTTPBin-compatible routes
	for path, handler := range getHTTPBinPaths() {
		paths[path] = handler
	}

	return paths
}

// getDynamicPaths returns handlers that match path prefixes (e.g., /delay/5)
func getDynamicPaths() map[string]func(types.Response, url.Values) ([]byte, string) {
	return getDynamicHTTPBinPaths()
}
