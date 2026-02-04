// Package opcua provides security utilities for OPC UA connections.
package opcua

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/rs/zerolog"
)

// SecurityConfig holds validated security configuration.
type SecurityConfig struct {
	// Certificates
	ClientCert []byte
	ClientKey  *rsa.PrivateKey
	ServerCert []byte

	// Parsed policy/mode
	SecurityPolicy string
	SecurityMode   ua.MessageSecurityMode

	// Authentication
	AuthType ua.UserTokenType
	Username string
	Password string

	// Application identity
	ApplicationName string
	ApplicationURI  string

	// Flags
	InsecureSkipVerify bool
}

// ValidateSecurityConfig validates and loads security configuration.
// Returns an error if certificate files are specified but invalid.
func ValidateSecurityConfig(config ClientConfig, logger zerolog.Logger) (*SecurityConfig, error) {
	sc := &SecurityConfig{
		SecurityPolicy:     config.SecurityPolicy,
		ApplicationName:    config.ApplicationName,
		ApplicationURI:     config.ApplicationURI,
		InsecureSkipVerify: config.InsecureSkipVerify,
		Username:           config.Username,
		Password:           config.Password,
	}

	// Set defaults
	if sc.ApplicationName == "" {
		sc.ApplicationName = "NexusEdge Protocol Gateway"
	}
	if sc.ApplicationURI == "" {
		sc.ApplicationURI = "urn:nexusedge:protocol-gateway"
	}

	// Parse security mode
	sc.SecurityMode = parseSecurityMode(config.SecurityMode)

	// Determine auth type
	switch config.AuthMode {
	case "UserName":
		sc.AuthType = ua.UserTokenTypeUserName
	case "Certificate":
		sc.AuthType = ua.UserTokenTypeCertificate
	default:
		sc.AuthType = ua.UserTokenTypeAnonymous
	}

	// Load client certificate if specified
	if config.CertificateFile != "" {
		cert, err := loadCertificate(config.CertificateFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		sc.ClientCert = cert
		logger.Debug().Str("file", config.CertificateFile).Msg("Loaded client certificate")
	}

	// Load private key if specified
	if config.PrivateKeyFile != "" {
		key, err := loadPrivateKey(config.PrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load private key: %w", err)
		}
		sc.ClientKey = key
		logger.Debug().Str("file", config.PrivateKeyFile).Msg("Loaded private key")
	}

	// Load server certificate if specified (for trust verification)
	if config.ServerCertificateFile != "" {
		cert, err := loadCertificate(config.ServerCertificateFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load server certificate: %w", err)
		}
		sc.ServerCert = cert
		logger.Debug().Str("file", config.ServerCertificateFile).Msg("Loaded server certificate for trust")
	}

	// Validate certificate/key pair if both specified
	if (sc.ClientCert != nil) != (sc.ClientKey != nil) {
		return nil, fmt.Errorf("both certificate and private key must be specified together")
	}

	// Validate security requirements
	if needsCertificates(sc.SecurityPolicy, sc.SecurityMode) {
		if sc.ClientCert == nil || sc.ClientKey == nil {
			return nil, fmt.Errorf("security policy %s with mode %s requires client certificate and key",
				sc.SecurityPolicy, config.SecurityMode)
		}
	}

	return sc, nil
}

// BuildClientOptions builds gopcua client options from security config.
func (sc *SecurityConfig) BuildClientOptions() []opcua.Option {
	opts := []opcua.Option{
		opcua.ApplicationName(sc.ApplicationName),
		opcua.ApplicationURI(sc.ApplicationURI),
	}

	// Configure security policy and mode
	secPolicyURI := getSecurityPolicyURI(sc.SecurityPolicy)
	if secPolicyURI != ua.SecurityPolicyURINone {
		opts = append(opts, opcua.SecurityPolicy(secPolicyURI))
		opts = append(opts, opcua.SecurityMode(sc.SecurityMode))
	}

	// Configure client certificate
	if sc.ClientCert != nil {
		opts = append(opts, opcua.Certificate(sc.ClientCert))
	}
	if sc.ClientKey != nil {
		opts = append(opts, opcua.PrivateKey(sc.ClientKey))
	}

	// Configure server certificate trust
	if sc.ServerCert != nil {
		opts = append(opts, opcua.RemoteCertificate(sc.ServerCert))
	}

	// Configure authentication
	switch sc.AuthType {
	case ua.UserTokenTypeUserName:
		opts = append(opts, opcua.AuthUsername(sc.Username, sc.Password))
	case ua.UserTokenTypeCertificate:
		if sc.ClientCert != nil {
			opts = append(opts, opcua.AuthCertificate(sc.ClientCert))
			if sc.ClientKey != nil {
				opts = append(opts, opcua.AuthPrivateKey(sc.ClientKey))
			}
		}
	default:
		opts = append(opts, opcua.AuthAnonymous())
	}

	return opts
}

// DiscoverAndSelectEndpoint queries available endpoints and selects the best match.
func DiscoverAndSelectEndpoint(ctx context.Context, endpointURL string, config ClientConfig, logger zerolog.Logger) (*ua.EndpointDescription, error) {
	logger.Debug().Str("endpoint", endpointURL).Msg("Discovering OPC UA endpoints")

	endpoints, err := opcua.GetEndpoints(ctx, endpointURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints: %w", err)
	}

	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints returned from server")
	}

	logger.Debug().Int("count", len(endpoints)).Msg("Found endpoints")

	// Determine desired auth type
	desiredAuthType := ua.UserTokenTypeAnonymous
	switch config.AuthMode {
	case "UserName":
		desiredAuthType = ua.UserTokenTypeUserName
	case "Certificate":
		desiredAuthType = ua.UserTokenTypeCertificate
	}

	// Try to find endpoint matching requested security
	desiredPolicy := getSecurityPolicyURI(config.SecurityPolicy)
	desiredMode := parseSecurityMode(config.SecurityMode)

	var bestEndpoint *ua.EndpointDescription
	var bestScore int

	for _, ep := range endpoints {
		// Check if endpoint supports desired auth type
		if !endpointSupportsAuth(ep, desiredAuthType) {
			continue
		}

		score := scoreEndpoint(ep, desiredPolicy, desiredMode)

		logger.Debug().
			Str("url", ep.EndpointURL).
			Str("policy", ep.SecurityPolicyURI).
			Int32("mode", int32(ep.SecurityMode)).
			Int("score", score).
			Msg("Evaluating endpoint")

		if score > bestScore {
			bestScore = score
			bestEndpoint = ep
		}
	}

	if bestEndpoint == nil {
		// Fall back to any endpoint with matching auth
		for _, ep := range endpoints {
			if endpointSupportsAuth(ep, desiredAuthType) {
				bestEndpoint = ep
				break
			}
		}
	}

	if bestEndpoint == nil {
		return nil, fmt.Errorf("no suitable endpoint found for auth mode %s", config.AuthMode)
	}

	logger.Info().
		Str("url", bestEndpoint.EndpointURL).
		Str("policy", bestEndpoint.SecurityPolicyURI).
		Int32("mode", int32(bestEndpoint.SecurityMode)).
		Msg("Selected endpoint")

	return bestEndpoint, nil
}

// BuildOptionsFromEndpoint creates client options using discovered endpoint.
func BuildOptionsFromEndpoint(ep *ua.EndpointDescription, sc *SecurityConfig) []opcua.Option {
	opts := []opcua.Option{
		opcua.ApplicationName(sc.ApplicationName),
		opcua.ApplicationURI(sc.ApplicationURI),
	}

	// Use endpoint's security settings
	opts = append(opts, opcua.SecurityFromEndpoint(ep, sc.AuthType))

	// Add client credentials
	if sc.ClientCert != nil {
		opts = append(opts, opcua.Certificate(sc.ClientCert))
	}
	if sc.ClientKey != nil {
		opts = append(opts, opcua.PrivateKey(sc.ClientKey))
	}

	// Trust server certificate from endpoint
	if len(ep.ServerCertificate) > 0 && sc.InsecureSkipVerify {
		opts = append(opts, opcua.RemoteCertificate(ep.ServerCertificate))
	} else if sc.ServerCert != nil {
		opts = append(opts, opcua.RemoteCertificate(sc.ServerCert))
	}

	// Add authentication
	switch sc.AuthType {
	case ua.UserTokenTypeUserName:
		opts = append(opts, opcua.AuthUsername(sc.Username, sc.Password))
	case ua.UserTokenTypeCertificate:
		if sc.ClientCert != nil {
			opts = append(opts, opcua.AuthCertificate(sc.ClientCert))
			if sc.ClientKey != nil {
				opts = append(opts, opcua.AuthPrivateKey(sc.ClientKey))
			}
		}
	default:
		opts = append(opts, opcua.AuthAnonymous())
	}

	return opts
}

// =============================================================================
// Helper Functions
// =============================================================================

// loadCertificate loads a certificate from file (PEM or DER format).
func loadCertificate(filename string) ([]byte, error) {
	filename = filepath.Clean(filename)

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}

	// Try PEM first
	block, _ := pem.Decode(data)
	if block != nil && block.Type == "CERTIFICATE" {
		return block.Bytes, nil
	}

	// Try DER (raw certificate)
	_, err = x509.ParseCertificate(data)
	if err == nil {
		return data, nil
	}

	return nil, fmt.Errorf("file is not a valid PEM or DER certificate")
}

// loadPrivateKey loads a private key from file (PEM format).
func loadPrivateKey(filename string) (*rsa.PrivateKey, error) {
	filename = filepath.Clean(filename)

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Try PKCS#1 format
	if strings.Contains(block.Type, "RSA PRIVATE KEY") {
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err == nil {
			return key, nil
		}
	}

	// Try PKCS#8 format
	keyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err == nil {
		if rsaKey, ok := keyInterface.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, fmt.Errorf("private key is not RSA")
	}

	return nil, fmt.Errorf("failed to parse private key: unsupported format")
}

// getSecurityPolicyURI converts policy name to URI.
func getSecurityPolicyURI(policy string) string {
	switch policy {
	case "Basic128Rsa15":
		return ua.SecurityPolicyURIBasic128Rsa15
	case "Basic256":
		return ua.SecurityPolicyURIBasic256
	case "Basic256Sha256":
		return ua.SecurityPolicyURIBasic256Sha256
	case "Aes128_Sha256_RsaOaep":
		return ua.SecurityPolicyURIAes128Sha256RsaOaep
	case "Aes256_Sha256_RsaPss":
		return ua.SecurityPolicyURIAes256Sha256RsaPss
	default:
		return ua.SecurityPolicyURINone
	}
}

// parseSecurityMode converts mode string to enum.
func parseSecurityMode(mode string) ua.MessageSecurityMode {
	switch mode {
	case "Sign":
		return ua.MessageSecurityModeSign
	case "SignAndEncrypt":
		return ua.MessageSecurityModeSignAndEncrypt
	default:
		return ua.MessageSecurityModeNone
	}
}

// needsCertificates returns true if the security config requires certificates.
func needsCertificates(policy string, mode ua.MessageSecurityMode) bool {
	if mode == ua.MessageSecurityModeNone {
		return false
	}
	return policy != "" && policy != "None"
}

// endpointSupportsAuth checks if endpoint supports the given auth type.
func endpointSupportsAuth(ep *ua.EndpointDescription, authType ua.UserTokenType) bool {
	for _, token := range ep.UserIdentityTokens {
		if token.TokenType == authType {
			return true
		}
	}
	return false
}

// scoreEndpoint scores an endpoint based on how well it matches desired settings.
// Higher score = better match.
func scoreEndpoint(ep *ua.EndpointDescription, desiredPolicy string, desiredMode ua.MessageSecurityMode) int {
	score := 0

	// Exact policy match
	if ep.SecurityPolicyURI == desiredPolicy {
		score += 100
	} else if desiredPolicy == ua.SecurityPolicyURINone && ep.SecurityPolicyURI == ua.SecurityPolicyURINone {
		score += 100
	}

	// Exact mode match
	if ep.SecurityMode == desiredMode {
		score += 50
	}

	// Prefer higher security when no preference
	switch ep.SecurityPolicyURI {
	case ua.SecurityPolicyURIAes256Sha256RsaPss:
		score += 5
	case ua.SecurityPolicyURIAes128Sha256RsaOaep:
		score += 4
	case ua.SecurityPolicyURIBasic256Sha256:
		score += 3
	case ua.SecurityPolicyURIBasic256:
		score += 2
	case ua.SecurityPolicyURIBasic128Rsa15:
		score += 1
	}

	// Prefer SignAndEncrypt over Sign over None
	switch ep.SecurityMode {
	case ua.MessageSecurityModeSignAndEncrypt:
		score += 3
	case ua.MessageSecurityModeSign:
		score += 2
	}

	return score
}

// CertificateInfo contains parsed certificate information.
type CertificateInfo struct {
	Subject      string
	Issuer       string
	SerialNumber string
	NotBefore    string
	NotAfter     string
	IsCA         bool
	KeyUsage     string
}

// GetCertificateInfo parses and returns information about a certificate.
func GetCertificateInfo(certBytes []byte) (*CertificateInfo, error) {
	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, err
	}

	return &CertificateInfo{
		Subject:      cert.Subject.String(),
		Issuer:       cert.Issuer.String(),
		SerialNumber: cert.SerialNumber.String(),
		NotBefore:    cert.NotBefore.String(),
		NotAfter:     cert.NotAfter.String(),
		IsCA:         cert.IsCA,
		KeyUsage:     fmt.Sprintf("%d", cert.KeyUsage),
	}, nil
}
