# modelmaxx

> Coding bang-for-buck model recommender for the oh-my-opencode-slim (OMO-Slim) agent harness.

`modelmaxx` is a small Go CLI that recommends and configures the best opencode
model preset for each OMO-Slim agent role, based on a role-specific, multi-metric
"coding bang for buck" score. It weights capability metrics per role, applies a
free-model penalty, prices each model by its real $/1M token cost (and the cost
impact of the role's recommended reasoning variant), and ranks by **value**
(capability per dollar).

## Why

OMO-Slim splits work across 7 specialist agents (orchestrator, oracle, council,
librarian, explorer, designer, fixer). Each role needs a different capability
profile — an oracle needs raw reasoning; an explorer needs speed and cheapness;
a designer needs visual judgment. A single "best model" is wrong. `modelmaxx`
scores every candidate model against each role's profile and picks the best value.

## How models are ranked & rated per role

### Metrics
Every model is rated on 8 capability metrics (0–100):
`coding`, `visual`, `reasoning`, `speed`, `context`, `tooluse`, `instruction`, `multimodal`.

### Role weights
Each role has a weight vector over the 8 metrics (summing to 1.0), derived from the
OMO-Slim "Model Guidance" for that agent. Example: `oracle` is reasoning-dominant
(0.45), `explorer` is speed-dominant (0.40), `designer` is visual-dominant (0.45).

### Merit (`bench`)
```
bench(role, m) = Σ(roleWeight · metric) × freePenalty
freePenalty = 0.7 if model is free else 1.0
```
Free models are penalized 30% on merit (reliability/quality risk).

### Cost
```
cost(role, m)   = priceIn · wIn + priceOut · wOut        # roleCostSplit per role
effCost(V_COST) = cost × variantMult        (paid)
                = max(cost, 0.5)            (free, floored)
```
`variantMult` reflects the role's recommended reasoning effort: low/none = 1.0,
medium = 1.3, high = 1.8. Free models are floored at `freeCostFloor = 0.5`
(regardless of variant — they cost $0; the floor captures reliability risk).

### Value (the ranking score)
```
value(role, m) = bench(role, m) / cost(role, m) ^ roleCostWeight
```
`roleCostWeight` is a per-role exponent. High exponent (0.70 for
librarian/explorer) → cost matters a lot, cheap/free models win. Low exponent
(0.05–0.15 for thinking/design roles) → capability dominates, paid models win.

### Ranking
Models are ranked by `value` descending per role. There is no hard merit gate —
ranking is pure value (a previous `minBench` floor was removed as a crude crutch
that contradicted the weights).

### Design principle
The goal is **not** to pick paid models for their own sake. When two models are
within ~1–2 bench points (negligible quality difference), the cheaper/free model
should win. This is a future refinement (a "free wins on near-tie" rule) beyond
the current exponent tuning.

## Current recommendations
| Role | Model | Notes |
|------|-------|-------|
| orchestrator | gpt-5-6-luna | generalist |
| oracle | gpt-5-6-luna | reasoning |
| council | gpt-5-6-luna | synthesis |
| librarian | gpt-5-6-luna | cheap + fast |
| explorer | gpt-5-6-luna | cheap + fast |
| designer | claude-opus-4-8 | visual divergence |
| fixer | gpt-5-6-luna | coding-dominant |

(6/7 pick the luna generalist; designer diverges on visual capability.)

## Build & run
```bash
go build -o modelmaxx .      # IMPORTANT: not `go build ./...` (discards the binary)
./modelmaxx recommend         # ranks all roles, auto-fetches prices from models.dev
./modelmaxx list              # shows all models + metrics
./modelmaxx apply --preset free --dry-run   # preview preset edits
```

### Commands
- `list` — list models and metrics
- `recommend` — recommend a model per role
- `apply` — write recommendations into an opencode config
- `fetch` — refresh prices/contexts from models.dev

### Flags
- `--provider` (opencode|copilot|all)
- `--free` — restrict to free models
- `--preset` — target preset name
- `--no-fetch` — skip the models.dev price refresh
- `--config` — path to opencode config
- `--dry-run` — preview only
- `--role` — limit to one role

## Data
`models.json` holds the candidate models and their metrics (46 entries). Prices
and contexts are refreshed from [models.dev](https://models.dev) on each run
unless `--no-fetch`.

## License
MIT — see [LICENSE](LICENSE).
