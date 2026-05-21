# Releasing tofu-summary

End-to-end checklist for shipping a new version. The first release is the only
one that needs the one-time setup in §1.

## 1. One-time setup

### 1a. Push the source repo to GitHub

```sh
# The local repo is already initialized; create the remote and push.
gh repo create pablocolson/opentofu-summary --public --source=. --remote=origin --push
# …or without `gh`:
#   git remote add origin git@github.com:pablocolson/opentofu-summary.git
#   git push -u origin main
```

### 1b. Create the Homebrew tap repo

A Homebrew tap is just a GitHub repo whose name starts with `homebrew-`.
GoReleaser will commit the generated formula into it on every release.

```sh
gh repo create pablocolson/homebrew-tap --public --add-readme
```

You don't need to clone it — GoReleaser pushes to it via the GitHub API. The
repo needs to exist (empty is fine) and contain a `Formula/` directory (or
have one created on first push; GoReleaser will create it for you).

### 1c. Create a Personal Access Token for cross-repo pushes

The default `GITHUB_TOKEN` in Actions can only write to the current repo,
so GoReleaser needs a separate token to push to the tap.

1. Open <https://github.com/settings/tokens/new?scopes=repo&description=goreleaser-tap>
2. Generate a classic token with the `repo` scope (or `public_repo` if the
   tap will stay public — recommended).
3. Copy the token.
4. Add it as a repository secret on `pablocolson/opentofu-summary`:

   ```sh
   gh secret set HOMEBREW_TAP_GITHUB_TOKEN --body 'ghp_xxx…'
   ```

   Or via the UI: `Settings → Secrets and variables → Actions → New repository
   secret`, name `HOMEBREW_TAP_GITHUB_TOKEN`.

## 2. Cut a release

For every release, including the first:

```sh
# 1. make sure main is clean and tests pass locally
go test ./...

# 2. tag and push — the v prefix is required, semver after it
git tag v0.1.0
git push origin v0.1.0
```

That's it. GitHub Actions picks up the tag and runs GoReleaser, which:

- builds binaries for darwin/amd64, darwin/arm64, linux/amd64, linux/arm64
- creates a GitHub release with the archives and a `checksums.txt`
- updates `Formula/tofu-summary.rb` in `pablocolson/homebrew-tap` to point
  at the new release artifacts

You can watch the run with:

```sh
gh run watch
```

## 3. Install via Homebrew

Once the workflow is green:

```sh
brew install pablocolson/tap/tofu-summary
```

The `pablocolson/tap` shorthand resolves to `https://github.com/pablocolson/homebrew-tap`.

To upgrade later:

```sh
brew update && brew upgrade tofu-summary
```

## 4. If something goes wrong

- **Workflow fails on `brews:` step** — likely the `HOMEBREW_TAP_GITHUB_TOKEN`
  secret is missing or unauthorized. The release itself still publishes; you
  just need to add the secret and re-run, or remove the `brews:` block from
  `.goreleaser.yaml` to ship without Homebrew for now.
- **`brew install` fails with a 404** — the GitHub release artifacts must be
  publicly accessible. Confirm `pablocolson/opentofu-summary` is public.
- **Tap repo says "no such formula"** — confirm GoReleaser pushed to
  `homebrew-tap` (check the repo's commit history). If not, look at the
  Actions log for permission errors on the tap repo.

## 5. Local sanity check (optional)

Before cutting the first tag you can dry-run goreleaser against the current
HEAD without publishing anything:

```sh
brew install goreleaser
goreleaser release --snapshot --clean --skip=publish
```

Outputs go to `dist/`. If `tofu-summary_*_darwin_arm64.tar.gz` shows up there
and the `dist/homebrew/Formula/tofu-summary.rb` file looks plausible, the real
release will work too.
