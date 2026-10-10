package panel

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCredentialFetchUsesHeaderAndSingleUser(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/server/credential-sync" || r.URL.Query().Get("user_id") != "23" || r.Header.Get("X-Node-Secret") != "test-key" || r.URL.Query().Get("secret_key") != "" {
			t.Error("invalid scoped authentication/request")
		}
		fmt.Fprint(w, `{"users":[{"id":23,"uuid":"new","speed_limit":5,"device_limit":2}]}`)
	}))
	defer s.Close()
	c := &NodeClient{APIHost: s.URL + "/", NodeId: 8, NodeType: "tuic", SecretKey: "test-key"}
	u, err := c.GetCredentialUser(context.Background(), 23)
	if err != nil || len(u) != 1 || u[0].Uuid != "new" || u[0].DeviceLimit != 2 {
		t.Fatalf("invalid response: %v %v", u, err)
	}
}

func TestCredentialFetchRejectsWrongUserAndRedirect(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if redirect {
				http.Redirect(w, r, "https://example.invalid/", 302)
				return
			}
			fmt.Fprint(w, `{"users":[{"id":99,"uuid":"wrong"}]}`)
		}))
		c := &NodeClient{APIHost: s.URL, NodeId: 1, NodeType: "vless"}
		if _, err := c.GetCredentialUser(context.Background(), 23); err == nil {
			t.Fatal("unsafe response accepted")
		}
		s.Close()
	}
}
