# tofu-summary

[![ci](https://github.com/pablocolson/opentofu-summary/actions/workflows/ci.yml/badge.svg)](https://github.com/pablocolson/opentofu-summary/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/pablocolson/opentofu-summary?logo=github)](https://github.com/pablocolson/opentofu-summary/releases/latest)
[![license](https://img.shields.io/github/license/pablocolson/opentofu-summary)](LICENSE)

Condensed, readable summaries of OpenTofu / Terraform plans.

Built because a real `tofu apply` against a multi-region OpenStack platform
produced a 1,500-line diff for what was essentially "migrate 42 subnet routes
to router routes across 3 modules" — and the answer to "is this safe?" was
buried.

## What it does

Reads the JSON form of a plan (`tofu show -json plan.tfplan`) and prints:

- **Totals** — `+78 add  -42 destroy  ±0 replace`
- **By resource type** — collapses every indexed instance of the same
  `for_each`/`count` resource into one line with a count
- **By module** — same grouping, but rolled up per module address
- **Pay attention** — highlights replaces and destroys of resources whose
  names suggest persistent state (`*volume*`, `*db_instance*`, `*bucket*`, …)

Example output for the route-migration plan above:

```
Plan summary  +78 add  -42 destroy

By resource type
  +72  openstack_networking_router_route_v2  in 3 modules
  -42  openstack_networking_subnet_route_v2  in 3 modules
  +6   local_file                            in 3 modules

By module
  module.par1_platform[0]                    +24 -14
  module.par1_platform[0].module.kubernetes  +2
  module.par2_platform[0]                    +24 -14
  module.par2_platform[0].module.kubernetes  +2
  module.par3_platform[0]                    +24 -14
  module.par3_platform[0].module.kubernetes  +2

No replaces, no destroys of persistent resources.
```

Pass `-v` to also get per-module detail (resource type × name × action × count).

## Install

**Homebrew** (macOS and Linux):

```sh
brew install pablocolson/tap/tofu-summary
```

**From source**:

```sh
go install github.com/pablocolson/opentofu-summary@latest
```

**Pre-built binaries** for darwin/linux × amd64/arm64 are attached to every
[GitHub release](https://github.com/pablocolson/opentofu-summary/releases).

The binary is named `tofu-summary` and has zero runtime dependencies — only
the Go standard library.

## Use

Three workflows, pick whichever fits.

### 1. As a pipe after `tofu show`

```sh
tofu plan -out=tfplan
tofu show -json tfplan | tofu-summary
```

### 2. From a saved JSON file

```sh
tofu show -json tfplan > plan.json
tofu-summary -plan plan.json
```

### 3. As a wrapper

```sh
# runs `tofu plan -out=<tmp>` and prints the summary
tofu-summary plan -- -var-file=preprod.tfvars

# same, then runs `tofu apply <tmp>` against the saved plan
tofu-summary apply -- -var-file=preprod.tfvars
```

Arguments after `--` are passed through to `tofu`. Set `OPENTOFU_BINARY` to
override the `tofu` lookup (it falls back to `terraform` automatically).

## Flags

| Flag         | Effect                                                        |
| ------------ | ------------------------------------------------------------- |
| `-plan FILE` | Read plan-as-JSON from `FILE` instead of stdin                |
| `-v`         | Also print per-module resource group detail                   |
| `-no-color`  | Disable ANSI color even when stdout is a TTY                  |

## A note on "plugin"

OpenTofu's plugin system covers **providers** and **provisioners** — there is
no native hook to run code after `tofu plan` / `tofu apply`. So this is a
companion CLI rather than a plugin in the strict sense. The closest thing to
the integration most people want is the wrapper subcommand
(`tofu-summary plan`, `tofu-summary apply`), or a shell alias:

```sh
alias tofu-plan='tofu plan -out=/tmp/tfplan && tofu show -json /tmp/tfplan | tofu-summary'
```

## Development

```sh
go test ./...                                # run tests
go run ./_examples/gen_fixture | go run .    # render the route-migration demo
```

The test fixture in `summary_test.go` mirrors the real-world plan that
motivated this project, so changes to the renderer can be eyeballed by
running the demo command above. `_examples/` lives outside the install path
(the leading `_` makes `go install ./...` skip it).

## Releasing

See [RELEASE.md](RELEASE.md). In short: `git tag vX.Y.Z && git push --tags`
triggers a GitHub Actions workflow that publishes a release and updates the
Homebrew tap.

## License

MIT.
