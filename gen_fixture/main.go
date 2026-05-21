// gen_fixture writes a synthetic plan JSON to stdout matching the route migration
// scenario used in tests. Useful for eyeballing the renderer output.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Change struct {
	Actions []string `json:"actions"`
}

type ResourceChange struct {
	Address       string `json:"address"`
	ModuleAddress string `json:"module_address"`
	Mode          string `json:"mode"`
	Type          string `json:"type"`
	Name          string `json:"name"`
	ProviderName  string `json:"provider_name"`
	Change        Change `json:"change"`
}

type Plan struct {
	FormatVersion    string           `json:"format_version"`
	TerraformVersion string           `json:"terraform_version"`
	ResourceChanges  []ResourceChange `json:"resource_changes"`
}

var nutanixCIDRs = []string{
	"10.16.145.0/27", "10.16.200.0/24", "10.16.250.64/27", "10.16.39.64/26",
	"10.36.92.0/25", "10.36.93.200/29", "10.36.93.208/29",
	"100.66.105.128/25", "100.66.105.64/26", "100.66.2.198/32", "100.66.6.74/31",
	"100.69.128.0/24", "172.16.21.0/24", "172.16.248.0/24",
	"10.150.26.69/32", "10.150.26.71/32",
	"10.250.0.222/32", "10.250.1.222/32",
	"10.250.4.236/32", "10.250.4.237/32",
	"172.16.0.221/32", "172.16.0.53/32",
}

var subnetCIDRs = nutanixCIDRs[:14]

func make_(modAddr, rtype, name, index, action string) ResourceChange {
	addr := modAddr + "." + rtype + "." + name
	if index != "" {
		addr += fmt.Sprintf("[%q]", index)
	}
	return ResourceChange{
		Address:       addr,
		ModuleAddress: modAddr,
		Mode:          "managed",
		Type:          rtype,
		Name:          name,
		ProviderName:  "registry.opentofu.org/hashicorp/openstack",
		Change:        Change{Actions: []string{action}},
	}
}

func main() {
	p := &Plan{FormatVersion: "1.2", TerraformVersion: "1.11.4"}
	platforms := []struct {
		name    string
		interco []string
	}{
		{"module.par1_platform[0]", []string{"10.222.104.0/21", "10.222.112.0/21"}},
		{"module.par2_platform[0]", []string{"10.222.96.0/21", "10.222.112.0/21"}},
		{"module.par3_platform[0]", []string{"10.222.104.0/21", "10.222.96.0/21"}},
	}
	for _, pl := range platforms {
		for _, c := range subnetCIDRs {
			p.ResourceChanges = append(p.ResourceChanges,
				make_(pl.name, "openstack_networking_subnet_route_v2", "nutanix_subnet_route", c, "delete"))
		}
		for _, c := range nutanixCIDRs {
			p.ResourceChanges = append(p.ResourceChanges,
				make_(pl.name, "openstack_networking_router_route_v2", "nutanix_router_route", c, "create"))
		}
		for _, c := range pl.interco {
			p.ResourceChanges = append(p.ResourceChanges,
				make_(pl.name, "openstack_networking_router_route_v2", "interco_dc_router_route", c, "create"))
		}
		kmod := pl.name + ".module.kubernetes"
		p.ResourceChanges = append(p.ResourceChanges,
			make_(kmod, "local_file", "zoned_talos_control_plane_machine_configuration", "", "create"),
			make_(kmod, "local_file", "zoned_talos_worker_machine_configuration", "", "create"),
		)
	}
	if err := json.NewEncoder(os.Stdout).Encode(p); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
