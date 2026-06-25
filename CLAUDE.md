# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A read-only CLI calendar (`cal`). Fetches ICS/CalDAV feeds in parallel, prints
the next few days of events with lipgloss styling, then exits. It never writes
to calendars. The Go module is named `cli-cal`; the built binary is `cal`.

## Commands

Use `task` (Taskfile.yml), not raw `go` commands — `task build` is the gate that
must pass before considering work done.

```sh
task build        # goimports + go vet + golangci-lint + go test, then compile to ./build/cal
task test         # tests only
task lint         # goimports (via fd), go vet, golangci-lint
task run -- -days 7   # go run . with args
task release-snapshot # local goreleaser build to ./dist, no publish
go test -run TestName ./...   # single test
```

`task build` injects the version via `-ldflags "-X main.Version=..."` from `git describe`.

## Architecture

Two source files at the repo root:

- **`main.go`** — config, fetching, ICS parsing, recurrence expansion.
- **`render.go`** — all lipgloss styles and the day-grouped output.

Pipeline: `loadConfig` → `fetchAll` (one goroutine per calendar, results merged
under a mutex) → sort by start time → `render`. Per-feed fetch errors are
collected and printed as warnings *after* the calendar, so one bad feed never
blocks the others.

### Non-obvious things

- **All feeds are treated as ICS over HTTP.** `webcal://` URLs are rewritten to
  `https://` (`normalizeURL`). There is no real CalDAV `PROPFIND` — "CalDAV" here
  means published ICS feeds.
- **`sanitizeICS` exists for Apple/iCloud feeds.** Their `X-APPLE-STRUCTURED-LOCATION`
  values contain multi-line content with inconsistent line folding that crashes
  the strict `golang-ical` parser. `sanitizeICS` rebuilds logical lines (unfolds
  proper continuations, reattaches stray broken-fold lines) before parsing. If
  parsing breaks on a new feed, look here first.
- **Recurring events** are expanded in `expandEvent` using `rrule-go`, bounded to
  the view window. Non-recurring events are filtered to the window directly.
- **All-day detection** keys off `VALUE=DATE` params / 8-char `YYYYMMDD` values.
- **Days precedence**: `-days` flag › config `days` › `defaultDays` (2).
- **Config** is YAML at `$XDG_CONFIG_HOME/cal/config.yaml` (falls back to
  `~/.config/cal`). `os.UserConfigDir()` is deliberately avoided — on darwin it
  returns `~/Library/Application Support`, not the XDG path. Missing config →
  write a commented sample and exit 0.

## llm-shared submodule

`llm-shared/` is a git submodule shared across projects. **Do not modify it**
without explicit instruction, and keep it out of tooling: tasks already filter it
with `grep -v llm-shared` and `.golangci.yml` excludes its path. It carries the
canonical Go guidelines, Taskfile/golangci/CI templates this repo is based on.

## Releases

Pushing a `v*` tag triggers `.github/workflows/release.yml` → GoReleaser builds
cross-platform archives named `cal_<version>_<os>_<arch>` (project_name pinned to
`cal`) plus `checksums.txt`, and publishes a GitHub Release. `{{.Version}}` strips
the leading `v`, so `cal --version` reports e.g. `0.1.0` for tag `v0.1.0`.
