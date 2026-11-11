package node

import (
	"fmt"

	"github.com/perfect-panel/ppanel-node/api/panel"
	"github.com/perfect-panel/ppanel-node/common/porthop"
)

type portHopper interface {
	Prepare() error
	Start() error
	Stop() error
	CleanupStale() error
}

func hoppingSpecs(protocols []panel.Protocol) ([]porthop.Spec, error) {
	var specs []porthop.Spec
	for i, p := range protocols {
		if !p.Enable || (p.Type != "hysteria" && p.Type != "hysteria2") {
			continue
		}
		s, err := porthop.Parse(p.Port, p.HopPorts)
		if err != nil {
			return nil, fmt.Errorf("Hy2 port %d: %w", p.Port, err)
		}
		if len(s.Ranges) == 0 {
			continue
		}
		for _, r := range s.Ranges {
			for j, other := range protocols {
				// Conservative: prevent a hopping range from stealing another
				// enabled protocol's port, including protocols with UDP support.
				if i != j && other.Enable && r.Contains(other.Port) {
					return nil, fmt.Errorf("Hy2 hopping range overlaps %s listening port %d", other.Type, other.Port)
				}
			}
			for _, previous := range specs {
				for _, old := range previous.Ranges {
					if r.First <= old.Last && old.First <= r.Last {
						return nil, fmt.Errorf("overlapping Hy2 hopping ranges")
					}
				}
			}
		}
		specs = append(specs, s)
	}
	return specs, nil
}
