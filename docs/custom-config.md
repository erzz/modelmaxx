# Custom Configuration

modelmaxx supports fully custom agent roles via a YAML config file. This is useful if you:

- Use a different agent harness than oh-my-opencode-slim
- Have custom agent names (e.g., `builder`, `reviewer`, `planner`)
- Want to tune weights, cost splits, or reasoning variants per role

## Quick Start

```bash
# Generate default config (shows all options)
modelmaxx config --overwrite

# Or create a custom config from the example
cp tests/custom-config.yaml ~/.config/modelmaxx/config.yaml
```

## Config Structure

```yaml
version: 1
roles:
  builder:
    weights:
      coding: 0.5
      context: 0.1
      instruction: 0.15
      multimodal: 0
      reasoning: 0.15
      speed: 0.05
      tooluse: 0.05
      visual: 0
    costSplit:
      input: 0.2
      output: 0.8
    costWeight: 0.2
    variant: medium
  reviewer:
    weights:
      coding: 0.2
      context: 0.15
      instruction: 0.2
      multimodal: 0
      reasoning: 0.35
      speed: 0
      tooluse: 0.05
      visual: 0
    costSplit:
      input: 0.4
      output: 0.6
    costWeight: 0.1
    variant: high
freePenalty: 0.7
freeCostFloor: 0.5
variantCostMult:
  high: 1.8
  low: 1
  medium: 1.3
  none: 1
```

## Field Reference

| Field | Description | Default |
|-------|-------------|---------|
| `version` | Config schema version (for future migrations) | `1` |
| `roles.<name>.weights` | 8 metric weights (sum to 1.0) | Required per role |
| `roles.<name>.costSplit.input` | Input token cost weight (0–1) | Required per role |
| `roles.<name>.costSplit.output` | Output token cost weight (0–1) | Required per role |
| `roles.<name>.costWeight` | Cost exponent (higher = free/cheap favored) | Required per role |
| `roles.<name>.variant` | Reasoning effort: `low` \| `medium` \| `high` \| `none` | Required per role |
| `freePenalty` | Merit multiplier for free models (0–1) | `0.7` |
| `freeCostFloor` | Minimum effective cost for free models ($/1M) | `0.5` |
| `variantCostMult` | Cost multipliers per variant | See above |

## Example: Builder + Reviewer

The [`tests/custom-config.yaml`](../tests/custom-config.yaml) defines two roles:

- **builder** — Coding-heavy (50% coding), output-heavy cost split (80% output), medium variant
- **reviewer** — Reasoning-heavy (35% reasoning), balanced cost split, high variant

```bash
modelmaxx recommend --no-fetch
```

Output:
```
ROLE      MODEL                     VARIANT   COD VIS REAS SPD CTX TOOL INST MULT   COST  V_COST  BENCH  VALUE  WHY
builder   opencode/gpt-5-6-luna     medium     75  72   80  90 100   78   80   85   1.00    1.30   79.9   75.8  COD 75×0.50 · REAS 80×0.15 · INST 80×0.15
reviewer  opencode/gpt-5-6-luna     high       75  72   80  90 100   78   80   85   0.80    1.44   77.9   75.1  REAS 80×0.35 · INST 80×0.20 · COD 75×0.20
```

## Tips

- **Weights must sum to 1.0** per role (validated at runtime)
- **Cost split must sum to 1.0** (input + output)
- **Variant** affects cost: `low`=1.0x, `medium`=1.3x, `high`=1.8x, `none`=1.0x
- **Cost weight** controls price sensitivity: 0.70 (explorer/librarian) = cost matters a lot; 0.05 (designer) = capability dominates
- Run `modelmaxx config --dry-run` to see the full default schema
- Config is loaded from `$XDG_CONFIG_HOME/modelmaxx/config.yaml` (or `~/.config/modelmaxx/config.yaml`)

## Migration

If you upgrade modelmaxx and the config schema changes, you'll see a warning:

```
warning: Config version 0 is outdated (current: 1). Run 'modelmaxx config --overwrite' to regenerate.
```

Just run `modelmaxx config --overwrite` to get the latest schema with your existing values preserved where possible.