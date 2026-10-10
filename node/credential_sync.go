package node

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/gorilla/websocket"
	"github.com/perfect-panel/ppanel-node/api/panel"
)

func (c *Controller) startCredentialSync() {
	ctx, cancel := context.WithCancel(context.Background())
	c.syncCancel, c.syncDone = cancel, make(chan struct{})
	go func() {
		defer close(c.syncDone)
		backoff := time.Second
		for ctx.Err() == nil {
			started := time.Now()
			c.credentialSession(ctx)
			if time.Since(started) > time.Minute {
				backoff = time.Second
			}
			t := time.NewTimer(backoff + time.Duration(rand.IntN(1000))*time.Millisecond)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
			backoff = min(60*time.Second, backoff*2)
		}
	}()
}

func (c *Controller) credentialSession(ctx context.Context) {
	conn, err := c.apiClient.DialCredentialSync(ctx)
	if err != nil {
		return
	} // Old panels remain usable through periodic pulls.
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopClose()
	conn.SetReadLimit(4096)
	_ = conn.SetReadDeadline(time.Now().Add(75 * time.Second))
	// Establish the stream FIRST, then repair events missed during disconnect.
	c.userMu.Lock()
	c.apiClient.ResetUserETag()
	err = c.refreshUsers(ctx)
	c.userMu.Unlock()
	if err != nil {
		return
	}
	for ctx.Err() == nil {
		var event panel.CredentialEvent
		// Server pings also refresh the read deadline while idle.
		conn.SetPingHandler(func(data string) error {
			_ = conn.SetReadDeadline(time.Now().Add(75 * time.Second))
			return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(5*time.Second))
		})
		if conn.ReadJSON(&event) != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(75 * time.Second))
		if event.UserID <= 0 || event.Request == "" {
			return
		}
		c.userMu.Lock()
		// Repeated subscription pulls in the UUID cooldown reuse an applied
		// credential without another DB lookup or core update.
		var users []panel.UserInfo
		for _, u := range c.userList {
			if u.Id == event.UserID && u.Uuid == event.UUID {
				users = append(users, u)
				break
			}
		}
		var fetchErr error
		alreadyApplied := len(users) != 0
		if !alreadyApplied {
			users, fetchErr = c.apiClient.GetCredentialUser(ctx, event.UserID)
		}
		if fetchErr == nil && !alreadyApplied {
			next := make([]panel.UserInfo, 0, len(c.userList)+1)
			for _, u := range c.userList {
				if u.Id != event.UserID {
					next = append(next, u)
				}
			}
			next = append(next, users...)
			fetchErr = c.applyUsers(next)
		}
		if fetchErr == nil {
			for _, u := range users {
				if u.Id == event.UserID && u.Uuid == event.UUID {
					fetchErr = c.waitCredentialReady(ctx, u.Uuid)
					break
				}
			}
		}
		c.userMu.Unlock()
		if fetchErr != nil {
			return
		} // No false ACK; reconnect + polling retries.
		matched := false
		for _, u := range users {
			if u.Id == event.UserID && u.Uuid == event.UUID {
				matched = true
			}
		}
		if !matched {
			continue
		}
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if conn.WriteJSON(map[string]string{"request": event.Request, "uuid": event.UUID}) != nil {
			return
		}
	}
}

// Some protocol managers publish their authentication table asynchronously.
// A successful AddUser call alone is not an applied acknowledgement.
func (c *Controller) waitCredentialReady(parent context.Context, credential string) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	manager, err := c.server.GetUserManager(c.tag)
	if err != nil {
		return err
	}
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if manager.GetUser(ctx, c.tag+"|"+credential) != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("credential not visible in core before deadline")
		case <-tick.C:
		}
	}
}
