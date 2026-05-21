package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// buildRouteMigrationPlan constructs an in-memory Plan matching the real-world
// scenario the tool was designed to summarize: subnet routes torn down and
// recreated as router routes across three OpenStack platforms, plus a handful
// of local_file outputs. See the project README for the original raw plan.
func buildRouteMigrationPlan() *Plan {
	p := &Plan{FormatVersion: "1.2"}

	// Per-platform CIDR sets.
	platforms := []struct {
		name             string
		nextHop          string
		subnetCIDRs      []string // destroyed
		nutanixRouteCIDR []string // created — superset of subnetCIDRs
		intercoCIDR      []string // created (new)
	}{
		{
			name:    "par1_platform[0]",
			nextHop: "10.222.97.52",
			subnetCIDRs: []string{
				"10.16.145.0/27", "10.16.200.0/24", "10.16.250.64/27", "10.16.39.64/26",
				"10.36.92.0/25", "10.36.93.200/29", "10.36.93.208/29",
				"100.66.105.128/25", "100.66.105.64/26", "100.66.2.198/32", "100.66.6.74/31",
				"100.69.128.0/24", "172.16.21.0/24", "172.16.248.0/24",
			},
			nutanixRouteCIDR: cidrSuperset,
			intercoCIDR:      []string{"10.222.104.0/21", "10.222.112.0/21"},
		},
		{
			name:    "par2_platform[0]",
			nextHop: "10.222.106.29",
			subnetCIDRs: []string{
				"10.16.145.0/27", "10.16.200.0/24", "10.16.250.64/27", "10.16.39.64/26",
				"10.36.92.0/25", "10.36.93.200/29", "10.36.93.208/29",
				"100.66.105.128/25", "100.66.105.64/26", "100.66.2.198/32", "100.66.6.74/31",
				"100.69.128.0/24", "172.16.21.0/24", "172.16.248.0/24",
			},
			nutanixRouteCIDR: cidrSuperset,
			intercoCIDR:      []string{"10.222.96.0/21", "10.222.112.0/21"},
		},
		{
			name:    "par3_platform[0]",
			nextHop: "10.222.113.18",
			subnetCIDRs: []string{
				"10.16.145.0/27", "10.16.200.0/24", "10.16.250.64/27", "10.16.39.64/26",
				"10.36.92.0/25", "10.36.93.200/29", "10.36.93.208/29",
				"100.66.105.128/25", "100.66.105.64/26", "100.66.2.198/32", "100.66.6.74/31",
				"100.69.128.0/24", "172.16.21.0/24", "172.16.248.0/24",
			},
			nutanixRouteCIDR: cidrSuperset,
			intercoCIDR:      []string{"10.222.104.0/21", "10.222.96.0/21"},
		},
	}

	for _, pl := range platforms {
		mod := "module." + pl.name
		// destroyed subnet routes
		for _, c := range pl.subnetCIDRs {
			p.ResourceChanges = append(p.ResourceChanges, makeChange(
				mod, "openstack_networking_subnet_route_v2", "nutanix_subnet_route", c, "delete",
			))
		}
		// created nutanix router routes
		for _, c := range pl.nutanixRouteCIDR {
			p.ResourceChanges = append(p.ResourceChanges, makeChange(
				mod, "openstack_networking_router_route_v2", "nutanix_router_route", c, "create",
			))
		}
		// created interco_dc router routes
		for _, c := range pl.intercoCIDR {
			p.ResourceChanges = append(p.ResourceChanges, makeChange(
				mod, "openstack_networking_router_route_v2", "interco_dc_router_route", c, "create",
			))
		}
		// 2 local_file creates per platform (nested under module.kubernetes)
		kmod := mod + ".module.kubernetes"
		p.ResourceChanges = append(p.ResourceChanges, makeChange(
			kmod, "local_file", "zoned_talos_control_plane_machine_configuration", "", "create",
		))
		p.ResourceChanges = append(p.ResourceChanges, makeChange(
			kmod, "local_file", "zoned_talos_worker_machine_configuration", "", "create",
		))
	}

	return p
}

// cidrSuperset is the union of all 22 nutanix CIDRs per platform: the 14
// existing subnet CIDRs plus 8 newly-added /32s and /24s.
var cidrSuperset = []string{
	// the 14 carried over from subnet_route
	"10.16.145.0/27", "10.16.200.0/24", "10.16.250.64/27", "10.16.39.64/26",
	"10.36.92.0/25", "10.36.93.200/29", "10.36.93.208/29",
	"100.66.105.128/25", "100.66.105.64/26", "100.66.2.198/32", "100.66.6.74/31",
	"100.69.128.0/24", "172.16.21.0/24", "172.16.248.0/24",
	// the 8 newly added
	"10.150.26.69/32", "10.150.26.71/32",
	"10.250.0.222/32", "10.250.1.222/32",
	"10.250.4.236/32", "10.250.4.237/32",
	"172.16.0.221/32", "172.16.0.53/32",
}

// makeChange builds a single ResourceChange. If index is "" the address has
// no [...] suffix — used for non-count/for_each resources.
func makeChange(modAddr, rtype, name, index, action string) ResourceChange {
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
		Change: Change{
			Actions: []string{action},
		},
	}
}

func TestSummarize_RouteMigration(t *testing.T) {
	p := buildRouteMigrationPlan()
	s := Summarize(p)

	if got, want := s.Totals.Create, 78; got != want {
		t.Errorf("Totals.Create = %d, want %d", got, want)
	}
	if got, want := s.Totals.Delete, 42; got != want {
		t.Errorf("Totals.Delete = %d, want %d", got, want)
	}
	if got, want := s.Totals.Update, 0; got != want {
		t.Errorf("Totals.Update = %d, want %d", got, want)
	}
	if got, want := s.Totals.Replace, 0; got != want {
		t.Errorf("Totals.Replace = %d, want %d", got, want)
	}

	// 3 platform modules + 3 kubernetes submodules = 6 module buckets.
	if got, want := len(s.ByModule), 6; got != want {
		t.Errorf("len(ByModule) = %d, want %d", got, want)
	}

	// Find the par1 module and verify its group counts.
	var par1 *ModuleBucket
	for i := range s.ByModule {
		if s.ByModule[i].Module == "module.par1_platform[0]" {
			par1 = &s.ByModule[i]
			break
		}
	}
	if par1 == nil {
		t.Fatal("expected module.par1_platform[0] in summary")
	}
	if got, want := par1.Counts.Create, 24; got != want {
		t.Errorf("par1 creates = %d, want %d", got, want)
	}
	if got, want := par1.Counts.Delete, 14; got != want {
		t.Errorf("par1 deletes = %d, want %d", got, want)
	}
	// Should have 3 groups: nutanix_subnet_route(delete×14), nutanix_router_route(create×22),
	// interco_dc_router_route(create×2). Sorted with deletes first.
	if got, want := len(par1.Groups), 3; got != want {
		t.Errorf("par1 group count = %d, want %d", got, want)
	}
	if len(par1.Groups) >= 1 && par1.Groups[0].Action != ActionDelete {
		t.Errorf("par1 first group action = %s, want delete (most disruptive first)",
			par1.Groups[0].Action)
	}

	// No replaces, no stateful destroys -> no warnings.
	if len(s.Warnings) != 0 {
		t.Errorf("expected no warnings on a pure route migration, got %d: %+v",
			len(s.Warnings), s.Warnings)
	}
}

func TestSummarize_Replace(t *testing.T) {
	p := &Plan{
		ResourceChanges: []ResourceChange{{
			Address:       "module.db.aws_db_instance.main",
			ModuleAddress: "module.db",
			Type:          "aws_db_instance",
			Name:          "main",
			Change:        Change{Actions: []string{"delete", "create"}},
		}},
	}
	s := Summarize(p)
	if s.Totals.Replace != 1 {
		t.Fatalf("expected 1 replace, got %d", s.Totals.Replace)
	}
	if len(s.Warnings) == 0 {
		t.Fatal("expected a warning for stateful replace, got none")
	}
	if s.Warnings[0].Severity != "danger" {
		t.Errorf("expected danger severity for db_instance replace, got %s",
			s.Warnings[0].Severity)
	}
}

func TestParsePlan_FromJSON(t *testing.T) {
	// Round-trip: build a plan, marshal it, parse it back.
	original := buildRouteMigrationPlan()
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePlan(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(parsed.ResourceChanges), len(original.ResourceChanges); got != want {
		t.Errorf("round-trip resource change count = %d, want %d", got, want)
	}
}

func TestRender_RouteMigration_Snapshot(t *testing.T) {
	// Smoke test for the renderer: ensure key strings appear in output.
	p := buildRouteMigrationPlan()
	s := Summarize(p)
	var buf bytes.Buffer
	if err := Render(&buf, s, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"Plan summary",
		"+78 add",
		"-42 destroy",
		"By resource type",
		"openstack_networking_router_route_v2",
		"openstack_networking_subnet_route_v2",
		"local_file",
		"in 3 modules",
		"By module",
		"module.par1_platform[0]",
		"No replaces",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, did not.\n--- OUTPUT ---\n%s", want, out)
		}
	}
}

func TestParseAction(t *testing.T) {
	cases := []struct {
		in   []string
		want Action
	}{
		{[]string{"no-op"}, ActionNoop},
		{[]string{"create"}, ActionCreate},
		{[]string{"delete"}, ActionDelete},
		{[]string{"update"}, ActionUpdate},
		{[]string{"read"}, ActionRead},
		{[]string{"forget"}, ActionForget},
		{[]string{"delete", "create"}, ActionReplace},
		{[]string{"create", "delete"}, ActionReplace},
		{[]string{}, ActionUnknown},
	}
	for _, tc := range cases {
		if got := ParseAction(tc.in); got != tc.want {
			t.Errorf("ParseAction(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
