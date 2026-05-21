package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// Plan is a minimal subset of the `tofu show -json` / `terraform show -json`
// schema — only the fields the summarizer reads. Full schema:
// https://opentofu.org/docs/internals/json-format/ (stable across 1.x).
type Plan struct {
	FormatVersion    string           `json:"format_version"`
	TerraformVersion string           `json:"terraform_version"`
	ResourceChanges  []ResourceChange `json:"resource_changes"`
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
	Actions []string `json:"actions"`
}

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

// ParseAction reduces a Change.Actions slice to a single Action. The shapes
// tofu/terraform emit are: ["no-op"], ["create"], ["read"], ["update"],
// ["delete"], ["forget"], ["delete","create"] (replace) and
// ["create","delete"] (replace with create-before-destroy).
func ParseAction(actions []string) Action {
	switch len(actions) {
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
		a, b := actions[0], actions[1]
		if (a == "delete" && b == "create") || (a == "create" && b == "delete") {
			return ActionReplace
		}
	}
	return ActionUnknown
}

func ParsePlan(r io.Reader) (*Plan, error) {
	var p Plan
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return nil, fmt.Errorf("decoding plan JSON: %w", err)
	}
	return &p, nil
}
