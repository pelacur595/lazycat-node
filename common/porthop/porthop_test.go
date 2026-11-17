package porthop

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	s, err := Parse(443, " 3005,3000-3003,3002-3004,4000 ")
	if err != nil || !reflect.DeepEqual(s.Ranges, []Range{{3000, 3005}, {4000, 4000}}) {
		t.Fatalf("unexpected normalized ranges: %+v, %v", s, err)
	}
	for _, raw := range []string{"0", "65536", "4000-3000", "3000,", "1:2", "1;reboot", "-1", "1--2", "3000—4000", "1 2"} {
		if _, err := Parse(443, raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	if _, err := Parse(0, "3000"); err == nil {
		t.Fatal("accepted invalid target")
	}
	if s, err := Parse(443, " "); err != nil || len(s.Ranges) != 0 {
		t.Fatal("empty means fixed port")
	}
}

type fakeFirewall struct {
	applies, removes    int
	applyErr, removeErr error
}

func (f *fakeFirewall) check([]Spec) error { return nil }
func (f *fakeFirewall) apply([]Spec) error { f.applies++; return f.applyErr }
func (f *fakeFirewall) remove() error      { f.removes++; return f.removeErr }

func TestLifecycle(t *testing.T) {
	f := &fakeFirewall{}
	m := New("test", []Spec{{Port: 443, Ranges: []Range{{3000, 4000}}}})
	m.backend = f
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if f.applies != 1 {
		t.Fatal("duplicate apply")
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if f.removes != 1 {
		t.Fatal("old close deleted replacement rules")
	}
	f.applyErr = errors.New("partial failure")
	if m.Start() == nil || m.active || f.removes != 2 {
		t.Fatal("partial apply was not cleaned")
	}
	f.applyErr = nil
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	f.removeErr = errors.New("busy")
	if m.Stop() == nil || !m.active {
		t.Fatal("must retain cleanup retry")
	}
	f.removeErr = nil
	before := f.applies
	if err := m.Start(); err != nil || f.applies != before+1 {
		t.Fatal("rollback must reinstall after partial removal")
	}
	if err := m.Stop(); err != nil || m.active {
		t.Fatal("cleanup retry failed")
	}
}

func TestNFTPreflightAndOwnership(t *testing.T) {
	f := newNFT("test", nil)
	var inputs []string
	f.run = func(input, binary string, args ...string) ([]byte, error) {
		if strings.Join(args, " ") == "-j list tables" {
			return []byte(`{"nftables":[]}`), nil
		}
		if len(args) == 0 || args[0] != "--check" {
			t.Fatalf("preflight mutated state: %v", args)
		}
		inputs = append(inputs, input)
		return nil, nil
	}
	s, _ := Parse(443, "3000-4000,5000")
	if err := f.check([]Spec{s}); err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 || !strings.Contains(inputs[0], "fib daddr type local udp dport { 3000-4000, 5000 }") {
		t.Fatal(inputs)
	}
	f.run = func(input, binary string, args ...string) ([]byte, error) {
		if input != "" {
			t.Fatal("must not modify unowned table")
		}
		return []byte(`{"nftables":[{"table":{"family":"inet","name":"lc_hy2_test"}}]}`), nil
	}
	if f.remove() == nil {
		t.Fatal("removed unowned table")
	}
	f.run = func(string, string, ...string) ([]byte, error) { return nil, errors.New("permission denied") }
	if f.apply([]Spec{s}) == nil {
		t.Fatal("permission failure treated as absent")
	}
}

func TestXTOwnershipAndCleanup(t *testing.T) {
	f := newXT("test", nil, true)
	var calls []string
	f.run = func(input, binary string, args ...string) ([]byte, error) {
		call := strings.Join(args, " ")
		calls = append(calls, binary+" "+call)
		if strings.HasSuffix(call, " -S") {
			return []byte("-N " + f.chain + "\n" + strings.Join(f.sentinel(), " ") + "\n" + strings.Join(f.jump(), " ") + "\n"), nil
		}
		return nil, nil
	}
	if err := f.remove(); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 8 {
		t.Fatal(calls)
	}
	for _, c := range calls {
		if strings.HasSuffix(c, " -F") || strings.HasSuffix(c, " -X") {
			t.Fatal("global flush", c)
		}
	}
	f.run = func(input, binary string, args ...string) ([]byte, error) { return []byte("-N " + f.chain + "\n"), nil }
	if f.check(nil) == nil {
		t.Fatal("accepted unowned chain")
	}
}
