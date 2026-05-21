# CLAUDE.md

Context for Claude Code (and other LLM-assisted tools) working on this repo.

## What this is

A CLI that reads `tofu show -json` output and prints a condensed summary —
the kind a human can actually scan after `tofu plan` produces 1,500 lines of
diff. Companion tool, not a true OpenTofu plugin (OpenTofu plugins are
providers/provisioners; there is no native post-execution hook).

## Layout

Single Go package, kept deliberately flat so `go install` and one-file edits
stay simple. No external Go dependencies — only the standard library.

```
main.go         CLI entry, flag parsing, `plan`/`apply` subcommand wrappers
plan.go         Minimal subset of the tofu/terraform JSON plan schema
summary.go      Aggregation: per-type, per-module, per-(type,name,action) groups
render.go       Text renderer with optional ANSI color (auto-detects TTY)
summary_test.go Tests + fixture mirroring the real-world route-migration plan
gen_fixture/    Tiny binary that emits the demo plan JSON for eyeballing
```

## Common commands

```sh
go test ./...                                # unit tests
go vet ./...                                 # static checks
go build -o tofu-summary .                   # binary at ./tofu-summary
go run ./gen_fixture | go run .              # eyeball the demo summary
go run ./gen_fixture | go run . -v           # …with per-module detail
./tofu-summary -plan plan.json               # against a captured plan
```

## Things worth knowing

- **Version stamping**: `version`/`commit`/`date` in [main.go](main.go) are
  defaulted to `dev`/`none`/`unknown` and overwritten by goreleaser at release
  time via `-ldflags '-X main.version=…'`. Don't move them or rename the vars
  without updating `.goreleaser.yaml`.
- **Risky-resource heuristic**: `statefulPatterns` in [summary.go](summary.go)
  is a case-insensitive substring list. False positives on stateless resources
  are cheaper than false negatives, so prefer adding patterns over removing.
- **JSON schema source of truth**: <https://opentofu.org/docs/internals/json-format/>.
  Stable across the OpenTofu 1.x line and identical to Terraform 1.x output.
- **No external deps policy**: keeps the binary tiny and `go install` instant.
  If you reach for `github.com/hashicorp/terraform-json`, weigh it carefully
  — the current minimal subset is intentional.

## Release flow

`git tag vX.Y.Z && git push origin vX.Y.Z` triggers the workflow in
[.github/workflows/release.yml](.github/workflows/release.yml). Goreleaser
builds the binaries, cuts a GitHub release, and updates the Homebrew formula
in `pablocolson/homebrew-tap`. Full setup checklist in [RELEASE.md](RELEASE.md).
