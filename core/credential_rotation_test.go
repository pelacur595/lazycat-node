package core

import (
	"context"
	"testing"

	"github.com/perfect-panel/ppanel-node/api/panel"
)

func TestCredentialReplacementInEveryProtocolManager(t *testing.T) {
	for _, name := range []string{"vless", "vmess", "shadowsocks", "trojan", "anytls", "hysteria2", "tuic"} {
		t.Run(name, func(t *testing.T) {
			server, _, info, old, _ := protocolServer(t, name)
			if err := server.DelUsers([]panel.UserInfo{old}, "protocol-test", info); err != nil {
				t.Fatal(err)
			}
			next := old
			next.Uuid = "00000000-0000-0000-0000-000000000002"
			if _, err := server.AddUsers(&AddUsersParams{Tag: "protocol-test", Users: []panel.UserInfo{next}, NodeInfo: info}); err != nil {
				t.Fatal(err)
			}
			manager, err := server.GetUserManager("protocol-test")
			if err != nil {
				t.Fatal(err)
			}
			waitProtocolUser(t, func() bool {
				return manager.GetUser(context.Background(), "protocol-test|"+old.Uuid) == nil && manager.GetUser(context.Background(), "protocol-test|"+next.Uuid) != nil
			})
		})
	}
}
