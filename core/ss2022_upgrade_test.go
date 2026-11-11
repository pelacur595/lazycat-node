package core

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	xnet "github.com/xtls/xray-core/common/net"
	xray "github.com/xtls/xray-core/core"
)

// The new core rewrote SS2022's UDP implementation. Check the actual wire
// path and per-user accounting without importing upstream's device admission.
func TestSS2022UpgradeUDP(t *testing.T) {
	for _, cipher := range []string{"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm"} {
		t.Run(cipher, func(t *testing.T) {
			server, port, _, user, pin := protocolServer(t, "shadowsocks", cipher)
			echo, err := net.ListenPacket("udp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer echo.Close()
			go func() {
				buf := make([]byte, 1500)
				n, addr, err := echo.ReadFrom(buf)
				if err == nil {
					_, _ = echo.WriteTo(buf[:n], addr)
				}
			}()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			target := xnet.UDPDestination(xnet.ParseAddress("127.0.0.1"), xnet.Port(echo.LocalAddr().(*net.UDPAddr).Port))
			conn, err := xray.Dial(ctx, protocolClient(t, "shadowsocks", port, pin, cipher), target)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			payload := []byte("SS2022 UDP upgrade regression")
			if _, err := conn.Write(payload); err != nil {
				t.Fatal(err)
			}
			got := make([]byte, len(payload))
			if _, err := io.ReadFull(conn, got); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("UDP payload = %q", got)
			}
			traffic, err := server.GetUserTrafficSlice("protocol-test", 0)
			if err != nil || len(traffic) != 1 || traffic[0].UID != user.Id || traffic[0].Upload < int64(len(payload)) || traffic[0].Download < int64(len(payload)) {
				t.Fatalf("UDP accounting = %+v, %v", traffic, err)
			}
		})
	}
}
