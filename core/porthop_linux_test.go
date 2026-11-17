//go:build linux

package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/perfect-panel/ppanel-node/common/porthop"
	xnet "github.com/xtls/xray-core/common/net"
	xray "github.com/xtls/xray-core/core"
)

// This is separate from the generic UDP redirect test: it exercises actual
// Hy2 TLS/authentication, Salamander, and TCP relay through the new core.
func TestHy2HoppingIntegration(t *testing.T) {
	if os.Getenv("PPNODE_HY2_HOP_TEST") != "isolated" {
		t.Skip("isolated Linux netns required")
	}
	ns := fmt.Sprintf("lc-hy2-%d", os.Getpid())
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v: %s", args, err, out)
		}
	}
	run("ip", "netns", "add", ns)
	t.Cleanup(func() { _ = exec.Command("ip", "netns", "del", ns).Run() })
	run("ip", "link", "add", "hy2-host", "type", "veth", "peer", "name", "hy2-peer")
	t.Cleanup(func() { _ = exec.Command("ip", "link", "del", "hy2-host").Run() })
	run("ip", "link", "set", "hy2-peer", "netns", ns)
	run("ip", "link", "set", "lo", "up")
	run("ip", "addr", "add", "192.0.2.1/24", "dev", "hy2-host")
	run("ip", "-6", "addr", "add", "fd00:2345::1/64", "dev", "hy2-host", "nodad")
	run("ip", "link", "set", "hy2-host", "up")
	run("ip", "-n", ns, "link", "set", "lo", "up")
	run("ip", "-n", ns, "addr", "add", "192.0.2.2/24", "dev", "hy2-peer")
	run("ip", "-n", ns, "-6", "addr", "add", "fd00:2345::2/64", "dev", "hy2-peer", "nodad")
	run("ip", "-n", ns, "link", "set", "hy2-peer", "up")
	for _, masked := range []bool{false, true} {
		t.Run(fmt.Sprintf("masked=%t", masked), func(t *testing.T) {
			var variants []string
			if masked {
				variants = []string{"salamander"}
			}
			_, mainPort, _, _, pin := protocolServer(t, "hysteria2", variants...)
			spec, err := porthop.Parse(mainPort, "31000-31002")
			if err != nil {
				t.Fatal(err)
			}
			manager := porthop.New("hy2-core-upgrade-integration", []porthop.Spec{spec})
			if err := manager.Prepare(); err != nil {
				t.Fatal(err)
			}
			if err := manager.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := manager.Stop(); err != nil {
					t.Error(err)
				}
			})
			echo, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = echo.Close() })
			go func() {
				for {
					c, err := echo.Accept()
					if err != nil {
						return
					}
					go func() { defer c.Close(); _ = c.SetDeadline(time.Now().Add(10 * time.Second)); _, _ = io.Copy(c, c) }()
				}
			}()
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			for _, host := range []string{"192.0.2.1", "fd00:2345::1"} {
				for _, port := range []int{mainPort, 31000, 31002} {
					ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
					cmd := exec.CommandContext(ctx, "ip", "netns", "exec", ns, exe, "-test.run=^TestHy2HoppingClient$", "-test.timeout=12s")
					cmd.Env = append(os.Environ(), "PPNODE_HY2_HOST="+host, "PPNODE_HY2_PORT="+strconv.Itoa(port), "PPNODE_HY2_PIN="+pin, "PPNODE_HY2_ECHO="+echo.Addr().String(), fmt.Sprintf("PPNODE_HY2_MASK=%t", masked))
					out, err := cmd.CombinedOutput()
					cancel()
					if err != nil {
						t.Fatalf("Hy2 %s:%d: %v: %s", host, port, err, out)
					}
				}
			}
		})
	}
}

func TestHy2HoppingClient(t *testing.T) {
	host := os.Getenv("PPNODE_HY2_HOST")
	if host == "" {
		t.Skip("subprocess only")
	}
	port, err := strconv.Atoi(os.Getenv("PPNODE_HY2_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	targetHost, targetPort, err := net.SplitHostPort(os.Getenv("PPNODE_HY2_ECHO"))
	if err != nil {
		t.Fatal(err)
	}
	targetNumber, err := strconv.Atoi(targetPort)
	if err != nil {
		t.Fatal(err)
	}
	var variant []string
	if os.Getenv("PPNODE_HY2_MASK") == "true" {
		variant = []string{"salamander"}
	}
	client := protocolClientAt(t, "hysteria2", host, port, os.Getenv("PPNODE_HY2_PIN"), variant...)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	conn, err := xray.Dial(ctx, client, xnet.TCPDestination(xnet.ParseAddress(targetHost), xnet.Port(targetNumber)))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	payload := []byte("Hy2 core upgrade through hopping port")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("unexpected relay data: %q", got)
	}
}
