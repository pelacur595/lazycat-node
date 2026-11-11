package node

import (
	"errors"
	"testing"
	"time"

	"github.com/perfect-panel/ppanel-node/api/panel"
)

func TestHoppingSpecs(t *testing.T) {
	base := panel.Protocol{Type: "hysteria", Enable: true, Port: 443, HopPorts: "3000-4000"}
	specs, err := hoppingSpecs([]panel.Protocol{base})
	if err != nil || len(specs) != 1 {
		t.Fatalf("%v %v", specs, err)
	}
	for _, other := range []panel.Protocol{
		{Type: "tuic", Enable: true, Port: 3500},
		{Type: "hysteria2", Enable: true, Port: 8443, HopPorts: "3999-4999"},
	} {
		if _, err := hoppingSpecs([]panel.Protocol{base, other}); err == nil {
			t.Fatal("accepted overlapping ports")
		}
	}
	base.Enable = false
	base.HopPorts = "invalid"
	if s, err := hoppingSpecs([]panel.Protocol{base}); err != nil || len(s) != 0 {
		t.Fatal("disabled protocol must be ignored")
	}
	base.Enable = true
	base.HopPorts = ""
	if s, err := hoppingSpecs([]panel.Protocol{base}); err != nil || len(s) != 0 {
		t.Fatal("fixed port must work")
	}
}

type fakeHopper struct {
	prepareErr, startErr error
	starts, stops        int
}

func (f *fakeHopper) Prepare() error      { return f.prepareErr }
func (f *fakeHopper) Start() error        { f.starts++; return f.startErr }
func (f *fakeHopper) Stop() error         { f.stops++; return nil }
func (f *fakeHopper) CleanupStale() error { return nil }

func TestNodeHoppingFailure(t *testing.T) {
	f := &fakeHopper{prepareErr: errors.New("bad range")}
	n := &Node{hopping: f, traffic: newTrafficQueue(), pushInterval: time.Hour}
	if n.Start() == nil || f.starts != 0 {
		t.Fatal("preflight must fail before starting")
	}
	f.prepareErr = nil
	f.startErr = errors.New("firewall denied")
	if n.Start() == nil || n.reportPeriodic != nil || f.stops != 1 {
		t.Fatal("partial start must stop")
	}
	f.startErr = nil
	if err := n.Start(); err != nil {
		t.Fatal(err)
	}
	if err := n.Stop(); err != nil || n.reportPeriodic != nil {
		t.Fatal("stop failed")
	}
}
