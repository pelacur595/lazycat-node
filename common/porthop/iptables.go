package porthop

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type xtFirewall struct {
	chain    string
	run      command
	binaries []string
	created  map[string]bool
}

func newXT(id string, run command, v6 bool) *xtFirewall {
	f := &xtFirewall{chain: "LC_HY2_" + id, run: run, binaries: []string{"iptables"}, created: map[string]bool{}}
	if v6 {
		f.binaries = append(f.binaries, "ip6tables")
	}
	return f
}
func (f *xtFirewall) call(binary string, args ...string) ([]byte, error) {
	return f.run("", binary, append([]string{"-w", "5", "-t", "nat"}, args...)...)
}
func (f *xtFirewall) sentinel() []string {
	return []string{"-A", f.chain, "-m", "comment", "--comment", owner, "-j", "RETURN"}
}
func (f *xtFirewall) jump() []string {
	return []string{"-A", "PREROUTING", "-p", "udp", "-m", "addrtype", "--dst-type", "LOCAL", "-m", "comment", "--comment", owner, "-j", f.chain}
}
func normalized(line string) string {
	fields := strings.Fields(line)
	for i := range fields {
		fields[i] = strings.Trim(fields[i], "\"")
	}
	return strings.Join(fields, " ")
}
func (f *xtFirewall) state(binary string) (bool, int, error) {
	out, err := f.call(binary, "-S") // failures cannot be treated as 'chain absent'
	if err != nil {
		return false, 0, err
	}
	exists, marked, jumps := false, false, 0
	for _, line := range strings.Split(string(out), "\n") {
		line = normalized(line)
		if line == "-N "+f.chain {
			exists = true
		}
		if line == strings.Join(f.sentinel(), " ") {
			marked = true
		}
		if line == strings.Join(f.jump(), " ") {
			jumps++
		}
	}
	if exists && !marked && !f.created[binary] {
		return false, 0, fmt.Errorf("refusing unowned %s chain %s", binary, f.chain)
	}
	return exists, jumps, nil
}
func (f *xtFirewall) check(_ []Spec) error {
	for _, b := range f.binaries {
		if _, _, err := f.state(b); err != nil {
			return err
		}
	}
	return nil
}
func (f *xtFirewall) apply(specs []Spec) error {
	if err := f.remove(); err != nil {
		return err
	}
	for _, b := range f.binaries {
		if _, err := f.call(b, "-N", f.chain); err != nil {
			return err
		}
		f.created[b] = true
		if _, err := f.call(b, f.sentinel()...); err != nil {
			return err
		}
		for _, s := range specs {
			for _, r := range s.Ranges {
				if _, err := f.call(b, "-I", f.chain, "1", "-p", "udp", "--dport", r.text(":"), "-j", "REDIRECT", "--to-ports", strconv.Itoa(s.Port)); err != nil {
					return err
				}
			}
		}
		// Attach only after the complete chain is ready.
		if _, err := f.call(b, f.jump()...); err != nil {
			return err
		}
	}
	return nil
}
func (f *xtFirewall) remove() error {
	var result error
	for _, b := range f.binaries {
		result = errors.Join(result, f.removeFamily(b))
	}
	return result
}
func (f *xtFirewall) removeFamily(binary string) error {
	exists, jumps, err := f.state(binary)
	if err != nil {
		return err
	}
	if !exists {
		delete(f.created, binary)
		return nil
	}
	args := f.jump()
	args[0] = "-D"
	for i := 0; i < jumps; i++ {
		if _, err = f.call(binary, args...); err != nil {
			return err
		}
	}
	// Flush this marked private chain ONLY, never nat/PREROUTING/ruleset.
	if _, err = f.call(binary, "-F", f.chain); err != nil {
		return err
	}
	if _, err = f.call(binary, "-X", f.chain); err != nil {
		return err
	}
	delete(f.created, binary)
	return nil
}
