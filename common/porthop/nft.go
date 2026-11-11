package porthop

import (
	"encoding/json"
	"fmt"
	"strings"
)

const owner = "lazycat-hy2-v1"

type nftFirewall struct {
	table string
	run   command
}

func newNFT(id string, run command) *nftFirewall {
	return &nftFirewall{table: "lc_hy2_" + id, run: run}
}

func (f *nftFirewall) exists() (bool, error) {
	out, err := f.run("", "nft", "-j", "list", "tables")
	if err != nil {
		return false, err
	}
	var data struct {
		NFTables []struct {
			Table *struct{ Family, Name, Comment string } `json:"table"`
		} `json:"nftables"`
	}
	if err = json.Unmarshal(out, &data); err != nil {
		return false, err
	}
	for _, item := range data.NFTables {
		if item.Table == nil || item.Table.Family != "inet" || item.Table.Name != f.table {
			continue
		}
		out, err = f.run("", "nft", "-j", "list", "table", "inet", f.table)
		if err != nil {
			return false, err
		}
		data.NFTables = nil
		if err = json.Unmarshal(out, &data); err != nil {
			return false, err
		}
		for _, e := range data.NFTables {
			if e.Table != nil && e.Table.Family == "inet" && e.Table.Name == f.table && e.Table.Comment == owner {
				return true, nil
			}
		}
		return false, fmt.Errorf("refusing unowned nft table %s", f.table)
	}
	return false, nil
}

func (f *nftFirewall) script(specs []Spec, exists bool) string {
	var b strings.Builder
	if exists {
		fmt.Fprintf(&b, "delete table inet %s\n", f.table)
	}
	if len(specs) == 0 {
		return b.String()
	}
	fmt.Fprintf(&b, "add table inet %s { comment \"%s\"; }\n", f.table, owner)
	fmt.Fprintf(&b, "add chain inet %s ingress { type nat hook prerouting priority dstnat; policy accept; }\n", f.table)
	for _, s := range specs {
		var ports []string
		for _, r := range s.Ranges {
			ports = append(ports, r.text("-"))
		}
		// Only destinations on this machine: never redirect routed/container
		// transit traffic. Match both IP families without guessing eth0 names.
		fmt.Fprintf(&b, "add rule inet %s ingress fib daddr type local udp dport { %s } counter redirect to :%d\n", f.table, strings.Join(ports, ", "), s.Port)
	}
	return b.String()
}

func (f *nftFirewall) check(specs []Spec) error {
	exists, err := f.exists()
	if err != nil {
		return err
	}
	_, err = f.run(f.script(specs, exists), "nft", "--check", "-f", "-")
	return err
}
func (f *nftFirewall) apply(specs []Spec) error {
	exists, err := f.exists()
	if err != nil {
		return err
	}
	_, err = f.run(f.script(specs, exists), "nft", "-f", "-") // atomic batch
	return err
}
func (f *nftFirewall) remove() error {
	exists, err := f.exists()
	if err != nil || !exists {
		return err
	}
	_, err = f.run(f.script(nil, true), "nft", "-f", "-")
	return err
}
