// Package opcua provides trust store management for OPC UA certificates.
package opcua

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// PKI directory structure per OPC UA Part 12:
//
//	pki/
//	├── trusted/
//	│   └── certs/         # Trusted CA and server certs
//	├── rejected/
//	│   └── certs/         # Server certs that failed validation
//	├── issuers/
//	│   └── certs/         # Intermediate CA certs
//	└── own/
//	    ├── cert.der       # Gateway's own certificate
//	    └── private/
//	        └── key.pem    # Gateway's private key

const (
	trustedCertsDir  = "trusted/certs"
	rejectedCertsDir = "rejected/certs"
	issuersCertsDir  = "issuers/certs"
	ownCertDir       = "own"
	ownPrivateDir    = "own/private"
)

// TrustStoreInfo contains metadata about a certificate in the trust store.
type TrustStoreInfo struct {
	Fingerprint  string    `json:"fingerprint"`   // SHA-256 fingerprint
	Subject      string    `json:"subject"`       // Certificate subject
	Issuer       string    `json:"issuer"`        // Certificate issuer
	SerialNumber string    `json:"serial_number"` // Certificate serial number
	NotBefore    time.Time `json:"not_before"`    // Valid from
	NotAfter     time.Time `json:"not_after"`     // Valid until
	DaysUntilExpiry int    `json:"days_until_expiry,omitempty"`
	IsExpired    bool      `json:"is_expired,omitempty"`
	IsCA         bool      `json:"is_ca"`         // Is this a CA certificate
	Filename     string    `json:"filename"`      // Filename in the store
}

// TrustStore manages OPC UA certificate trust relationships.
type TrustStore struct {
	basePath string
	logger   zerolog.Logger
	mu       sync.RWMutex
}

// NewTrustStore creates a new trust store at the specified path.
// Creates the PKI directory structure if it doesn't exist.
func NewTrustStore(basePath string, logger zerolog.Logger) (*TrustStore, error) {
	ts := &TrustStore{
		basePath: basePath,
		logger:   logger.With().Str("component", "trust-store").Logger(),
	}

	if err := ts.EnsureDirectories(); err != nil {
		return nil, fmt.Errorf("failed to create PKI directories: %w", err)
	}

	ts.logger.Info().Str("path", basePath).Msg("Trust store initialized")
	return ts, nil
}

// EnsureDirectories creates the PKI directory structure if it doesn't exist.
func (ts *TrustStore) EnsureDirectories() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	dirs := []string{
		trustedCertsDir,
		rejectedCertsDir,
		issuersCertsDir,
		ownCertDir,
		ownPrivateDir,
	}

	for _, dir := range dirs {
		fullPath := filepath.Join(ts.basePath, dir)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", fullPath, err)
		}
	}

	return nil
}

// AddTrustedCert adds a certificate to the trusted store.
func (ts *TrustStore) AddTrustedCert(cert *x509.Certificate) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	fingerprint := ts.calculateFingerprint(cert)
	filename := ts.certFilename(cert, fingerprint)
	fullPath := filepath.Join(ts.basePath, trustedCertsDir, filename)

	if err := ts.writeCert(fullPath, cert); err != nil {
		return fmt.Errorf("failed to write trusted cert: %w", err)
	}

	ts.logger.Info().
		Str("fingerprint", fingerprint).
		Str("subject", cert.Subject.String()).
		Msg("Certificate added to trusted store")

	return nil
}

// RejectCert adds a certificate to the rejected store.
func (ts *TrustStore) RejectCert(cert *x509.Certificate) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	fingerprint := ts.calculateFingerprint(cert)
	filename := ts.certFilename(cert, fingerprint)
	fullPath := filepath.Join(ts.basePath, rejectedCertsDir, filename)

	if err := ts.writeCert(fullPath, cert); err != nil {
		return fmt.Errorf("failed to write rejected cert: %w", err)
	}

	ts.logger.Warn().
		Str("fingerprint", fingerprint).
		Str("subject", cert.Subject.String()).
		Msg("Certificate added to rejected store")

	return nil
}

// IsTrusted checks if a certificate is in the trusted store by fingerprint.
func (ts *TrustStore) IsTrusted(cert *x509.Certificate) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	fingerprint := ts.calculateFingerprint(cert)
	trustedPath := filepath.Join(ts.basePath, trustedCertsDir)

	entries, err := os.ReadDir(trustedPath)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		certPath := filepath.Join(trustedPath, entry.Name())
		trustedCert, err := ts.readCert(certPath)
		if err != nil {
			continue
		}

		if ts.calculateFingerprint(trustedCert) == fingerprint {
			return true
		}
	}

	return false
}

// IsRejected checks if a certificate is in the rejected store.
func (ts *TrustStore) IsRejected(cert *x509.Certificate) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	fingerprint := ts.calculateFingerprint(cert)
	rejectedPath := filepath.Join(ts.basePath, rejectedCertsDir)

	entries, err := os.ReadDir(rejectedPath)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		certPath := filepath.Join(rejectedPath, entry.Name())
		rejectedCert, err := ts.readCert(certPath)
		if err != nil {
			continue
		}

		if ts.calculateFingerprint(rejectedCert) == fingerprint {
			return true
		}
	}

	return false
}

// ListTrustedCerts returns information about all trusted certificates.
func (ts *TrustStore) ListTrustedCerts() ([]TrustStoreInfo, error) {
	return ts.listCertsInDir(trustedCertsDir)
}

// ListRejectedCerts returns information about all rejected certificates.
func (ts *TrustStore) ListRejectedCerts() ([]TrustStoreInfo, error) {
	return ts.listCertsInDir(rejectedCertsDir)
}

// PromoteCert moves a certificate from rejected to trusted by fingerprint.
func (ts *TrustStore) PromoteCert(fingerprint string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	fingerprint = ts.normalizeFingerprint(fingerprint)
	rejectedPath := filepath.Join(ts.basePath, rejectedCertsDir)

	entries, err := os.ReadDir(rejectedPath)
	if err != nil {
		return fmt.Errorf("failed to read rejected certs: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		srcPath := filepath.Join(rejectedPath, entry.Name())
		cert, err := ts.readCert(srcPath)
		if err != nil {
			continue
		}

		if ts.calculateFingerprint(cert) == fingerprint {
			// Move to trusted
			dstPath := filepath.Join(ts.basePath, trustedCertsDir, entry.Name())
			if err := os.Rename(srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to move cert to trusted: %w", err)
			}

			ts.logger.Info().
				Str("fingerprint", fingerprint).
				Str("subject", cert.Subject.String()).
				Msg("Certificate promoted from rejected to trusted")

			return nil
		}
	}

	return fmt.Errorf("certificate with fingerprint %s not found in rejected store", fingerprint)
}

// RemoveTrustedCert removes a certificate from the trusted store by fingerprint.
func (ts *TrustStore) RemoveTrustedCert(fingerprint string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	fingerprint = ts.normalizeFingerprint(fingerprint)
	trustedPath := filepath.Join(ts.basePath, trustedCertsDir)

	entries, err := os.ReadDir(trustedPath)
	if err != nil {
		return fmt.Errorf("failed to read trusted certs: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		certPath := filepath.Join(trustedPath, entry.Name())
		cert, err := ts.readCert(certPath)
		if err != nil {
			continue
		}

		if ts.calculateFingerprint(cert) == fingerprint {
			if err := os.Remove(certPath); err != nil {
				return fmt.Errorf("failed to remove cert: %w", err)
			}

			ts.logger.Info().
				Str("fingerprint", fingerprint).
				Str("subject", cert.Subject.String()).
				Msg("Certificate removed from trusted store")

			return nil
		}
	}

	return fmt.Errorf("certificate with fingerprint %s not found in trusted store", fingerprint)
}

// GetExpiringCerts returns certificates that expire within the specified number of days.
func (ts *TrustStore) GetExpiringCerts(daysUntilExpiry int) ([]TrustStoreInfo, error) {
	allCerts, err := ts.ListTrustedCerts()
	if err != nil {
		return nil, err
	}

	var expiring []TrustStoreInfo
	threshold := time.Now().AddDate(0, 0, daysUntilExpiry)

	for _, info := range allCerts {
		if info.NotAfter.Before(threshold) {
			expiring = append(expiring, info)
		}
	}

	return expiring, nil
}

// listCertsInDir returns certificate info for all certs in a directory.
func (ts *TrustStore) listCertsInDir(relPath string) ([]TrustStoreInfo, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	fullPath := filepath.Join(ts.basePath, relPath)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var certs []TrustStoreInfo
	now := time.Now()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		certPath := filepath.Join(fullPath, entry.Name())
		cert, err := ts.readCert(certPath)
		if err != nil {
			ts.logger.Debug().Err(err).Str("file", entry.Name()).Msg("Failed to read certificate")
			continue
		}

		daysUntil := int(cert.NotAfter.Sub(now).Hours() / 24)

		info := TrustStoreInfo{
			Fingerprint:     ts.calculateFingerprint(cert),
			Subject:         cert.Subject.String(),
			Issuer:          cert.Issuer.String(),
			SerialNumber:    cert.SerialNumber.String(),
			NotBefore:       cert.NotBefore,
			NotAfter:        cert.NotAfter,
			DaysUntilExpiry: daysUntil,
			IsExpired:       now.After(cert.NotAfter),
			IsCA:            cert.IsCA,
			Filename:        entry.Name(),
		}

		certs = append(certs, info)
	}

	return certs, nil
}

// calculateFingerprint returns the SHA-256 fingerprint of a certificate.
func (ts *TrustStore) calculateFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// normalizeFingerprint ensures the fingerprint has the correct prefix.
func (ts *TrustStore) normalizeFingerprint(fingerprint string) string {
	fingerprint = strings.ToLower(fingerprint)
	if !strings.HasPrefix(fingerprint, "sha256:") {
		fingerprint = "sha256:" + fingerprint
	}
	return fingerprint
}

// certFilename generates a filename for a certificate.
func (ts *TrustStore) certFilename(cert *x509.Certificate, fingerprint string) string {
	// Use common name or serial number as base
	name := cert.Subject.CommonName
	if name == "" {
		name = cert.SerialNumber.String()
	}

	// Sanitize for filesystem
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")

	// Add fingerprint suffix for uniqueness
	fpShort := fingerprint[7:15] // First 8 chars of hex
	return fmt.Sprintf("%s_%s.der", name, fpShort)
}

// writeCert writes a certificate to a file in DER format.
func (ts *TrustStore) writeCert(path string, cert *x509.Certificate) error {
	return os.WriteFile(path, cert.Raw, 0644)
}

// readCert reads a certificate from a file (DER or PEM format).
func (ts *TrustStore) readCert(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Try PEM first
	block, _ := pem.Decode(data)
	if block != nil && block.Type == "CERTIFICATE" {
		return x509.ParseCertificate(block.Bytes)
	}

	// Try DER
	return x509.ParseCertificate(data)
}

// ValidateServerCertificate checks a server certificate against the trust store.
// Returns nil if trusted, error if rejected.
// If autoTrust is true, untrusted certs are automatically added to the trusted store.
// If autoTrust is false, untrusted certs are added to the rejected store and an error is returned.
func (ts *TrustStore) ValidateServerCertificate(certBytes []byte, autoTrust bool) error {
	if len(certBytes) == 0 {
		return nil // No cert to validate (e.g., SecurityPolicy=None)
	}

	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return fmt.Errorf("failed to parse server certificate: %w", err)
	}

	fingerprint := ts.calculateFingerprint(cert)

	// Already trusted
	if ts.IsTrusted(cert) {
		ts.logger.Debug().
			Str("fingerprint", fingerprint).
			Str("subject", cert.Subject.String()).
			Msg("Server certificate is trusted")
		return nil
	}

	// Already explicitly rejected
	if ts.IsRejected(cert) {
		return fmt.Errorf("server certificate is in rejected store (fingerprint: %s, subject: %s)",
			fingerprint, cert.Subject.String())
	}

	// Unknown certificate — auto-trust or reject
	if autoTrust {
		if err := ts.AddTrustedCert(cert); err != nil {
			ts.logger.Warn().Err(err).Str("fingerprint", fingerprint).
				Msg("Failed to auto-trust server certificate")
		} else {
			ts.logger.Info().
				Str("fingerprint", fingerprint).
				Str("subject", cert.Subject.String()).
				Msg("Server certificate auto-trusted")
		}
		return nil
	}

	// Reject unknown cert and store for manual review
	if err := ts.RejectCert(cert); err != nil {
		ts.logger.Warn().Err(err).Str("fingerprint", fingerprint).
			Msg("Failed to store rejected certificate")
	}

	return fmt.Errorf("untrusted server certificate (fingerprint: %s, subject: %s) — use /api/opcua/certificates/trust to approve",
		fingerprint, cert.Subject.String())
}

// GetFingerprint returns the SHA-256 fingerprint of a certificate.
func GetFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	return "sha256:" + hex.EncodeToString(hash[:])
}
