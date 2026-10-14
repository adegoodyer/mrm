package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adegoodyer/mrm/internal/config"
)

func TestSend(t *testing.T) {
	var gotifyKey, gotifyQuery, ntfyAuth, ntfyTitle, ntfyBody string
	var gotifyBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/message":
			gotifyKey, gotifyQuery = r.Header.Get("X-Gotify-Key"), r.URL.RawQuery
			json.Unmarshal(body, &gotifyBody)
		case "/topic":
			ntfyAuth, ntfyTitle, ntfyBody = r.Header.Get("Authorization"), r.Header.Get("Title"), string(body)
		}
	}))
	defer srv.Close()

	t.Setenv("MRM_TEST_NTFY", "ntfy-secret")
	n := config.Notify{
		Gotify: &config.Endpoint{URL: srv.URL, TokenCmd: "echo gotify-secret"},
		Ntfy:   &config.Endpoint{URL: srv.URL + "/topic", TokenEnv: "MRM_TEST_NTFY"},
	}
	if err := Send(context.Background(), n, Message{Title: "mrm sync: work", Body: "1 skipped", Priority: 5}); err != nil {
		t.Fatal(err)
	}
	if gotifyKey != "gotify-secret" || gotifyQuery != "" {
		t.Fatalf("gotify token must go in a header, not the URL: key %q query %q", gotifyKey, gotifyQuery)
	}
	if gotifyBody["title"] != "mrm sync: work" || gotifyBody["message"] != "1 skipped" {
		t.Fatalf("gotify body %v", gotifyBody)
	}
	if ntfyAuth != "Bearer ntfy-secret" || ntfyTitle != "mrm sync: work" || ntfyBody != "1 skipped" {
		t.Fatalf("ntfy: auth %q title %q body %q", ntfyAuth, ntfyTitle, ntfyBody)
	}
}

func TestTokenErrorsDontLeakOutput(t *testing.T) {
	_, err := Token(context.Background(), &config.Endpoint{TokenCmd: "echo leaked-secret; exit 3"})
	if err == nil || strings.Contains(err.Error(), "leaked-secret") {
		t.Fatalf("err %v", err)
	}
	if _, err := Token(context.Background(), &config.Endpoint{TokenEnv: "MRM_TEST_UNSET_VAR"}); err == nil {
		t.Fatal("empty env var: no error")
	}
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	err := Send(context.Background(), config.Notify{Gotify: &config.Endpoint{URL: srv.URL}}, Message{})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err %v", err)
	}
}
