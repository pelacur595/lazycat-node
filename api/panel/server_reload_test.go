package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/perfect-panel/ppanel-node/conf"
)

func TestConfigCandidateRetriedUntilInstanceReplaced(t *testing.T) {
	port := 2000
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etag := `"old"`
		if port == 2001 {
			etag = `"new"`
		}
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", etag)
		_ = json.NewEncoder(w).Encode(&ServerConfigResponse{Code: 200, Data: &Data{Protocols: &[]Protocol{{Type: "vless", Port: port}}}})
	}))
	defer server.Close()
	cfg := &conf.ServerApiConfig{ApiHost: server.URL, ServerId: 7}
	running := NewServerClient(cfg)
	if _, err := GetServerConfig(t.Context(), running); err != nil {
		t.Fatal(err)
	}
	port = 2001
	for i := 0; i < 3; i++ {
		candidate, err := CheckServerConfig(t.Context(), running)
		if err != nil || candidate == nil {
			t.Fatalf("retry %d: candidate=%v err=%v", i, candidate, err)
		}
		if running.ServerConfigEtag != `"old"` {
			t.Fatal("candidate acknowledged before apply")
		}
	}
	// reload constructs a separate client; only the successfully started instance replaces running.
	replacement := NewServerClient(cfg)
	if _, err := GetServerConfig(t.Context(), replacement); err != nil {
		t.Fatal(err)
	}
	if next, err := CheckServerConfig(t.Context(), replacement); err != nil || next != nil {
		t.Fatalf("unchanged: %v %v", next, err)
	}
}

func TestConfigCandidateRetriedWithoutETag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"data":{"protocols":[{"type":"vless","port":2001}]}}`))
	}))
	defer server.Close()
	client := NewServerClient(&conf.ServerApiConfig{ApiHost: server.URL, ServerId: 7})
	for i := 0; i < 3; i++ {
		if candidate, err := CheckServerConfig(t.Context(), client); err != nil || candidate == nil {
			t.Fatalf("retry: %v %v", candidate, err)
		}
	}
}
