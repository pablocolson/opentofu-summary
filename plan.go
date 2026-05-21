package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// Plan is a minimal subset of the `tofu show -json` / `terraform show -json` schema.
// We only model the fields we need to produce a summary. The full schema is documented at
// https://opentofu.org/docs/internals/json-format/ and is stable across the 1.x line.
type Plan struct {
	FormatVersion    string            `json:"format_version"`
	TerraformVersion string            `json:"terraform_version"`
	ResourceChanges  []ResourceChange  `json:"resource_changes"`
	OutputChanges    map[string]Change `json:"output_changes,omitempty"`
}

type ResourceChange struct {
	Address       string `json:"address"`
	ModuleAddress string `json:"module_address,omitempty"`
	Mode          string `json:"mode"`
	Type          string `json:"type"`
	Name          string `json:"name"`
	ProviderName  string `json:"provider_name"`
	Change        Change `json:"change"`
}

type Change struct {
	Actions      []string        `json:"actions"`
	Before       json.RawMessage `json:"before,omitempty"`
	After        json.RawMessage `json:"after,omitempty"`
	ReplacePaths json.RawMessage `json:"replace_paths,omitempty"`
	Importing    json.RawMessage `json:"importing,omitempty"`
}

// Action represents the kind of change for a single resource. It is computed from
// Change.Actions which can be ["no-op"], ["create"], ["read"], ["update"], ["delete"],
// ["delete","create"] (replace, destroy-first) or ["create","delete"] (replace,
// create-before-destroy). OpenTofu also defines ["forget"] for resources removed
// from state without destroying the underlying object.
type Action int

const (
	ActionNoop Action = iota
	ActionCreate
	ActionRead
	ActionUpdate
	ActionDelete
	ActionReplace
	ActionForget
	ActionUnknown
)

func (a Action) String() string {
	switch a {
	case ActionNoop:
		return "no-op"
	case ActionCreate:
		return "create"
	case ActionRead:
		return "read"
	case ActionUpdate:
		return "update"
	case ActionDelete:
		return "destroy"
	case ActionReplace:
		return "replace"
	case ActionForget:
		return "forget"
	default:
		return "unknown"
	}
}

// ParseAction reduces a Change.Actions slice to a single Action. The semantics
// match those used by tofu/terraform when printing the textual plan.
func ParseAction(actions []string) Action {
	switch len(actions) {
	case 0:
		return ActionUnknown
	case 1:
		switch actions[0] {
		case "no-op":
			return ActionNoop
		case "create":
			return ActionCreate
		case "read":
			return ActionRead
		case "update":
			return ActionUpdate
		case "delete":
			return ActionDelete
		case "forget":
			return ActionForget
		}
	case 2:
		// ["delete","create"] or ["create","delete"] — both mean replace.
		a, b := actions[0], actions[1]
		if (a == "delete" && b == "create") || (a == "create" && b == "delete") {
			return ActionReplace
		}
	}
	return ActionUnknown
}

// ParsePlan decodes a tofu/terraform plan-as-JSON document.
func ParsePlan(r io.Reader) (*Plan, error) {
	var p Plan
	dec := json.NewDecoder(r)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("decoding plan JSON: %w", err)
	}
	return &p, nil
}
