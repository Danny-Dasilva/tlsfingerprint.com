package http

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pagpeter/trackme/pkg/utils"
)

// JA4H implements HTTP client fingerprinting per FoxIO specification
// Format: [method][version][header_count]_[header_hash]_[cookie_hash]
// Example: ge11cn020000_9ed1ff1f7b03_cd8dafe26982

// getMethodPrefix returns first 2 characters of HTTP method in lowercase
func getMethodPrefix(method string) string {
	method = strings.ToLower(method)
	if len(method) >= 2 {
		return method[:2]
	}
	return method
}

// getVersionString converts HTTP version to JA4H format
func getVersionString(httpVersion string) string {
	switch strings.ToLower(httpVersion) {
	case "http/0.9", "0.9":
		return "09"
	case "http/1.0", "1.0":
		return "10"
	case "http/1.1", "1.1", "http/1":
		return "11"
	case "http/2", "http/2.0", "h2", "2", "2.0":
		return "2"
	case "http/3", "http/3.0", "h3", "3", "3.0":
		return "3"
	default:
		return "00"
	}
}

// extractHeaderName extracts the header name from "Name: Value" format
func extractHeaderName(header string) string {
	parts := strings.SplitN(header, ":", 2)
	if len(parts) > 0 {
		return strings.ToLower(strings.TrimSpace(parts[0]))
	}
	return ""
}

// isCookieHeader checks if header is a Cookie header
func isCookieHeader(header string) bool {
	lower := strings.ToLower(header)
	return strings.HasPrefix(lower, "cookie:")
}

// isRefererHeader checks if header is a Referer header
func isRefererHeader(header string) bool {
	lower := strings.ToLower(header)
	return strings.HasPrefix(lower, "referer:") || strings.HasPrefix(lower, "referrer:")
}

// calculateHeaderHash calculates SHA256 hash of sorted header names
// Excludes Cookie and Referer per JA4H spec
func calculateHeaderHash(headers []string) string {
	filtered := []string{}

	for _, header := range headers {
		// Skip Cookie and Referer headers
		if isCookieHeader(header) || isRefererHeader(header) {
			continue
		}

		name := extractHeaderName(header)
		if name != "" {
			filtered = append(filtered, name)
		}
	}

	// Sort headers alphabetically
	sort.Strings(filtered)

	// Join and hash
	if len(filtered) == 0 {
		return "000000000000"
	}

	joined := strings.Join(filtered, ",")
	return utils.SHA256trunc(joined)
}

// extractCookieValue extracts cookie value from header
func extractCookieValue(header string) string {
	parts := strings.SplitN(header, ":", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// calculateCookieHash calculates SHA256 hash of sorted cookies
func calculateCookieHash(headers []string) string {
	cookies := []string{}

	for _, header := range headers {
		if isCookieHeader(header) {
			value := extractCookieValue(header)
			if value != "" {
				cookies = append(cookies, value)
			}
		}
	}

	if len(cookies) == 0 {
		return "000000000000"
	}

	// Sort cookies
	sort.Strings(cookies)

	// Join with semicolon and hash
	joined := strings.Join(cookies, ";")
	return utils.SHA256trunc(joined)
}

// countHeaders counts headers excluding Cookie and Referer
func countHeaders(headers []string) int {
	count := 0
	for _, header := range headers {
		if !isCookieHeader(header) && !isRefererHeader(header) {
			count++
		}
	}
	return count
}

// CalculateJA4H calculates JA4H fingerprint (hashed mode)
func CalculateJA4H(method string, httpVersion string, headers []string) string {
	methodPrefix := getMethodPrefix(method)
	versionStr := getVersionString(httpVersion)

	// Count headers (excluding Cookie and Referer)
	count := countHeaders(headers)
	// Cap at 99 per spec
	if count > 99 {
		count = 99
	}
	headerCount := fmt.Sprintf("%02d", count)

	headerHash := calculateHeaderHash(headers)
	cookieHash := calculateCookieHash(headers)

	return fmt.Sprintf("%s%s%s_%s_%s",
		methodPrefix, versionStr, headerCount, headerHash, cookieHash)
}

// CalculateJA4H_r calculates JA4H fingerprint (raw mode - shows header names)
func CalculateJA4H_r(method string, httpVersion string, headers []string) string {
	methodPrefix := getMethodPrefix(method)
	versionStr := getVersionString(httpVersion)

	// Extract and filter header names
	filtered := []string{}
	cookies := []string{}

	for _, header := range headers {
		if isCookieHeader(header) {
			value := extractCookieValue(header)
			if value != "" {
				cookies = append(cookies, value)
			}
			continue
		}

		if isRefererHeader(header) {
			continue
		}

		name := extractHeaderName(header)
		if name != "" {
			filtered = append(filtered, name)
		}
	}

	// Sort headers and cookies
	sort.Strings(filtered)
	sort.Strings(cookies)

	// Count and cap
	count := len(filtered)
	if count > 99 {
		count = 99
	}
	headerCount := fmt.Sprintf("%02d", count)

	// Join headers and cookies
	headerList := strings.Join(filtered, ",")
	if headerList == "" {
		headerList = "none"
	}

	cookieList := strings.Join(cookies, ";")
	if cookieList == "" {
		cookieList = "none"
	}

	return fmt.Sprintf("%s%s%s_%s_%s",
		methodPrefix, versionStr, headerCount, headerList, cookieList)
}