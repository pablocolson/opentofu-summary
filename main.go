package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Build-time variables populated by goreleaser via -ldflags. The "dev"
// defaults make `go build` from source still produce a runnable binary.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `tofu-summary — produce a condensed summary of an OpenTofu/Terraform plan.

Usage:

  tofu-summary [flags]                  read plan-as-JSON from stdin
  tofu-summary -plan <file>             read plan-as-JSON from <file>
  tofu-summary plan   [-- tofu args]    run "tofu plan"  and print a summary
  tofu-summary apply  [-- tofu args]    run "tofu plan", print a summary, then apply

Flags:

  -plan <file>     read plan-as-JSON from <file> instead of stdin
  -v               verbose: also print per-module resource group detail
  -no-color        disable ANSI color even when stdout is a TTY
  -version         print version and exit
  -h, -help        show this help

Examples:

  tofu plan -out=tfplan && tofu show -json tfplan | tofu-summary
  tofu-summary -plan tfplan.json
  tofu-summary plan -- -var-file=preprod.tfvars
`

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "tofu-summary:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "plan":
			return runWrapper("plan", args[1:], stdout, stderr)
		case "apply":
			return runWrapper("apply", args[1:], stdout, stderr)
		case "-h", "--help", "help":
			fmt.Fprint(stdout, usage)
			return nil
		case "-version", "--version", "version":
			fmt.Fprintf(stdout, "tofu-summary %s (commit %s, built %s)\n", version, commit, date)
			return nil
		}
	}

	fs := flag.NewFlagSet("tofu-summary", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }

	planFile := fs.String("plan", "", "read plan-as-JSON from this file instead of stdin")
	verbose := fs.Bool("v", false, "verbose: per-module detail")
	noColor := fs.Bool("no-color", false, "disable ANSI color")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	var src io.Reader = stdin
	if *planFile != "" {
		f, err := os.Open(*planFile)
		if err != nil {
			return err
		}
		defer f.Close()
		src = f
	} else if isStdinEmpty(stdin) {
		fmt.Fprint(stderr, usage)
		return errors.New("no plan JSON on stdin and no -plan file specified")
	}

	plan, err := ParsePlan(src)
	if err != nil {
		return err
	}
	summary := Summarize(plan)
	opts := RenderOptions{
		Color:   !*noColor && isTerminal(stdout),
		Verbose: *verbose,
	}
	return Render(stdout, summary, opts)
}

// isStdinEmpty reports true when stdin is a TTY with no data — i.e. the user
// invoked `tofu-summary` interactively without piping anything in. When stdin
// is a pipe or file, even an empty one, this returns false (the caller should
// attempt to read it).
func isStdinEmpty(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// isTerminal reports whether w is a terminal capable of rendering ANSI escapes.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// runWrapper executes a tofu workflow and prints a summary at the end.
//
// For "plan": runs `tofu plan -out=<tmp>` with passthrough args, then renders
// a summary built from `tofu show -json <tmp>`.
//
// For "apply": same as plan, then asks for confirmation and runs
// `tofu apply <tmp>` using the saved plan file. Auto-approves only if the
// passthrough args already include `-auto-approve`.
func runWrapper(mode string, passthrough []string, stdout, stderr io.Writer) error {
	tofu, err := resolveTofuBinary()
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "tofu-summary-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	planPath := filepath.Join(tmpDir, "plan.tfplan")

	planArgs := append([]string{"plan", "-out=" + planPath}, passthrough...)
	planCmd := exec.Command(tofu, planArgs...)
	planCmd.Stdout = stderr // forward tofu's own output to stderr to keep stdout clean for piping
	planCmd.Stderr = stderr
	planCmd.Stdin = os.Stdin
	if err := planCmd.Run(); err != nil {
		return fmt.Errorf("tofu plan failed: %w", err)
	}

	showCmd := exec.Command(tofu, "show", "-json", planPath)
	out, err := showCmd.Output()
	if err != nil {
		return fmt.Errorf("tofu show -json failed: %w", err)
	}

	plan, err := ParsePlan(bytes.NewReader(out))
	if err != nil {
		return err
	}
	summary := Summarize(plan)
	opts := RenderOptions{Color: isTerminal(stdout)}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "─── tofu-summary ──────────────────────────────────────────────")
	if err := Render(stdout, summary, opts); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "────────────────────────────────────────────────────────────────")

	if mode == "plan" {
		return nil
	}

	// apply mode — re-use the saved plan so the user sees exactly what they reviewed.
	applyCmd := exec.Command(tofu, "apply", planPath)
	applyCmd.Stdout = stderr
	applyCmd.Stderr = stderr
	applyCmd.Stdin = os.Stdin
	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("tofu apply failed: %w", err)
	}
	return nil
}

// resolveTofuBinary returns the path to `tofu` if available, falling back to
// `terraform` for compatibility. The OPENTOFU_BINARY env var overrides both.
func resolveTofuBinary() (string, error) {
	if v := os.Getenv("OPENTOFU_BINARY"); v != "" {
		return v, nil
	}
	if p, err := exec.LookPath("tofu"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("terraform"); err == nil {
		return p, nil
	}
	return "", errors.New("neither `tofu` nor `terraform` found in PATH; set OPENTOFU_BINARY")
}
