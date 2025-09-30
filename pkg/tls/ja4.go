package tls

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/pagpeter/trackme/pkg/types"
	"github.com/pagpeter/trackme/pkg/utils"
)

// detectSNI determines if SNI is an IP address or domain name
func detectSNI(parsed ClientHello) string {
	// Extract SNI from extensions
	for _, ext := range parsed.Extensions {
		if m, ok := ext.(map[string]interface{}); ok {
			// Try to get server_name field
			if serverName, ok := m["server_name"].(string); ok && serverName != "" {
				// Check if it's an IP address
				if net.ParseIP(serverName) != nil {
					return "i" // IP address
				}
				return "d" // Domain name
			}
		}
		// Also handle struct types with ServerName field
		if extStruct, ok := ext.(struct {
			Name       string `json:"name"`
			ServerName string `json:"server_name"`
		}); ok && extStruct.ServerName != "" {
			if net.ParseIP(extStruct.ServerName) != nil {
				return "i"
			}
			return "d"
		}
	}
	return "d" // Default to domain if no SNI found
}

// ja4a_direct calculates Part A directly from ClientHello (RECOMMENDED)
func ja4a_direct(parsed ClientHello, negotiatedVersion string) string {
	proto := "t" // we only support tcp (t), not quic (q) or dtls (d)

	tlsVersionMapping := map[string]string{
		"769": "10", // TLS 1.0
		"770": "11", // TLS 1.1
		"771": "12", // TLS 1.2
		"772": "13", // TLS 1.3
	}

	httpVersionMapping := map[string]string{
		"h2":  "h2", // HTTP/2
		"h3":  "h3", // HTTP/3
		"2":   "h2", // HTTP/2 alternate
		"1.1": "h1", // HTTP/1.1
		"1.0": "h1", // HTTP/1.0
		"0.9": "h1", // HTTP/0.9
	}

	// Get TLS version from negotiated version
	tlsVersion := getOrReturnOG(negotiatedVersion, tlsVersionMapping)

	// Detect SNI type (IP vs domain)
	sniMode := detectSNI(parsed)

	// Count ciphers (excluding GREASE)
	numSuites := 0
	for _, cipher := range parsed.CipherSuites {
		name := types.GetCipherSuiteName(cipher)
		if !types.IsGrease(name) {
			numSuites++
		}
	}
	// Cap at 99 per spec
	if numSuites > 99 {
		numSuites = 99
	}

	// Count extensions (excluding GREASE, SNI=0x0000, ALPN=0x0010)
	numExtensions := 0
	for _, ext := range parsed.AllExtensions {
		hexStr := fmt.Sprintf("0x%04X", ext)
		if !types.IsGrease(hexStr) && ext != 0x0000 && ext != 0x0010 {
			numExtensions++
		}
	}
	// Cap at 99 per spec
	if numExtensions > 99 {
		numExtensions = 99
	}

	// Get first ALPN
	firstALPN := "00" // default if no ALPN
	if len(parsed.SupportedProtocols) > 0 {
		alpn := parsed.SupportedProtocols[0]
		if mapped, ok := httpVersionMapping[alpn]; ok {
			firstALPN = mapped
		} else if mapped, ok := httpVersionMapping[strings.ToLower(alpn)]; ok {
			firstALPN = mapped
		} else {
			// Use first and last character per spec
			if len(alpn) >= 2 {
				firstALPN = string(alpn[0]) + string(alpn[len(alpn)-1])
			} else if len(alpn) == 1 {
				firstALPN = string(alpn[0]) + string(alpn[0])
			}
		}
	}

	return fmt.Sprintf("%s%s%s%02d%02d%s", proto, tlsVersion, sniMode, numSuites, numExtensions, firstALPN)
}

// ja4b_r_direct calculates Part B (raw) directly from ClientHello (RECOMMENDED)
func ja4b_r_direct(parsed ClientHello) string {
	// Extract cipher suites, filter GREASE, sort, and convert to hex
	ciphers := []string{}
	for _, cipher := range parsed.CipherSuites {
		name := types.GetCipherSuiteName(cipher)
		if !types.IsGrease(name) {
			hexStr := fmt.Sprintf("%04x", cipher)
			ciphers = append(ciphers, hexStr)
		}
	}
	// Sort in ascending order
	sort.Strings(ciphers)
	return strings.Join(ciphers, ",")
}

// ja4b_direct calculates Part B (hashed) directly from ClientHello (RECOMMENDED)
func ja4b_direct(parsed ClientHello) string {
	result := ja4b_r_direct(parsed)
	if result == "" {
		return "000000000000"
	}
	return utils.SHA256trunc(result)
}

// ja4c_r_direct calculates Part C (raw) directly from ClientHello (RECOMMENDED)
func ja4c_r_direct(parsed ClientHello) string {
	// Extract extensions, filter GREASE/SNI/ALPN/padding, sort
	extensions := []string{}
	for _, ext := range parsed.AllExtensions {
		hexStr := fmt.Sprintf("0x%04X", ext)
		// Skip GREASE, SNI (0x0000), ALPN (0x0010), and padding (0x0015)
		if !types.IsGrease(hexStr) && ext != 0x0000 && ext != 0x0010 && ext != 0x0015 {
			extensions = append(extensions, fmt.Sprintf("%04x", ext))
		}
	}
	// Sort extensions
	sort.Strings(extensions)

	// Get signature algorithms in ORIGINAL order (not sorted)
	sigAlgs := []string{}
	for _, alg := range parsed.SignatureAlgorithms {
		hexStr := fmt.Sprintf("%04x", alg)
		sigAlgs = append(sigAlgs, hexStr)
	}

	// Join: sorted_extensions + "_" + original_order_sig_algs
	result := strings.Join(extensions, ",")
	if len(sigAlgs) > 0 {
		result += "_" + strings.Join(sigAlgs, ",")
	}
	return result
}

// ja4c_direct calculates Part C (hashed) directly from ClientHello (RECOMMENDED)
func ja4c_direct(parsed ClientHello) string {
	result := ja4c_r_direct(parsed)
	if result == "" {
		return "000000000000"
	}
	return utils.SHA256trunc(result)
}

// CalculateJa4Direct calculates JA4 directly from ClientHello (RECOMMENDED METHOD)
func CalculateJa4Direct(parsed ClientHello, negotiatedVersion string) string {
	return ja4a_direct(parsed, negotiatedVersion) + "_" + ja4b_direct(parsed) + "_" + ja4c_direct(parsed)
}

// CalculateJa4Direct_r calculates JA4 raw mode directly from ClientHello (RECOMMENDED METHOD)
func CalculateJa4Direct_r(parsed ClientHello, negotiatedVersion string) string {
	return ja4a_direct(parsed, negotiatedVersion) + "_" + ja4b_r_direct(parsed) + "_" + ja4c_r_direct(parsed)
}

// ===== LEGACY METHODS (DEPRECATED - kept for backward compatibility) =====

// ja4a calculates Part A from TLSDetails (LEGACY - uses JA3/PeetPrint string parsing)
func ja4a(tls *types.TLSDetails) string {
	proto := "t" // we dont support quic (q), only tcp (t)

	tlsVersionMapping := map[string]string{
		"769": "10", // TLS 1.0
		"770": "11", // TLS 1.1
		"771": "12", // TLS 1.2
		"772": "13", // TLS 1.3
	}

	httpVersionMapping := map[string]string{
		"2":   "h2", // HTTP/2
		"1.1": "h1", // HTTP/1
		"1.0": "h1", // HTTP/1
		"0.9": "h1", // HTTP/1
	}

	tlsVersion := getOrReturnOG(tls.NegotiatedVesion, tlsVersionMapping)

	sniMode := "d" // IP: i, domain: d
	numSuites := len(strings.Split(strings.Split(tls.JA3, ",")[1], "-"))
	numExtensions := len(strings.Split(strings.Split(tls.JA3, ",")[2], "-"))
	firstALPN := getOrReturnOG(strings.Split(strings.Split(tls.PeetPrint, "|")[1], "-")[0], httpVersionMapping)

	// Cap counts at 99 per spec
	if numSuites > 99 {
		numSuites = 99
	}
	if numExtensions > 99 {
		numExtensions = 99
	}

	return fmt.Sprintf("%v%v%v%02d%02d%v", proto, tlsVersion, sniMode, numSuites, numExtensions, firstALPN)
}

// ja4b_r calculates Part B (raw) from TLSDetails (LEGACY)
func ja4b_r(tls *types.TLSDetails) string {
	suites := strings.Split(strings.Split(tls.JA3, ",")[1], "-")
	parsed := utils.ToHexAll(suites, false, true)
	return strings.Join(parsed, ",")
}

// ja4b calculates Part B (hashed) from TLSDetails (LEGACY)
func ja4b(tls *types.TLSDetails) string {
	result := ja4b_r(tls)
	if result == "" {
		return "000000000000"
	}
	return utils.SHA256trunc(result)
}

// ja4c_r calculates Part C (raw) from TLSDetails (LEGACY)
func ja4c_r(tls *types.TLSDetails) string {
	// Get extensions and signature algorithms
	extensions := strings.Split(strings.Split(tls.JA3, ",")[2], "-")
	sigAlgs := strings.Split(strings.Split(tls.PeetPrint, "|")[3], "-")

	// Convert extensions to hex, filter GREASE and padding, and sort
	parsedExt := []string{}
	for _, ext := range extensions {
		num, _ := strconv.Atoi(ext)
		hexStr := fmt.Sprintf("%04x", num)
		// Skip if it's a GREASE value or padding extension
		if types.IsGrease("0x"+strings.ToUpper(hexStr)) || hexStr == "0010" || hexStr == "0000" || hexStr == "0015" {
			continue
		}
		parsedExt = append(parsedExt, hexStr)
	}
	sort.Strings(parsedExt)

	// Convert signature algorithms to hex
	parsedAlg := []string{}
	for _, alg := range sigAlgs {
		if alg == "GREASE" {
			continue
		}
		num, _ := strconv.Atoi(alg)
		hexStr := fmt.Sprintf("%04x", num)
		parsedAlg = append(parsedAlg, hexStr)
	}

	// Join the results
	parsed := strings.Join(parsedExt, ",")
	if len(parsedAlg) > 0 {
		parsed += "_" + strings.Join(parsedAlg, ",")
	}
	return parsed
}

// ja4c calculates Part C (hashed) from TLSDetails (LEGACY)
func ja4c(tls *types.TLSDetails) string {
	result := ja4c_r(tls)
	if result == "" {
		return "000000000000"
	}
	return utils.SHA256trunc(result)
}

// CalculateJa4 calculates JA4 from TLSDetails (LEGACY - kept for backward compatibility)
func CalculateJa4(tls *types.TLSDetails) string {
	return ja4a(tls) + "_" + ja4b(tls) + "_" + ja4c(tls)
}

// CalculateJa4_r calculates JA4 raw mode from TLSDetails (LEGACY - kept for backward compatibility)
func CalculateJa4_r(tls *types.TLSDetails) string {
	return ja4a(tls) + "_" + ja4b_r(tls) + "_" + ja4c_r(tls)
}
