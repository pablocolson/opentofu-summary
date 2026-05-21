package main

import (
	"sort"
	"strconv"
	"strings"
)

type Summary struct {
	Totals   Counts
	ByType   []TypeBucket
	ByModule []ModuleBucket
	Replaces []ResourceChange
	Destroys []ResourceChange
	Warnings []Warning
}

type Counts struct {
	Create  int
	Update  int
	Delete  int
	Replace int
	Forget  int
	Read    int
	Noop    int
}

// Total returns the count of actionable changes — what tofu reports as
// "X to add, Y to change, Z to destroy" (excludes no-ops and reads).
func (c Counts) Total() int {
	return c.Create + c.Update + c.Delete + c.Replace + c.Forget
}

type TypeBucket struct {
	Type    string
	Counts  Counts
	Modules map[string]Counts
}

type ModuleBucket struct {
	Module string
	Counts Counts
	Groups []ResourceGroup
}

// ResourceGroup collapses for_each / count instances of the same (type, name)
// under the same action within one module.
type ResourceGroup struct {
	Type   string
	Name   string
	Action Action
	Count  int
}

type Warning struct {
	Severity string // "info", "warn", "danger"
	Message  string
}

// groupKey identifies a single ResourceGroup. Pulled out as a named type so
// the aggregation map has an obvious shape.
type groupKey struct {
	module, typ, name string
	action            Action
}

func Summarize(p *Plan) *Summary {
	s := &Summary{}

	typeIndex := map[string]*TypeBucket{}
	moduleIndex := map[string]*ModuleBucket{}
	groupCounts := map[groupKey]int{}

	for _, rc := range p.ResourceChanges {
		action := ParseAction(rc.Change.Actions)
		incrementCounts(&s.Totals, action)

		tb, ok := typeIndex[rc.Type]
		if !ok {
			tb = &TypeBucket{Type: rc.Type, Modules: map[string]Counts{}}
			typeIndex[rc.Type] = tb
		}
		incrementCounts(&tb.Counts, action)
		mc := tb.Modules[rc.ModuleAddress]
		incrementCounts(&mc, action)
		tb.Modules[rc.ModuleAddress] = mc

		mb, ok := moduleIndex[rc.ModuleAddress]
		if !ok {
			mb = &ModuleBucket{Module: rc.ModuleAddress}
			moduleIndex[rc.ModuleAddress] = mb
		}
		incrementCounts(&mb.Counts, action)

		groupCounts[groupKey{rc.ModuleAddress, rc.Type, rc.Name, action}]++

		switch action {
		case ActionReplace:
			s.Replaces = append(s.Replaces, rc)
		case ActionDelete:
			s.Destroys = append(s.Destroys, rc)
		}
	}

	for k, count := range groupCounts {
		mb := moduleIndex[k.module]
		mb.Groups = append(mb.Groups, ResourceGroup{
			Type:   k.typ,
			Name:   k.name,
			Action: k.action,
			Count:  count,
		})
	}
	// Sort groups within each module: most disruptive action first, then by
	// type, then by name.
	for _, mb := range moduleIndex {
		sort.Slice(mb.Groups, func(i, j int) bool {
			gi, gj := mb.Groups[i], mb.Groups[j]
			if gi.Action != gj.Action {
				return actionRank(gi.Action) < actionRank(gj.Action)
			}
			if gi.Type != gj.Type {
				return gi.Type < gj.Type
			}
			return gi.Name < gj.Name
		})
	}

	for _, mb := range moduleIndex {
		s.ByModule = append(s.ByModule, *mb)
	}
	sort.Slice(s.ByModule, func(i, j int) bool {
		return s.ByModule[i].Module < s.ByModule[j].Module
	})

	// Sort types by impact (descending), tie-breaking on name. Most affected
	// resource types should surface first.
	for _, tb := range typeIndex {
		s.ByType = append(s.ByType, *tb)
	}
	sort.Slice(s.ByType, func(i, j int) bool {
		ai, aj := s.ByType[i].Counts.Total(), s.ByType[j].Counts.Total()
		if ai != aj {
			return ai > aj
		}
		return s.ByType[i].Type < s.ByType[j].Type
	})

	s.Warnings = detectWarnings(s)
	return s
}

func incrementCounts(c *Counts, a Action) {
	switch a {
	case ActionCreate:
		c.Create++
	case ActionUpdate:
		c.Update++
	case ActionDelete:
		c.Delete++
	case ActionReplace:
		c.Replace++
	case ActionForget:
		c.Forget++
	case ActionRead:
		c.Read++
	case ActionNoop:
		c.Noop++
	}
}

func actionRank(a Action) int {
	switch a {
	case ActionReplace:
		return 0
	case ActionDelete:
		return 1
	case ActionForget:
		return 2
	case ActionUpdate:
		return 3
	case ActionCreate:
		return 4
	case ActionRead:
		return 5
	default:
		return 6
	}
}

// statefulPatterns lists case-insensitive substrings of resource type names
// that suggest persistent state — destroying or replacing these means data
// loss unless backups exist. Conservative on purpose: false positives (a
// warning on a stateless resource) cost less than false negatives.
var statefulPatterns = []string{
	"volume",
	"disk",
	"bucket",
	"blob",
	"database",
	"db_instance",
	"rds_cluster",
	"postgres",
	"mysql",
	"mongo",
	"redis_cluster",
	"snapshot",
	"filesystem",
	"efs",
}

func isStateful(resourceType string) bool {
	lt := strings.ToLower(resourceType)
	for _, p := range statefulPatterns {
		if strings.Contains(lt, p) {
			return true
		}
	}
	return false
}

func detectWarnings(s *Summary) []Warning {
	var ws []Warning

	var statefulDestroys []string
	for _, rc := range s.Destroys {
		if isStateful(rc.Type) {
			statefulDestroys = append(statefulDestroys, rc.Address)
		}
	}
	if len(statefulDestroys) > 0 {
		ws = append(ws, Warning{
			Severity: "danger",
			Message:  pluralizeAddresses("destroy of persistent resource", statefulDestroys),
		})
	}

	var statefulReplaces, otherReplaces []string
	for _, rc := range s.Replaces {
		if isStateful(rc.Type) {
			statefulReplaces = append(statefulReplaces, rc.Address)
		} else {
			otherReplaces = append(otherReplaces, rc.Address)
		}
	}
	if len(statefulReplaces) > 0 {
		ws = append(ws, Warning{
			Severity: "danger",
			Message:  pluralizeAddresses("replace of persistent resource", statefulReplaces),
		})
	}
	if len(otherReplaces) > 0 {
		ws = append(ws, Warning{
			Severity: "warn",
			Message:  pluralizeAddresses("replacement", otherReplaces),
		})
	}

	return ws
}

func pluralizeAddresses(noun string, addrs []string) string {
	if len(addrs) == 1 {
		return noun + ": " + addrs[0]
	}
	const maxShown = 5
	shown := addrs
	suffix := ""
	if len(shown) > maxShown {
		shown = shown[:maxShown]
		suffix = " (+" + strconv.Itoa(len(addrs)-maxShown) + " more)"
	}
	return noun + "s (" + strconv.Itoa(len(addrs)) + "): " + strings.Join(shown, ", ") + suffix
}
