package porthop

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type command func(input, binary string, args ...string) ([]byte, error)
type firewall interface {
	check([]Spec) error
	apply([]Spec) error
	remove() error
}

type Manager struct {
	id        string
	specs     []Spec
	backend   firewall
	active    bool
	installed bool
}

func New(identity string, specs []Spec) *Manager {
	hash := sha256.Sum256([]byte(identity))
	return &Manager{id: fmt.Sprintf("%x", hash[:8]), specs: specs}
}

func runCommand(input, binary string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s failed (Hy2 hopping requires firewall tools and root/CAP_NET_ADMIN): %w: %s", binary, err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func hasTool(name string) bool { _, err := exec.LookPath(name); return err == nil }

func ipv6Available() bool {
	c, err := net.ListenPacket("udp6", "[::]:0")
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// Prepare never modifies firewall state, so invalid configurations fail before
// the running node is stopped. Do not silently fall back on permission errors.
func (m *Manager) Prepare() error {
	if len(m.specs) == 0 {
		return nil
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("automatic Hy2 port hopping requires Linux")
	}
	if m.backend == nil {
		if hasTool("nft") {
			m.backend = newNFT(m.id, runCommand)
		} else if hasTool("iptables") {
			v6 := ipv6Available()
			if v6 && !hasTool("ip6tables") {
				return fmt.Errorf("IPv6 enabled but ip6tables missing; install nftables or iptables with IPv6 support")
			}
			m.backend = newXT(m.id, runCommand, v6)
		} else {
			return fmt.Errorf("Hy2 hop_ports configured but nftables/iptables not installed")
		}
	}
	return m.backend.check(m.specs)
}

func (m *Manager) Start() error {
	if m.installed {
		return nil
	}
	if len(m.specs) == 0 {
		return nil
	}
	if m.backend == nil {
		if err := m.Prepare(); err != nil {
			return err
		}
	}
	// A previous backend may have left rules after a crash. Remove only our
	// marked resources, including when the administrator changed firewall tools.
	if err := m.cleanupOther(); err != nil {
		return err
	}
	m.active = true // Stop also retries cleanup after a partially applied xt batch.
	if err := m.backend.apply(m.specs); err != nil {
		return errors.Join(err, m.Stop())
	}
	m.installed = true
	return nil
}

func (m *Manager) Stop() error {
	m.installed = false // A failed/partial removal must be reapplied on rollback.
	if !m.active {
		return nil
	} // Old Node.Close must not erase its replacement.
	if err := m.backend.remove(); err != nil {
		return err
	}
	m.active = false
	return nil
}

func (m *Manager) cleanupOther() error {
	if _, ok := m.backend.(*nftFirewall); ok && hasTool("iptables") {
		return newXT(m.id, runCommand, hasTool("ip6tables") && ipv6Available()).remove()
	}
	if _, ok := m.backend.(*xtFirewall); ok && hasTool("nft") {
		return newNFT(m.id, runCommand).remove()
	}
	return nil
}

// CleanupStale is used only by a fixed-port startup, not by Close on an old
// instance. It removes marked rules left by a crash with hopping now disabled.
func (m *Manager) CleanupStale() error {
	if len(m.specs) != 0 || runtime.GOOS != "linux" {
		return nil
	}
	var err error
	if hasTool("nft") {
		err = errors.Join(err, newNFT(m.id, runCommand).remove())
	}
	if hasTool("iptables") {
		err = errors.Join(err, newXT(m.id, runCommand, hasTool("ip6tables") && ipv6Available()).remove())
	}
	return err
}
