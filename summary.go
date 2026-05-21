package main

import (
	"sort"
	"strings"
)

// Summary holds aggregated information about a Plan, ready for rendering.
type Summary struct {
	Totals    Counts
	ByType    []TypeBucket
	ByModule  []ModuleBucket
	Replaces  []ResourceChange
	Destroys  []ResourceChange
	Warnings  []Warning
	FormatVer string
	TFVersion string
}

// Counts holds per-action counts.
type Counts struct {
	Create  int
	Update  int
	Delete  int
	Replace int
	Forget  int
	Read    int
	Noop    int
}

// Total returns the count of all non-noop, non-read actions — the number a
// human typically wants to see ("X to add, Y to change, Z to destroy").
func (c Counts) Total() int {
	return c.Create + c.Update + c.Delete + c.Replace + c.Forget
}

// TypeBucket groups changes by resource type, splitting by action.
type TypeBucket struct {
	Type    string
	Counts  Counts
	Modules map[string]Counts // module_address -> counts
}

// ModuleBucket groups changes by module address.
type ModuleBucket struct {
	Module string
	Counts Counts
	Groups []ResourceGroup
}

// ResourceGroup collapses multiple instances of the same (type, name) under
// the same action within one module. Useful for count/for_each resources.
type ResourceGroup struct {
	Type   string
	Name   string
	Action Action
	Count  int
	// Sample address for display (first encountered).
	Sample string
}

// Warning is a human-readable note about something noteworthy in the plan.
type Warning struct {
	Severity string // "info", "warn", "danger"
	Message  string
}

// Summarize walks a Plan and returns an aggregated Summary.
func Summarize(p *Plan) *Summary {
	s := &Summary{
		FormatVer: p.FormatVersion,
		TFVersion: p.TerraformVersion,
	}

	typeIndex := map[string]*TypeBucket{}
	moduleIndex := map[string]*ModuleBucket{}
	// (module, type, name, action) -> *ResourceGroup
	groupIndex := map[string]*ResourceGroup{}

	for _, rc := range p.ResourceChanges {
		action := ParseAction(rc.Change.Actions)
		incrementCounts(&s.Totals, action)

		// Per-type bucket.
		tb, ok := typeIndex[rc.Type]
		if !ok {
			tb = &TypeBucket{Type: rc.Type, Modules: map[string]Counts{}}
			typeIndex[rc.Type] = tb
		}
		incrementCounts(&tb.Counts, action)
		mc := tb.Modules[rc.ModuleAddress]
		incrementCounts(&mc, action)
		tb.Modules[rc.ModuleAddress] = mc

		// Per-module bucket.
		mb, ok := moduleIndex[rc.ModuleAddress]
		if !ok {
			mb = &ModuleBucket{Module: rc.ModuleAddress}
			moduleIndex[rc.ModuleAddress] = mb
		}
		incrementCounts(&mb.Counts, action)

		// Per-(module,type,name,action) group inside the module bucket.
		key := rc.ModuleAddress + "\x00" + rc.Type + "\x00" + rc.Name + "\x00" + action.String()
		g, ok := groupIndex[key]
		if !ok {
			g = &ResourceGroup{
				Type:   rc.Type,
				Name:   rc.Name,
				Action: action,
				Sample: rc.Address,
			}
			groupIndex[key] = g
			mb.Groups = append(mb.Groups, ResourceGroup{}) // placeholder, fixed below
		}
		g.Count++

		// Track destroys and replaces individually for the "pay attention" output.
		switch action {
		case ActionReplace:
			s.Replaces = append(s.Replaces, rc)
		case ActionDelete:
			s.Destroys = append(s.Destroys, rc)
		}
	}

	// Materialize per-module groups in a deterministic order.
	for _, mb := range moduleIndex {
		mb.Groups = mb.Groups[:0]
		// Collect all keys for this module from groupIndex.
		var keys []string
		prefix := mb.Module + "\x00"
		for k := range groupIndex {
			if strings.HasPrefix(k, prefix) {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(i, j int) bool {
			gi, gj := groupIndex[keys[i]], groupIndex[keys[j]]
			if gi.Action != gj.Action {
				return actionRank(gi.Action) < actionRank(gj.Action)
			}
			if gi.Type != gj.Type {
				return gi.Type < gj.Type
			}
			return gi.Name < gj.Name
		})
		for _, k := range keys {
			mb.Groups = append(mb.Groups, *groupIndex[k])
		}
	}

	// Flatten and sort modules.
	for _, mb := range moduleIndex {
		s.ByModule = append(s.ByModule, *mb)
	}
	sort.Slice(s.ByModule, func(i, j int) bool {
		return s.ByModule[i].Module < s.ByModule[j].Module
	})

	// Flatten and sort types. Sort by total change count desc, then name asc,
	// so the most impacted resource types surface first.
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

	s.Warnings = detectWarnings(p, s)
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

// actionRank orders actions so the most disruptive show up first inside a module listing.
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
// that suggest persistent state — destroying or replacing these means data loss
// unless backups exist. The list is intentionally conservative: false positives
// (a warning on a stateless resource) cost less than false negatives.
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

func detectWarnings(_ *Plan, s *Summary) []Warning {
	var ws []Warning

	// Stateful destroys.
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

	// Stateful replaces.
	var statefulReplaces []string
	for _, rc := range s.Replaces {
		if isStateful(rc.Type) {
			statefulReplaces = append(statefulReplaces, rc.Address)
		}
	}
	if len(statefulReplaces) > 0 {
		ws = append(ws, Warning{
			Severity: "danger",
			Message:  pluralizeAddresses("replace of persistent resource", statefulReplaces),
		})
	}

	// Non-stateful replaces — still worth a heads-up but lower severity.
	if len(s.Replaces) > len(statefulReplaces) {
		var others []string
		for _, rc := range s.Replaces {
			if !isStateful(rc.Type) {
				others = append(others, rc.Address)
			}
		}
		ws = append(ws, Warning{
			Severity: "warn",
			Message:  pluralizeAddresses("replacement", others),
		})
	}

	return ws
}

func pluralizeAddresses(noun string, addrs []string) string {
	if len(addrs) == 1 {
		return noun + ": " + addrs[0]
	}
	// Show up to 5 addresses, truncate the rest.
	const maxShown = 5
	shown := addrs
	suffix := ""
	if len(shown) > maxShown {
		shown = shown[:maxShown]
		suffix = " (+" + itoa(len(addrs)-maxShown) + " more)"
	}
	return noun + "s (" + itoa(len(addrs)) + "): " + strings.Join(shown, ", ") + suffix
}

// itoa is a tiny dependency-free integer to string helper.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
