# modelmaxx

Its a new week, so there is new models, new pricing, new tweaks, new benchmarks... it's an endless
task to continuously optimise your harness, agents and model selections to keep the quality up
whilst keeping the price down.

Is the latest and greatest good? Does it excel at the things you need it to? Does a model that works
well for one usecase suffice for others? Or am I throwing money after the marketing hype? Perhaps a
free model performs just as well or good enough for my research agent but it's worth paying a
premium for a special case?

modelmaxx pulls the latest pricing data and benchmarks from major providers (opencode and github
copilot currently), weights their performance according to specific roles and recommends the best
bang for buck for each type of agent in your harness!

If you are using oh-my-opencode-slim like me, it can even update your configuration too! Even if you
are using something else, you can map the OMO-Slim roles to those of your own harness to get those
recommendations.

# Build & run

```bash
go build -o modelmaxx .      # IMPORTANT: not `go build ./...` (discards the binary)
./modelmaxx recommend         # ranks all roles, auto-fetches prices from models.dev
./modelmaxx list              # shows all models + metrics
./modelmaxx apply --preset free --dry-run   # preview preset edits
```

# Roles

modelmaxx is designed with oh-my-opencode-slim in mind. But the roles are usually easily
translatable to your own set-up too.

OMO-slim have multiple pre-configured agents:

| Role         | What it does                                                                        | What we weight                                                                          |
| ------------ | ----------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| orchestrator | Plans, dispatches and judges across agents — needs sound judgment and obedience     | reasoning (0.30) + instruction (0.30) lead; context (0.15); light coding (0.05)         |
| oracle       | Deep-thinking reviewer of code, architecture & approach — highest reasoning demands | reasoning (0.45) dominant; context (0.20), instruction (0.15), coding (0.15)            |
| council      | Synthesises perspectives for high-stakes decisions                                  | reasoning (0.40) + instruction (0.25) for synthesis; context (0.20), coding (0.15)      |
| librarian    | Fast, cheap research / lookup                                                       | speed (0.25) + cheapness (cost exp 0.70); context (0.20), tooluse (0.20), coding (0.15) |
| explorer     | Fast, cheap codebase recon                                                          | speed (0.40) dominant; coding (0.20), tooluse (0.20); cheap (exp 0.70)                  |
| designer     | UI/UX design & visual polish                                                        | visual (0.45) dominant; coding (0.20), reasoning (0.10), multimodal (0.10)              |
| fixer        | Reliable, efficient coding execution                                                | coding (0.45) dominant; instruction (0.15), reasoning (0.15), context (0.10)            |

# How modelmaxx chooses models per role

Role specific capability benchmarks -> cost (including variant recommendations) -> value.

Take the following benchmarks for the "Oracle" agent. The role is a deep thinking reviewer and
architect that requires lots of reasoning, a large context plus decent coding, understanding of
instructions and a little tool use.

Picking a model from your head, you might say GPT Sol right? That's what the marketing says at
least!

```
ID                           COD  REAS★  CTX TOOL INST COST  V_COST  BENCH  VALUE
──────────────────────────────────────────────────────────────────────────────────
github-copilot/gpt-5-6-luna   75   80   100   78   80    0.80    1.44   83.2  80.17
opencode/gpt-5-6-luna         75   80   100   78   80    0.80    1.44   83.2  80.17
opencode/claude-opus-4-8      88   94   100   92   90   17.00   30.60   93.7  66.55
github-copilot/gpt-5-6-sol    82   82   100   78   80    6.80   12.24   85.1  66.27
```

According to this output, opus 4-8 wins right? It has a better score on almost every metric and our
weighting system (BENCH) gives it the highest score for the attributes it needs. It even beats GPT
Sol on the metrics that matter.i

But once you account for price? You actually get the best value by using luna! Even though its about
10% lower on our benchmark, when you consider bang-for-buck, an 9x markup for sol or a ~18x markup
for opus just doesnt make sense. Sol gives an excellent performance for a fraction of the price.

And given a choice between luna and sol? There just 2 points difference in the benchmark yet a 900%
price premium for sol.

THAT's what this tool does! Eliminate the guesswork and maxx's your model selection instead of your
tokens!

# The cost factor

After calculating weighting of capabilties, we also consider how pricing will work for that agent.

- what level of effort should it use?
- will it consume more input tokens or output tokens?
- how important will the "lag" and reliability of typically over-loaded free offerings be?

So for each role we figure out an estimation of the ratio of input/output tokens based on its role.

| Role | Input / Output token split | Cost exponent | Why |
|------|---------------------------|--------------|-----|
| orchestrator | 50% / 50% | 0.15 | balanced planning and responses |
| oracle | 40% / 60% | 0.10 | long reasoning outputs dominate the cost |
| council | 60% / 40% | 0.10 | reads many perspectives (input-heavy), shorter synthesis |
| librarian | 70% / 30% | 0.70 | ingests docs/queries, returns short answers |
| explorer | 70% / 30% | 0.70 | ingests codebases, returns short findings |
| designer | 20% / 80% | 0.05 | small prompts, large generated UI/code |
| fixer | 15% / 85% | 0.30 | small task briefs, large code output |

Higher exponent → cost weighs more in the value score, so free/cheap models win (see Value formula below).

Of course - if the tool was really dumb, free models would always win. However they tend to be
"inconsistent" in availability/performance. So we additionally balance this with some exponents that
try to balance when its worth paying for a premium model because of it's excellence versus lifting
out cheaper options where the real world experience is likely to perform extremely similarly.

`modelmaxx` is a small Go CLI that recommends and configures the best opencode model preset for each
OMO-Slim agent role, based on a role-specific, multi-metric "coding bang for buck" score. It weights
capability metrics per role, applies a free-model penalty, prices each model by its real $/1M token
cost (and the cost impact of the role's recommended reasoning variant), and ranks by **value**
(capability per dollar).

## Why

OMO-Slim splits work across 7 specialist agents (orchestrator, oracle, council, librarian, explorer,
designer, fixer). Each role needs a different capability profile — an oracle needs raw reasoning; an
explorer needs speed and cheapness; a designer needs visual judgment. A single "best model" is
wrong. `modelmaxx` scores every candidate model against each role's profile and picks the best
value.

## How models are ranked & rated per role

### Metrics

Every model is rated on 8 capability metrics (0–100): `coding`, `visual`, `reasoning`, `speed`,
`context`, `tooluse`, `instruction`, `multimodal`.

### Role weights

Each role has a weight vector over the 8 metrics (summing to 1.0), derived from the OMO-Slim "Model
Guidance" for that agent. Example: `oracle` is reasoning-dominant (0.45), `explorer` is
speed-dominant (0.40), `designer` is visual-dominant (0.45).

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

`variantMult` reflects the role's recommended reasoning effort: low/none = 1.0, medium = 1.3, high =
1.8. Free models are floored at `freeCostFloor = 0.5` (regardless of variant — they cost $0; the
floor captures reliability risk).

### Value (the ranking score)

```
value(role, m) = bench(role, m) / cost(role, m) ^ roleCostWeight
```

`roleCostWeight` is a per-role exponent. High exponent (0.70 for librarian/explorer) → cost matters
a lot, cheap/free models win. Low exponent (0.05–0.15 for thinking/design roles) → capability
dominates, paid models win.

### Ranking

Models are ranked by `value` descending per role. There is no hard merit gate — ranking is pure
value (a previous `minBench` floor was removed as a crude crutch that contradicted the weights).

### Design principle

The goal is **not** to pick paid models for their own sake. When two models are within ~1–2 bench
points (negligible quality difference), the cheaper/free model should win. This is a future
refinement (a "free wins on near-tie" rule) beyond the current exponent tuning.

## Current recommendations

| Role         | Model           | Notes             |
| ------------ | --------------- | ----------------- |
| orchestrator | gpt-5-6-luna    | generalist        |
| oracle       | gpt-5-6-luna    | reasoning         |
| council      | gpt-5-6-luna    | synthesis         |
| librarian    | gpt-5-6-luna    | cheap + fast      |
| explorer     | gpt-5-6-luna    | cheap + fast      |
| designer     | claude-opus-4-8 | visual divergence |
| fixer        | gpt-5-6-luna    | coding-dominant   |

(6/7 pick the luna generalist; designer diverges on visual capability.)

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

`models.json` holds the candidate models and their metrics (46 entries). Prices and contexts are
refreshed from [models.dev](https://models.dev) on each run unless `--no-fetch`.

# FAQ

## Why does copilot rank lower than others?

You may notice that sometimes the same model (e.g. GPT Terra) has different scores for different
providers. Often a model from copilot is ranking significantly lower than the same model from
another provider such as opencode-zen.

This is because copilot often reduces context size (presumably for cost saving). Other providers do
not do this! So if the context window size is an important metric for a specific role, then those
with the original, larger context window will of course win!

## License

MIT — see [LICENSE](LICENSE).
