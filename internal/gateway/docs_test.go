package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIDocumentationEndpoints(t *testing.T) {
	app := NewApp(testSerialSettings, "127.0.0.1", 7000, nil)
	mux := NewMux(app, http.Dir("../../internal/webui/public"))
	tests := []struct {
		path        string
		contentType string
		contains    []string
	}{
		{"/api/docs", "text/html", []string{"Serial Gateway API", "http://gateway.test:8080", "/api/v1/openapi.json"}},
		{"/api/v1/guide", "text/markdown", []string{"Serial Gateway AI Guide", "next_after", "command\\r"}},
		{"/api/v1/openapi.json", "application/vnd.oai.openapi+json", []string{"\"openapi\": \"3.1.0\"", "/api/v1/read"}},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, "http://gateway.test:8080"+test.path, nil)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Errorf("GET %s status = %d", test.path, response.Code)
			continue
		}
		if !strings.HasPrefix(response.Header().Get("Content-Type"), test.contentType) {
			t.Errorf("GET %s content type = %q", test.path, response.Header().Get("Content-Type"))
		}
		for _, expected := range test.contains {
			if !strings.Contains(response.Body.String(), expected) {
				t.Errorf("GET %s missing %q", test.path, expected)
			}
		}
	}
}

func TestOpenAPIIsValidJSON(t *testing.T) {
	var document struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal([]byte(openAPISpec), &document); err != nil {
		t.Fatalf("decode OpenAPI: %v", err)
	}
	if document.OpenAPI != "3.1.0" || document.Paths["/api/v1/write"] == nil || document.Paths["/api/v1/config"] == nil {
		t.Fatalf("incomplete OpenAPI document: %#v", document)
	}
}
