# Project: modelmaxx

## Stack
Go 1.27, single-binary CLI (stdlib + models.dev fetch; no third-party deps).

## Layout
- `main.go` — entire CLI: scoring, commands, table rendering (~1100 lines).
- `models.json` — 46 candidate models + 8 capability metrics each; refreshed from models.dev.
- `go.mod` — module `modelmaxx`, go 1.27.0.
- `oh-my-opencode-slim.jsonc` (external, `~/.config/opencode/`) — `apply` target for presets.

## How to run / test / build
- Build: `go build -o modelmaxx .`  ← NOT `go build ./...` (that discards the main binary).
- Run: `./modelmaxx recommend` (auto-fetches prices unless `--no-fetch`).
- Other commands: `list`, `apply`, `fetch`.
- Flags: `--provider`, `--free`, `--preset`, `--no-fetch`, `--config`, `--dry-run`, `--role`.
- No test suite yet; verify by running `recommend` and eyeballing the table.

## Conventions
- All logic lives in `main.go`; resist splitting unless it grows unmanageable.
- Role config is data, not code: `roleWeights`, `roleCostSplit`, `roleCostWeight`,
  `roleVariant` are top-level maps — tune there, don't hardcode in functions.
- `roleVariant` (effort per role) comes from OMO-Slim docs, NOT from the user's
  current config — don't "fix" it to match a config value.
- Free models: merit penalty 0.7, cost floor 0.5. Keep these constants named.
- Colored terminal output; keep it readable in `--no-fetch` mode.

## Entry points
- `main()` → command dispatch.
- `bench()` — role merit. `cost()` / `effCost()` — pricing. `value` formula inline in recommend.
- `cmdRecommend` / `cmdList` — the two table renders (both use the `V_COST` header).

## Gotchas
- `go build ./...` builds packages but does NOT emit the `modelmaxx` binary — always use `go build -o modelmaxx .`.
- `W_COST` was renamed to `V_COST` (effective variant cost). If you see old references, update them.
- The `minBench` gate was intentionally removed — ranking is pure value now.
- `apply` writes to `~/.config/opencode/oh-my-opencode-slim.jsonc`; run with `--dry-run` first.
- opus 4.6 / 4.7 / 4.8 share identical price (5/25) in models.json; 4.8 wins on higher coding.
