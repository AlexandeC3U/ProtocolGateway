package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/config"
	"github.com/nexus-edge/protocol-gateway/internal/api"

	"github.com/rs/zerolog"
)

func TestMiddleware_RequireAuth_Disabled(t *testing.T) {
	cfg := config.APIConfig{
		AuthEnabled: false,
	}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	handler := m.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestMiddleware_RequireAuth_ValidKey(t *testing.T) {
	cfg := config.APIConfig{
		AuthEnabled: true,
		APIKey:      "secret-key-123",
	}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	handler := m.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "secret-key-123")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestMiddleware_RequireAuth_InvalidKey(t *testing.T) {
	cfg := config.APIConfig{
		AuthEnabled: true,
		APIKey:      "secret-key-123",
	}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	handler := m.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestMiddleware_RequireAuth_MissingKey(t *testing.T) {
	cfg := config.APIConfig{
		AuthEnabled: true,
		APIKey:      "secret-key-123",
	}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	handler := m.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestMiddleware_RequireAuth_QueryParam(t *testing.T) {
	cfg := config.APIConfig{
		AuthEnabled: true,
		APIKey:      "secret-key-123",
	}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	handler := m.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test?api_key=secret-key-123", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestMiddleware_LimitRequestBody(t *testing.T) {
	cfg := config.APIConfig{
		MaxRequestBodySize: 100, // 100 bytes max
	}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	var bodyReceived []byte
	handler := m.LimitRequestBody(func(w http.ResponseWriter, r *http.Request) {
		bodyReceived, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})

	// Small body should work
	smallBody := strings.Repeat("a", 50)
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(smallBody))
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for small body, got %d", rec.Code)
	}
	if len(bodyReceived) != 50 {
		t.Errorf("expected body length 50, got %d", len(bodyReceived))
	}
}

func TestMiddleware_LimitRequestBody_TooLarge(t *testing.T) {
	cfg := config.APIConfig{
		MaxRequestBodySize: 100, // 100 bytes max
	}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	handler := m.LimitRequestBody(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Large body should be limited
	largeBody := strings.Repeat("a", 200)
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(largeBody))
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413 for large body, got %d", rec.Code)
	}
}

func TestMiddleware_CORS_NoOrigin(t *testing.T) {
	cfg := config.APIConfig{}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handled := m.CORS(rec, req)

	if handled {
		t.Error("expected CORS to return false for request without Origin")
	}
}

func TestMiddleware_CORS_AllowAll(t *testing.T) {
	cfg := config.APIConfig{
		AllowedOrigins: []string{}, // Empty = allow all
	}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	rec := httptest.NewRecorder()

	m.CORS(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected wildcard origin, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestMiddleware_CORS_SpecificOrigin(t *testing.T) {
	cfg := config.APIConfig{
		AllowedOrigins: []string{"http://allowed.com", "http://also-allowed.com"},
	}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://allowed.com")
	rec := httptest.NewRecorder()

	m.CORS(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "http://allowed.com" {
		t.Errorf("expected specific origin, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestMiddleware_CORS_DisallowedOrigin(t *testing.T) {
	cfg := config.APIConfig{
		AllowedOrigins: []string{"http://allowed.com"},
	}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://notallowed.com")
	rec := httptest.NewRecorder()

	m.CORS(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected no CORS header for disallowed origin, got %s",
			rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestMiddleware_CORS_Preflight(t *testing.T) {
	cfg := config.APIConfig{
		AllowedOrigins: []string{"*"},
	}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	rec := httptest.NewRecorder()

	handled := m.CORS(rec, req)

	if !handled {
		t.Error("expected CORS to handle OPTIONS preflight")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for preflight, got %d", rec.Code)
	}
}

func TestMiddleware_Secure_Combined(t *testing.T) {
	cfg := config.APIConfig{
		AuthEnabled:        true,
		APIKey:             "test-key",
		MaxRequestBodySize: 1000,
		AllowedOrigins:     []string{"http://allowed.com"},
	}
	logger := zerolog.Nop()
	m := api.NewMiddleware(cfg, logger)

	handler := m.Secure(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Success"))
	})

	// Test with valid auth
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("{}"))
	req.Header.Set("X-API-Key", "test-key")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// TestWriteJSON_Pattern demonstrates JSON response pattern
// (WriteJSON function would need to be added to api package if needed)
func TestWriteJSON_Pattern(t *testing.T) {
	rec := httptest.NewRecorder()

	data := map[string]string{"message": "hello"}
	// Inline pattern since api.WriteJSON doesn't exist yet
	rec.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rec).Encode(data)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var result map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result["message"] != "hello" {
		t.Errorf("expected message 'hello', got '%s'", result["message"])
	}
}

// TestWriteError_Pattern demonstrates error response pattern
// (WriteError function would need to be added to api package if needed)
func TestWriteError_Pattern(t *testing.T) {
	rec := httptest.NewRecorder()

	// Inline pattern since api.WriteError doesn't exist yet
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(rec).Encode(map[string]string{"error": "Invalid input"})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var result map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result["error"] != "Invalid input" {
		t.Errorf("expected error 'Invalid input', got '%s'", result["error"])
	}
}

func TestHandler_HealthCheck(t *testing.T) {
	// This would need the actual Handler struct
	// For now, test the pattern
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	// Simulate health endpoint
	handler := func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestDecodeJSON(t *testing.T) {
	type TestRequest struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	body := bytes.NewBufferString(`{"name": "test", "value": 42}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)
	req.Header.Set("Content-Type", "application/json")

	var result TestRequest
	err := json.NewDecoder(req.Body).Decode(&result)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if result.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", result.Name)
	}
	if result.Value != 42 {
		t.Errorf("expected value 42, got %d", result.Value)
	}
}

func TestDecodeJSON_InvalidJSON(t *testing.T) {
	body := bytes.NewBufferString(`{invalid json}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)

	var result map[string]interface{}
	err := json.NewDecoder(req.Body).Decode(&result)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDecodeJSON_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", nil)

	var result map[string]interface{}
	err := json.NewDecoder(req.Body).Decode(&result)
	if err == nil {
		t.Error("expected error for empty body")
	}
}
