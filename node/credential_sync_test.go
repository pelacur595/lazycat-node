package node

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/perfect-panel/ppanel-node/api/panel"
	"github.com/perfect-panel/ppanel-node/conf"
	"github.com/perfect-panel/ppanel-node/core"
)

func TestCredentialStreamAppliesBeforeAckWithoutPolling(t *testing.T) {
	const oldID = "00000000-0000-0000-0000-000000000001"
	const newID = "00000000-0000-0000-0000-000000000002"
	var latest atomic.Bool
	var fetches atomic.Int32
	var fullReads atomic.Int32
	resyncRead := make(chan struct{}, 1)
	ready := make(chan struct{}, 1)
	send := make(chan struct{})
	acks := make(chan string, 2)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			up := websocket.Upgrader{}
			conn, err := up.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			select {
			case ready <- struct{}{}:
			default:
			}
			<-send
			for _, id := range []string{"first", "repeat"} {
				if conn.WriteJSON(panel.CredentialEvent{Request: id, UserID: 23, UUID: newID}) != nil {
					return
				}
				var ack map[string]string
				if conn.ReadJSON(&ack) != nil {
					return
				}
				acks <- ack["uuid"]
			}
			// Keep stream open until the controller closes it.
			_, _, _ = conn.ReadMessage()
			return
		}
		credential := oldID
		if latest.Load() {
			credential = newID
		}
		if r.URL.Query().Get("user_id") != "" {
			fetches.Add(1)
		} else if fullReads.Add(1) == 2 {
			resyncRead <- struct{}{}
		}
		fmt.Fprintf(w, `{"users":[{"id":23,"uuid":%q}]}`, credential)
	}))
	defer s.Close()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	protocols := []panel.Protocol{{Type: "vless", Enable: true, Port: port, Transport: "tcp"}}
	settings := &panel.ServerConfigResponse{Data: &panel.Data{Protocols: &protocols, PullInterval: 3600, PushInterval: 3600}}
	cfg := conf.New()
	cfg.ApiConfig.ApiHost = s.URL
	cfg.ApiConfig.ServerId = 8
	x := core.New(cfg, panel.NewServerClient(&cfg.ApiConfig))
	if err := x.Start(settings); err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	n, err := New(x, cfg, settings)
	if err != nil {
		t.Fatal(err)
	}
	if err = n.Start(); err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("no stream")
	}
	select {
	case <-resyncRead:
	case <-time.After(5 * time.Second):
		t.Fatal("no reconnect snapshot")
	}
	// refreshUsers holds this lock across its HTTP request and application.
	// Wait until the A snapshot is installed before publishing B.
	c := n.controllers[0]
	c.userMu.Lock()
	c.userMu.Unlock()
	latest.Store(true)
	close(send)
	for i := 0; i < 2; i++ {
		select {
		case ack := <-acks:
			if ack != newID {
				t.Fatal("wrong ACK")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no applied ACK")
		}
	}
	c.userMu.Lock()
	applied := append([]panel.UserInfo(nil), c.userList...)
	c.userMu.Unlock()
	if len(applied) != 1 || applied[0].Uuid != newID {
		t.Fatal("ACK before apply")
	}
	manager, err := x.GetUserManager(c.tag)
	if err != nil {
		t.Fatal(err)
	}
	if manager.GetUsersCount(context.Background()) != 1 {
		t.Fatal("old credential retained or new missing")
	}
	if fetches.Load() != 1 {
		t.Fatalf("expected exactly one incremental fetch, got %d", fetches.Load())
	}
}

func TestUserComparisonIncludesDeviceLimit(t *testing.T) {
	a := panel.UserInfo{Id: 1, Uuid: "same", DeviceLimit: 1}
	b := a
	b.DeviceLimit = 2
	del, add := compareUserList([]panel.UserInfo{a}, []panel.UserInfo{b})
	if len(del) != 1 || len(add) != 1 {
		t.Fatal("device limit update ignored")
	}
}
