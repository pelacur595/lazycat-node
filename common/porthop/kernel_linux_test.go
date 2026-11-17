//go:build linux

package porthop

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Explicit opt-in only. CI runs this test inside an isolated network namespace.
// The second namespace simulates a remote client so traffic hits PREROUTING.
func TestLinuxForwarding(t *testing.T) {
	if os.Getenv("PPNODE_FIREWALL_TEST") != "isolated" {
		t.Skip("requires isolated Linux netns and CAP_NET_ADMIN")
	}
	ns := fmt.Sprintf("lc-hop-%d", os.Getpid())
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v: %s", args, err, out)
		}
	}
	run("ip", "netns", "add", ns)
	t.Cleanup(func() { _ = exec.Command("ip", "netns", "del", ns).Run() })
	run("ip", "link", "add", "hop-host", "type", "veth", "peer", "name", "hop-peer")
	t.Cleanup(func() { _ = exec.Command("ip", "link", "del", "hop-host").Run() })
	run("ip", "link", "set", "hop-peer", "netns", ns)
	run("ip", "link", "set", "lo", "up")
	run("ip", "addr", "add", "192.0.2.1/24", "dev", "hop-host")
	run("ip", "-6", "addr", "add", "fd00:1234::1/64", "dev", "hop-host", "nodad")
	run("ip", "link", "set", "hop-host", "up")
	run("ip", "-n", ns, "link", "set", "lo", "up")
	run("ip", "-n", ns, "addr", "add", "192.0.2.2/24", "dev", "hop-peer")
	run("ip", "-n", ns, "-6", "addr", "add", "fd00:1234::2/64", "dev", "hop-peer", "nodad")
	run("ip", "-n", ns, "link", "set", "hop-peer", "up")
	// Reproduce the embedded core's udp + 0.0.0.0 listener. Linux Go opens a
	// dual-stack wildcard socket; receiving on both families is asserted below.
	for _, port := range []int{24443, 24444} {
		c, err := net.ListenPacket("udp", fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		go func() {
			buf := make([]byte, 64)
			for {
				n, a, err := c.ReadFrom(buf)
				if err != nil {
					return
				}
				_, _ = c.WriteTo(append([]byte(fmt.Sprintf("%d:", port)), buf[:n]...), a)
			}
		}()
	}
	probe := func(t *testing.T, host string, port, target int, success bool) {
		t.Helper()
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("ip", "netns", "exec", ns, exe, "-test.run=^TestPortHopUDPClient$")
		cmd.Env = append(os.Environ(), "PPNODE_UDP_PROBE="+net.JoinHostPort(host, fmt.Sprint(port)), fmt.Sprintf("PPNODE_UDP_EXPECT=%d:hello", target))
		out, err := cmd.CombinedOutput()
		if (err == nil) != success {
			t.Fatalf("probe %s:%d target %d success %t: %v %s", host, port, target, success, err, out)
		}
	}
	run("nft", "add", "table", "inet", "unrelated_test")
	for _, backend := range []struct {
		name string
		f    firewall
	}{
		{"nft", newNFT("kerneltest", runCommand)},
		{"iptables", newXT("kerneltest", runCommand, true)},
	} {
		t.Run(backend.name, func(t *testing.T) {
			f := backend.f
			t.Cleanup(func() {
				if err := f.remove(); err != nil {
					t.Error(err)
				}
			})
			s, _ := Parse(24443, "31000-31002,32000")
			if err := f.check([]Spec{s}); err != nil {
				t.Fatal(err)
			}
			// Preflight must not install any redirect.
			probe(t, "192.0.2.1", 31000, 24443, false)
			if err := f.apply([]Spec{s}); err != nil {
				t.Fatal(err)
			}
			for _, host := range []string{"192.0.2.1", "fd00:1234::1"} {
				for _, p := range []int{24443, 31000, 31001, 31002, 32000} {
					probe(t, host, p, 24443, true)
				}
			}
			// Changing both target and range must remove old mappings.
			s, _ = Parse(24444, "33000-33002")
			if err := f.apply([]Spec{s}); err != nil {
				t.Fatal(err)
			}
			for _, host := range []string{"192.0.2.1", "fd00:1234::1"} {
				probe(t, host, 33000, 24444, true)
				probe(t, host, 31000, 24443, false)
			}
			if err := f.remove(); err != nil {
				t.Fatal(err)
			}
			if err := f.remove(); err != nil {
				t.Fatal(err)
			}
			probe(t, "192.0.2.1", 33000, 24444, false)
			probe(t, "fd00:1234::1", 33000, 24444, false)
			probe(t, "192.0.2.1", 24443, 24443, true)
			probe(t, "fd00:1234::1", 24443, 24443, true)
			run("nft", "list", "table", "inet", "unrelated_test")
		})
	}
}

func TestPortHopUDPClient(t *testing.T) {
	address := os.Getenv("PPNODE_UDP_PROBE")
	if address == "" {
		t.Skip("subprocess probe only")
	}
	c, err := net.DialTimeout("udp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = c.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 128)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(buf[:n])) != os.Getenv("PPNODE_UDP_EXPECT") {
		t.Fatalf("wrong target: %s", buf[:n])
	}
}
