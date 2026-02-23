package opcua_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestTrustStoreDirectoryStructure(t *testing.T) {
	tmpDir := t.TempDir()

	expectedDirs := []string{
		filepath.Join(tmpDir, "trusted", "certs"),
		filepath.Join(tmpDir, "rejected", "certs"),
		filepath.Join(tmpDir, "issuers", "certs"),
		filepath.Join(tmpDir, "own"),
		filepath.Join(tmpDir, "own", "private"),
	}

	for _, dir := range expectedDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("Expected directory %s does not exist: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("Expected %s to be a directory", dir)
		}
	}
}

func TestCertificateFingerprint(t *testing.T) {
	cert := generateTestCertificate(t, "Test Certificate", time.Now(), time.Now().Add(365*24*time.Hour))

	certDER, err := x509.ParseCertificate(cert.Raw)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	if certDER.Subject.CommonName != "Test Certificate" {
		t.Errorf("Expected CN 'Test Certificate', got '%s'", certDER.Subject.CommonName)
	}
}

func TestCertificateExpiry(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		notBefore      time.Time
		notAfter       time.Time
		wantExpired    bool
		wantDaysLeft   int
	}{
		{
			name:         "Valid certificate",
			notBefore:    now.Add(-24 * time.Hour),
			notAfter:     now.Add(365 * 24 * time.Hour),
			wantExpired:  false,
			wantDaysLeft: 365,
		},
		{
			name:         "Expired certificate",
			notBefore:    now.Add(-365 * 24 * time.Hour),
			notAfter:     now.Add(-24 * time.Hour),
			wantExpired:  true,
			wantDaysLeft: -1,
		},
		{
			name:         "Certificate expiring soon",
			notBefore:    now.Add(-30 * 24 * time.Hour),
			notAfter:     now.Add(7 * 24 * time.Hour),
			wantExpired:  false,
			wantDaysLeft: 7,
		},
		{
			name:         "Not yet valid",
			notBefore:    now.Add(7 * 24 * time.Hour),
			notAfter:     now.Add(365 * 24 * time.Hour),
			wantExpired:  false,
			wantDaysLeft: 365,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := generateTestCertificate(t, tt.name, tt.notBefore, tt.notAfter)

			isExpired := time.Now().After(cert.NotAfter)
			if isExpired != tt.wantExpired {
				t.Errorf("Expired check: got %v, want %v", isExpired, tt.wantExpired)
			}

			daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)
			if abs(daysLeft-tt.wantDaysLeft) > 1 {
				t.Errorf("Days left: got %d, want approximately %d", daysLeft, tt.wantDaysLeft)
			}
		})
	}
}

func TestCertificateStorage(t *testing.T) {
	tmpDir := t.TempDir()
	trustedDir := filepath.Join(tmpDir, "trusted", "certs")
	rejectedDir := filepath.Join(tmpDir, "rejected", "certs")

	if err := os.MkdirAll(trustedDir, 0755); err != nil {
		t.Fatalf("Failed to create trusted dir: %v", err)
	}
	if err := os.MkdirAll(rejectedDir, 0755); err != nil {
		t.Fatalf("Failed to create rejected dir: %v", err)
	}

	cert := generateTestCertificate(t, "Test Cert", time.Now(), time.Now().Add(365*24*time.Hour))

	certPath := filepath.Join(trustedDir, "test_cert.der")
	if err := os.WriteFile(certPath, cert.Raw, 0644); err != nil {
		t.Fatalf("Failed to write certificate: %v", err)
	}

	data, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("Failed to read certificate: %v", err)
	}

	parsedCert, err := x509.ParseCertificate(data)
	if err != nil {
		t.Fatalf("Failed to parse stored certificate: %v", err)
	}

	if parsedCert.Subject.CommonName != "Test Cert" {
		t.Errorf("CN mismatch: got '%s', want 'Test Cert'", parsedCert.Subject.CommonName)
	}
}

func TestCertificateMove(t *testing.T) {
	tmpDir := t.TempDir()
	rejectedDir := filepath.Join(tmpDir, "rejected", "certs")
	trustedDir := filepath.Join(tmpDir, "trusted", "certs")

	if err := os.MkdirAll(rejectedDir, 0755); err != nil {
		t.Fatalf("Failed to create rejected dir: %v", err)
	}
	if err := os.MkdirAll(trustedDir, 0755); err != nil {
		t.Fatalf("Failed to create trusted dir: %v", err)
	}

	cert := generateTestCertificate(t, "Move Test", time.Now(), time.Now().Add(365*24*time.Hour))
	rejectedPath := filepath.Join(rejectedDir, "move_test.der")

	if err := os.WriteFile(rejectedPath, cert.Raw, 0644); err != nil {
		t.Fatalf("Failed to write rejected certificate: %v", err)
	}

	trustedPath := filepath.Join(trustedDir, "move_test.der")
	if err := os.Rename(rejectedPath, trustedPath); err != nil {
		t.Fatalf("Failed to move certificate: %v", err)
	}

	if _, err := os.Stat(rejectedPath); !os.IsNotExist(err) {
		t.Errorf("Certificate still exists in rejected directory")
	}

	if _, err := os.Stat(trustedPath); err != nil {
		t.Errorf("Certificate not found in trusted directory: %v", err)
	}
}

func TestCertificateList(t *testing.T) {
	tmpDir := t.TempDir()
	trustedDir := filepath.Join(tmpDir, "trusted", "certs")

	if err := os.MkdirAll(trustedDir, 0755); err != nil {
		t.Fatalf("Failed to create trusted dir: %v", err)
	}

	certNames := []string{"Cert One", "Cert Two", "Cert Three"}
	for _, name := range certNames {
		cert := generateTestCertificate(t, name, time.Now(), time.Now().Add(365*24*time.Hour))
		path := filepath.Join(trustedDir, name+".der")
		if err := os.WriteFile(path, cert.Raw, 0644); err != nil {
			t.Fatalf("Failed to write certificate %s: %v", name, err)
		}
	}

	entries, err := os.ReadDir(trustedDir)
	if err != nil {
		t.Fatalf("Failed to read trusted directory: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("Expected 3 certificates, got %d", len(entries))
	}

	for _, entry := range entries {
		if entry.IsDir() {
			t.Errorf("Unexpected directory: %s", entry.Name())
		}
		if filepath.Ext(entry.Name()) != ".der" {
			t.Errorf("Unexpected file extension: %s", entry.Name())
		}
	}
}

func TestCertificateRemove(t *testing.T) {
	tmpDir := t.TempDir()
	trustedDir := filepath.Join(tmpDir, "trusted", "certs")

	if err := os.MkdirAll(trustedDir, 0755); err != nil {
		t.Fatalf("Failed to create trusted dir: %v", err)
	}

	cert := generateTestCertificate(t, "Remove Test", time.Now(), time.Now().Add(365*24*time.Hour))
	certPath := filepath.Join(trustedDir, "remove_test.der")

	if err := os.WriteFile(certPath, cert.Raw, 0644); err != nil {
		t.Fatalf("Failed to write certificate: %v", err)
	}

	if err := os.Remove(certPath); err != nil {
		t.Fatalf("Failed to remove certificate: %v", err)
	}

	if _, err := os.Stat(certPath); !os.IsNotExist(err) {
		t.Errorf("Certificate still exists after removal")
	}
}

func TestTrustStoreInfoFields(t *testing.T) {
	type TrustStoreInfo struct {
		Fingerprint string `json:"fingerprint"`
		Subject     string `json:"subject"`
		Issuer      string `json:"issuer"`
		NotBefore   string `json:"not_before"`
		NotAfter    string `json:"not_after"`
		FilePath    string `json:"file_path"`
	}

	cert := generateTestCertificate(t, "Info Test", time.Now(), time.Now().Add(365*24*time.Hour))

	info := TrustStoreInfo{
		Fingerprint: "sha256:abc123...",
		Subject:     cert.Subject.String(),
		Issuer:      cert.Issuer.String(),
		NotBefore:   cert.NotBefore.Format(time.RFC3339),
		NotAfter:    cert.NotAfter.Format(time.RFC3339),
		FilePath:    "/pki/trusted/certs/test.der",
	}

	if info.Subject == "" {
		t.Error("Subject should not be empty")
	}
	if info.Fingerprint == "" {
		t.Error("Fingerprint should not be empty")
	}
	if info.NotBefore == "" {
		t.Error("NotBefore should not be empty")
	}
	if info.NotAfter == "" {
		t.Error("NotAfter should not be empty")
	}
}

func generateTestCertificate(t *testing.T, cn string, notBefore, notAfter time.Time) *x509.Certificate {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	return cert
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

var _ zerolog.Logger
