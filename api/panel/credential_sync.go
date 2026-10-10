package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type CredentialEvent struct {
	Request string `json:"request"`
	UserID  int    `json:"user_id"`
	UUID    string `json:"uuid"`
}

func (c *NodeClient) ResetUserETag() { c.userEtag = "" }

func (c *NodeClient) syncURL() (*url.URL, error) {
	u, err := url.Parse(c.APIHost)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported panel scheme")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/server/credential-sync"
	q := url.Values{}
	q.Set("server_id", strconv.Itoa(c.NodeId))
	q.Set("protocol", c.NodeType)
	u.RawQuery = q.Encode()
	return u, nil
}

func (c *NodeClient) DialCredentialSync(ctx context.Context) (*websocket.Conn, error) {
	u, err := c.syncURL()
	if err != nil {
		return nil, err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	h := http.Header{}
	h.Set("X-Node-Secret", c.SecretKey)
	conn, resp, err := d.DialContext(ctx, u.String(), h)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	return conn, err
}

// Fetch one authoritative user using exactly the full-list access rules.
func (c *NodeClient) GetCredentialUser(ctx context.Context, id int) ([]UserInfo, error) {
	u, err := c.syncURL()
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("user_id", strconv.Itoa(id))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Node-Secret", c.SecretKey)
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("credential fetch failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("credential fetch status %d", resp.StatusCode)
	}
	var body struct {
		Users *[]UserInfo `json:"users"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&body); err != nil || body.Users == nil {
		return nil, fmt.Errorf("invalid credential response")
	}
	for _, u := range *body.Users {
		if u.Id != id {
			return nil, fmt.Errorf("unexpected credential user")
		}
	}
	if len(*body.Users) > 1 {
		return nil, fmt.Errorf("duplicate credential user")
	}
	return *body.Users, nil
}
