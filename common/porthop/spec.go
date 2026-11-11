// Package porthop redirects Hy2 UDP hopping ports to one listening socket.
// The design follows h-ui's port-forwarding approach; implementation is local.
package porthop

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Range struct{ First, Last int }
type Spec struct {
	Port   int
	Ranges []Range
}

func Parse(port int, raw string) (Spec, error) {
	s := Spec{Port: port}
	if strings.TrimSpace(raw) == "" {
		return s, nil
	}
	if port < 1 || port > 65535 || len(raw) > 16384 {
		return s, fmt.Errorf("invalid Hy2 port or oversized hop_ports")
	}
	for _, part := range strings.Split(raw, ",") {
		bounds := strings.Split(strings.TrimSpace(part), "-")
		if len(bounds) > 2 {
			return s, fmt.Errorf("invalid Hy2 port range")
		}
		values := make([]int, len(bounds))
		for i, b := range bounds {
			if b == "" || strings.Trim(b, "0123456789") != "" {
				return s, fmt.Errorf("invalid Hy2 port number")
			}
			n, err := strconv.Atoi(b)
			if err != nil || n < 1 || n > 65535 {
				return s, fmt.Errorf("Hy2 port outside 1-65535")
			}
			values[i] = n
		}
		r := Range{values[0], values[len(values)-1]}
		if r.Last < r.First {
			return s, fmt.Errorf("reversed Hy2 port range")
		}
		s.Ranges = append(s.Ranges, r)
	}
	sort.Slice(s.Ranges, func(i, j int) bool { return s.Ranges[i].First < s.Ranges[j].First })
	merged := []Range{s.Ranges[0]}
	for _, r := range s.Ranges[1:] {
		last := &merged[len(merged)-1]
		if r.First <= last.Last+1 {
			last.Last = max(last.Last, r.Last)
		} else {
			merged = append(merged, r)
		}
	}
	s.Ranges = merged
	return s, nil
}

func (r Range) Contains(port int) bool { return port >= r.First && port <= r.Last }
func (r Range) text(separator string) string {
	if r.First == r.Last {
		return strconv.Itoa(r.First)
	}
	return fmt.Sprintf("%d%s%d", r.First, separator, r.Last)
}
