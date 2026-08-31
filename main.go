package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ---- color / formatting ----
const (
	cReset   = "\033[0m"
	cBold    = "\033[1m"
	cDim     = "\033[2m"
	cRed     = "\033[31m"
	cGreen   = "\033[32m"
	cYellow  = "\033[33m"
	cBlue    = "\033[34m"
	cMagenta = "\033[35m"
	cCyan    = "\033[36m"
	cGray    = "\033[90m"
)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

var useColor = detectColor()

func detectColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func paint(code, s string) string {
	if !useColor {
		return s
	}
	return code + s + cReset
}

func vlen(s string) int { return len([]rune(ansiRe.ReplaceAllString(s, ""))) }

func pad(s string, w int) string {
	n := vlen(s)
	if n >= w {
		return s
	}
	return strings.Repeat(" ", w-n) + s
}

func lpad(s string, w int) string {
	n := vlen(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func valueColor(v float64) string {
	switch {
	case v >= 70:
		return cGreen
	case v >= 50:
		return cYellow
	default:
		return cRed
	}
}

type Metrics struct {
	Coding      float64 `json:"coding"`
	Visual      float64 `json:"visual"`
	Reasoning   float64 `json:"reasoning"`
	Speed       float64 `json:"speed"`
	Context     float64 `json:"context"`
	ToolUse     float64 `json:"tooluse"`
	Instruction float64 `json:"instruction"`
	Multimodal  float64 `json:"multimodal"`
	Source      string  `json:"source"`
}

type Model struct {
	Id          string  `json:"id"`
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	Free        bool    `json:"free"`
	PriceIn     float64 `json:"priceIn"`
	PriceOut    float64 `json:"priceOut"`
	PriceSource string  `json:"priceSource"`
	Metrics     Metrics `json:"metrics"`
}

type Dataset struct {
	Version int     `json:"version"`
	Updated string  `json:"updated"`
	Note    string  `json:"note"`
	Models  []Model `json:"models"`
}

// RoleConfig holds all tunable parameters for a single role.
type RoleConfig struct {
	Weights    map[string]float64 `yaml:"weights"`
	CostSplit  CostSplit          `yaml:"costSplit"`
	CostWeight float64            `yaml:"costWeight"`
	Variant    string             `yaml:"variant"`
}

// CostSplit defines input/output token cost weighting.
type CostSplit struct {
	Input  float64 `yaml:"input"`
	Output float64 `yaml:"output"`
}

// Config holds all tunable parameters for modelmaxx.
type Config struct {
	Version        int                   `yaml:"version"`
	Roles          map[string]RoleConfig `yaml:"roles"`
	FreePenalty    float64               `yaml:"freePenalty"`
	FreeCostFloor  float64               `yaml:"freeCostFloor"`
	VariantCostMult map[string]float64   `yaml:"variantCostMult"`
}

const currentConfigVersion = 1

var roles = []string{"orchestrator", "oracle", "council", "librarian", "explorer", "designer", "fixer"}

// defaultConfigPath returns the XDG config file path for modelmaxx.
func defaultConfigPath() string {
	home, _ := os.UserHomeDir()
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "modelmaxx", "config.yaml")
}

// loadConfig reads the config file from the XDG path.
// Returns nil if the file doesn't exist.
func loadConfig() *Config {
	path := defaultConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to parse config %s: %v\n", path, err)
		return nil
	}
	// Version check and migration
	if cfg.Version < currentConfigVersion {
		fmt.Fprintf(os.Stderr, paint(cYellow, "warning: Config version %d is outdated (current: %d). Run 'modelmaxx config --overwrite' to regenerate.\n"), cfg.Version, currentConfigVersion)
		// migrateConfig(&cfg) // Uncomment when migrations are implemented
	} else if cfg.Version > currentConfigVersion {
		fmt.Fprintf(os.Stderr, paint(cYellow, "warning: Config version %d is newer than supported (current: %d). Some features may not work.\n"), cfg.Version, currentConfigVersion)
	}
	return &cfg
}

// migrateConfig is a placeholder for future config migrations (v1 -> v2, etc.).
// Currently a no-op; implement version-specific transformations here when needed.
func migrateConfig(cfg *Config) {
	// Example future migration:
	// if cfg.Version == 1 {
	//     // Transform v1 fields to v2
	//     cfg.Version = 2
	// }
	_ = cfg
}

// generateDefaultConfig returns a Config with all current hardcoded defaults.
func generateDefaultConfig() *Config {
	return &Config{
		Version: currentConfigVersion,
		Roles: map[string]RoleConfig{
			"orchestrator": {
				Weights: map[string]float64{
					"coding": 0.05, "visual": 0, "reasoning": 0.30, "speed": 0,
					"context": 0.15, "tooluse": 0.10, "instruction": 0.30, "multimodal": 0.10,
				},
				CostSplit:  CostSplit{Input: 0.50, Output: 0.50},
				CostWeight: 0.15,
				Variant:    "medium",
			},
			"oracle": {
				Weights: map[string]float64{
					"coding": 0.15, "visual": 0, "reasoning": 0.45, "speed": 0,
					"context": 0.20, "tooluse": 0.05, "instruction": 0.15, "multimodal": 0,
				},
				CostSplit:  CostSplit{Input: 0.40, Output: 0.60},
				CostWeight: 0.10,
				Variant:    "high",
			},
			"council": {
				Weights: map[string]float64{
					"coding": 0.15, "visual": 0, "reasoning": 0.40, "speed": 0,
					"context": 0.20, "tooluse": 0, "instruction": 0.25, "multimodal": 0,
				},
				CostSplit:  CostSplit{Input: 0.60, Output: 0.40},
				CostWeight: 0.10,
				Variant:    "high",
			},
			"librarian": {
				Weights: map[string]float64{
					"coding": 0.15, "visual": 0, "reasoning": 0.10, "speed": 0.25,
					"context": 0.20, "tooluse": 0.20, "instruction": 0.05, "multimodal": 0.05,
				},
				CostSplit:  CostSplit{Input: 0.70, Output: 0.30},
				CostWeight: 0.70,
				Variant:    "low",
			},
			"explorer": {
				Weights: map[string]float64{
					"coding": 0.20, "visual": 0, "reasoning": 0.10, "speed": 0.40,
					"context": 0.05, "tooluse": 0.20, "instruction": 0.05, "multimodal": 0,
				},
				CostSplit:  CostSplit{Input: 0.70, Output: 0.30},
				CostWeight: 0.70,
				Variant:    "low",
			},
			"designer": {
				Weights: map[string]float64{
					"coding": 0.20, "visual": 0.45, "reasoning": 0.10, "speed": 0,
					"context": 0.05, "tooluse": 0.05, "instruction": 0.05, "multimodal": 0.10,
				},
				CostSplit:  CostSplit{Input: 0.20, Output: 0.80},
				CostWeight: 0.05,
				Variant:    "high",
			},
			"fixer": {
				Weights: map[string]float64{
					"coding": 0.45, "visual": 0, "reasoning": 0.15, "speed": 0.10,
					"context": 0.10, "tooluse": 0.05, "instruction": 0.15, "multimodal": 0,
				},
				CostSplit:  CostSplit{Input: 0.15, Output: 0.85},
				CostWeight: 0.30,
				Variant:    "medium",
			},
		},
		FreePenalty:   0.7,
		FreeCostFloor: 0.5,
		VariantCostMult: map[string]float64{
			"none":   1.0,
			"low":    1.0,
			"medium": 1.3,
			"high":   1.8,
		},
	}
}

// writeConfig writes the config to the given path.
// If overwrite is false and the file exists, it returns an error.
func writeConfig(path string, cfg *Config, overwrite bool) error {
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config file already exists at %s (use --overwrite to replace)", path)
		}
	}
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %v", err)
	}
	return nil
}

// applyConfig populates the package-level variables from the loaded config.
func applyConfig(cfg *Config) {
	if cfg == nil {
		return
	}
	if cfg.Roles != nil {
		for role, rc := range cfg.Roles {
			if rc.Weights != nil {
				roleWeights[role] = rc.Weights
			}
			roleCostSplit[role] = [2]float64{rc.CostSplit.Input, rc.CostSplit.Output}
			roleCostWeight[role] = rc.CostWeight
			if rc.Variant != "" {
				roleVariant[role] = rc.Variant
			}
		}
	}
	if cfg.FreePenalty != 0 {
		freePenalty = cfg.FreePenalty
	}
	if cfg.FreeCostFloor != 0 {
		freeCostFloor = cfg.FreeCostFloor
	}
	if cfg.VariantCostMult != nil {
		variantCostMult = cfg.VariantCostMult
	}
}

// freePenalty is the merit penalty applied to free models (default 0.7).
// This replaces the hardcoded 0.7 in b2f().
var freePenalty = 0.7

func loadDataset() Dataset {
	data, err := os.ReadFile("models.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "models.json: %v\n", err)
		os.Exit(1)
	}
	var ds Dataset
	if err := json.Unmarshal(data, &ds); err != nil {
		fmt.Fprintf(os.Stderr, "models.json parse: %v\n", err)
		os.Exit(1)
	}
	return ds
}

func loadModels() []Model { return loadDataset().Models }

// freeCostFloor represents the hidden cost of "free" models (rate limits,
// lower reliability, weaker performance) as an equivalent $/1M. It keeps free
// models comparable to paid ones and stops them winning purely on $0 cost.
// Tunable: lower => free wins more often; higher => paid wins more often.
var freeCostFloor = 0.5

// variantCostMult estimates the token-cost multiplier of each reasoning variant
// (the "effort" knob). Higher variants emit more reasoning tokens, so they cost
// more per request. Applied to paid models in effCost. Tunable.
var variantCostMult = map[string]float64{
	"none":   1.0,
	"low":    1.0,
	"medium": 1.3,
	"high":   1.8,
}

// roleCostSplit defines the input/output token-cost weighting per role. Coder-type
// roles (fixer/designer) generate large outputs from small prompts, so output cost
// dominates; reader-type roles (librarian/explorer) ingest large contexts and emit
// short answers, so input cost dominates. Feeds effCost; only affects paid models
// (free models are floored at freeCostFloor). Tunable.
var roleCostSplit = map[string][2]float64{
	"orchestrator": {0.50, 0.50},
	"oracle":       {0.40, 0.60},
	"council":      {0.60, 0.40},
	"librarian":    {0.70, 0.30},
	"explorer":     {0.70, 0.30},
	"designer":     {0.20, 0.80},
	"fixer":        {0.15, 0.85},
}

func cost(role string, m Model) float64 {
	s := [2]float64{0.3, 0.7} // default global split if role unspecified
	if v, ok := roleCostSplit[role]; ok {
		s = v
	}
	return s[0]*m.PriceIn + s[1]*m.PriceOut
}
func b2f(b bool) float64 {
	if b {
		return freePenalty
	}
	return 1.0
}

// effCost is the effective $/1M cost for a model in a given role. Paid models are
// multiplied by the cost impact of the role's recommended variant (high variant =>
// more reasoning tokens => higher cost). Free models stay floored at freeCostFloor
// regardless of variant (they cost $0; the floor already captures reliability risk).
func effCost(role string, m Model) float64 {
	if m.Free {
		return maxf(cost(role, m), freeCostFloor)
	}
	mult := 1.0
	if v, ok := variantCostMult[roleVariant[role]]; ok {
		mult = v
	}
	return cost(role, m) * mult
}

// roleWeights defines how much each metric matters per agent role, derived from
// the OMO-slim "Model Guidance" for each agent:
//   - orchestrator: planning/judgment > throughput => reasoning + coding, low speed
//   - oracle:       strongest high-reasoning        => reasoning dominant
//   - council:      strong synthesis model          => reasoning high, context for perspectives
//   - librarian:    fast, low-cost, speed>reasoning => speed dominant
//   - explorer:     fast, low-cost, speed>reasoning => speed dominant
//   - designer:     UI/UX judgment + visual polish  => visual dominant, coding + reasoning
//   - fixer:        reliable coding, efficient exec => coding dominant, speed secondary
// Weights sum to 1.0.
var roleWeights = map[string]map[string]float64{
	"orchestrator": {"coding": 0.05, "visual": 0, "reasoning": 0.30, "speed": 0, "context": 0.15, "tooluse": 0.10, "instruction": 0.30, "multimodal": 0.10},
	"oracle":       {"coding": 0.15, "visual": 0, "reasoning": 0.45, "speed": 0, "context": 0.20, "tooluse": 0.05, "instruction": 0.15, "multimodal": 0},
	"council":      {"coding": 0.15, "visual": 0, "reasoning": 0.40, "speed": 0, "context": 0.20, "tooluse": 0, "instruction": 0.25, "multimodal": 0},
	"librarian":    {"coding": 0.15, "visual": 0, "reasoning": 0.10, "speed": 0.25, "context": 0.20, "tooluse": 0.20, "instruction": 0.05, "multimodal": 0.05},
	"explorer":     {"coding": 0.20, "visual": 0, "reasoning": 0.10, "speed": 0.40, "context": 0.05, "tooluse": 0.20, "instruction": 0.05, "multimodal": 0},
	"designer":     {"coding": 0.20, "visual": 0.45, "reasoning": 0.10, "speed": 0, "context": 0.05, "tooluse": 0.05, "instruction": 0.05, "multimodal": 0.10},
	"fixer":        {"coding": 0.45, "visual": 0, "reasoning": 0.15, "speed": 0.10, "context": 0.10, "tooluse": 0.05, "instruction": 0.15, "multimodal": 0},
}

// bench is the role-specific merit rating (0-100) for a model: a weighted blend
// of the five capability metrics (NO cost), with the free-model penalty applied.
// With role=="" it averages all five metrics equally (the unspecialised rating).
func bench(role string, m Model) float64 {
	var w map[string]float64
	if role == "" {
		w = map[string]float64{"coding": 0.125, "visual": 0.125, "reasoning": 0.125, "speed": 0.125, "context": 0.125, "tooluse": 0.125, "instruction": 0.125, "multimodal": 0.125}
	} else {
		w = roleWeights[role]
	}
	raw := m.Metrics.Coding*w["coding"] +
		m.Metrics.Visual*w["visual"] +
		m.Metrics.Reasoning*w["reasoning"] +
		m.Metrics.Speed*w["speed"] +
		m.Metrics.Context*w["context"] +
		m.Metrics.ToolUse*w["tooluse"] +
		m.Metrics.Instruction*w["instruction"] +
		m.Metrics.Multimodal*w["multimodal"]
	return raw * b2f(m.Free)
}

// roleCostWeight is the exponent applied to cost in the value formula
// (value = bench / cost^roleCostWeight). A higher exponent means the role cares
// more about cost: cheaper models gain more, so free models win there. Execution/
// research roles use a high exponent (0.70); thinking/design roles use a low one
// (0.10-0.15) so capability dominates and paid models win. This is what stops free
// models from "always winning" while keeping them in the running where their lower
// performance is acceptable.
var roleCostWeight = map[string]float64{
	"orchestrator": 0.15,
	"oracle":       0.10,
	"council":      0.10,
	"librarian":    0.70,
	"explorer":     0.70,
	"designer":     0.05,
	"fixer":        0.30,
}

// costbias scales every role's cost exponent. Default 1.0 leaves the tuned
// exponents unchanged. >1 makes cost matter more (free/cheap models win); <1
// makes cost matter less (capability/paid models win); 0 ranks by pure bench.
var costbias = 1.0

// roleVariant is the recommended reasoning variant (effort) per role, derived from
// the OMO-Slim Model Guidance docs (NOT the user's current config values):
//   - orchestrator: "Medium reasoning is enough"        => medium
//   - oracle:       "High reasoning justified"          => high
//   - council:      synthesis / high-stakes decisions    => high
//   - librarian:    "Mini model is fine"                 => low
//   - explorer:     "Mini model is appropriate"          => low
//   - designer:     UI/UX judgment + visual polish        => high
//   - fixer:        Medium reasoning for coding tasks     => medium
var roleVariant = map[string]string{
	"orchestrator": "medium",
	"oracle":       "high",
	"council":      "high",
	"librarian":    "low",
	"explorer":     "low",
	"designer":     "high",
	"fixer":        "medium",
}

// value is the role-specific bang-for-buck: merit (bench) divided by cost raised
// to the role's cost-weight exponent. A higher exponent means the role cares more
// about cost (cheaper models gain more). With role=="" the exponent is 1.0 (plain
// bench/cost ratio).
func value(role string, m Model) float64 {
	c := effCost(role, m)
	if c <= 0 {
		c = 0.01
	}
	var exp float64
	if role == "" {
		exp = 1.0
	} else {
		exp = roleCostWeight[role] * costbias
	}
	return bench(role, m) / math.Pow(c, exp)
}

// metricContrib is a metric's weighted contribution to a role's score.
type metricContrib struct {
	name   string
	val    float64
	weight float64
}

// topMetrics returns the metrics driving a role's score for model m, ordered by
// their weighted contribution (weight × metric value), so the user can see WHY
// a model was picked.
func topMetrics(role string, m Model) []metricContrib {
	w := roleWeights[role]
	get := map[string]float64{
		"coding":      m.Metrics.Coding,
		"visual":      m.Metrics.Visual,
		"reasoning":   m.Metrics.Reasoning,
		"speed":       m.Metrics.Speed,
		"context":     m.Metrics.Context,
		"tooluse":     m.Metrics.ToolUse,
		"instruction": m.Metrics.Instruction,
		"multimodal":  m.Metrics.Multimodal,
	}
	var ks []metricContrib
	for _, n := range []string{"coding", "visual", "reasoning", "speed", "context", "tooluse", "instruction", "multimodal"} {
		if w[n] <= 0 {
			continue
		}
		ks = append(ks, metricContrib{n, get[n], w[n]})
	}
	sort.Slice(ks, func(i, j int) bool {
		return ks[i].val*ks[i].weight > ks[j].val*ks[j].weight
	})
	if len(ks) > 3 {
		ks = ks[:3]
	}
	return ks
}

// metricColor maps a metric name to its display color.
var metricColor = map[string]string{
	"coding":    cCyan,
	"visual":    cMagenta,
	"reasoning": cYellow,
	"speed":     cGreen,
	"context":   cBlue,
}

// formatDrivers renders the top contributing metrics as a colored "why" string.
func formatDrivers(role string, m Model) string {
	cs := topMetrics(role, m)
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		name := paint(metricColor[c.name], metricShort[c.name])
		val := paint(cDim, fmt.Sprintf("%d×%.2f", int(c.val), c.weight))
		parts = append(parts, name+" "+val)
	}
	return strings.Join(parts, paint(cDim, " · "))
}

// formatRoleWeights renders a role's metric weights (sorted desc, non-zero only)
// as a colored legend so the user can see which metrics the role cares about most.
func formatRoleWeights(role string) string {
	w := roleWeights[role]
	type wd struct{ name string; weight float64 }
	var ws []wd
	for _, n := range []string{"coding", "visual", "reasoning", "speed", "context", "tooluse", "instruction", "multimodal"} {
		if w[n] > 0 {
			ws = append(ws, wd{n, w[n]})
		}
	}
	sort.Slice(ws, func(i, j int) bool { return ws[i].weight > ws[j].weight })
	parts := make([]string, 0, len(ws))
	for _, x := range ws {
		name := paint(metricColor[x.name], x.name)
		pct := paint(cDim, fmt.Sprintf("%d%%", int(x.weight*100+0.5)))
		parts = append(parts, name+" "+pct)
	}
	return strings.Join(parts, paint(cDim, " · "))
}

// formatCostSplit renders a role's input/output cost weighting and cost exponent as a
// colored legend, so the user can see how token cost is weighted for that role.
func formatCostSplit(role string) string {
	s := [2]float64{0.3, 0.7}
	if v, ok := roleCostSplit[role]; ok {
		s = v
	}
	exp := roleCostWeight[role] * costbias
	if exp == 0 {
		exp = 1.0
	}
	in := paint(cBlue, "in "+fmt.Sprintf("%d%%", int(s[0]*100+0.5)))
	out := paint(cGreen, "out "+fmt.Sprintf("%d%%", int(s[1]*100+0.5)))
	ex := paint(cDim, "exp "+fmt.Sprintf("%.2f", exp))
	return in + paint(cDim, " · ") + out + paint(cDim, " · ") + ex
}

// boxedHeader builds a TUI-style boxed header for list/recommend commands.
// Returns lines ready to print.
func boxedHeader(title, scope, note string, role string) []string {
	const boxWidth = 72
	innerW := boxWidth - 2 // account for "│" on each side

	var lines []string
	// Top border
	lines = append(lines, paint(cDim, "┌"+strings.Repeat("─", boxWidth-2)+"┐"))
	// Title line - simplified
	var titleLine string
	if role != "" {
		titleLine = fmt.Sprintf(" %s %s  Role: %s", paint(cBold, paint(cCyan, title)), paint(cDim, scope), role)
	} else {
		titleLine = fmt.Sprintf(" %s %s", paint(cBold, paint(cCyan, title)), paint(cDim, scope))
	}
	lines = append(lines, paint(cDim, "│")+lpad(titleLine, innerW)+paint(cDim, "│"))

	if role != "" {
		// Separator
		lines = append(lines, paint(cDim, "├"+strings.Repeat("─", boxWidth-2)+"┤"))
		// Role weights section
		lines = append(lines, paint(cDim, "│")+paint(cBold, lpad(" Role Weights", innerW))+paint(cDim, "│"))
		weights := formatRoleWeightsBoxed(role, innerW)
		lines = append(lines, weights...)
		// Separator
		lines = append(lines, paint(cDim, "├"+strings.Repeat("─", boxWidth-2)+"┤"))
		// Cost split line
		costLine := " Cost Split: " + formatCostSplit(role)
		lines = append(lines, paint(cDim, "│")+lpad(costLine, innerW)+paint(cDim, "│"))
	}
	// Bottom border
	lines = append(lines, paint(cDim, "└"+strings.Repeat("─", boxWidth-2)+"┘"))
	return lines
}

// formatRoleWeightsBoxed renders role weights in a 2-column grid inside the box.
func formatRoleWeightsBoxed(role string, innerW int) []string {
	w := roleWeights[role]
	type wd struct{ name string; weight float64 }
	var ws []wd
	for _, n := range []string{"coding", "visual", "reasoning", "speed", "context", "tooluse", "instruction", "multimodal"} {
		if w[n] > 0 {
			ws = append(ws, wd{n, w[n]})
		}
	}
	sort.Slice(ws, func(i, j int) bool { return ws[i].weight > ws[j].weight })

	var lines []string
	colW := (innerW - 4) / 2 // 2 cols, 2 spaces between, 2 spaces padding each side
	for i := 0; i < len(ws); i += 2 {
		left := ""
		right := ""
		if i < len(ws) {
			name := paint(metricColor[ws[i].name], ws[i].name)
			pct := paint(cDim, fmt.Sprintf("%d%%", int(ws[i].weight*100+0.5)))
			left = fmt.Sprintf("  %s %s", name, pct)
		}
		if i+1 < len(ws) {
			name := paint(metricColor[ws[i+1].name], ws[i+1].name)
			pct := paint(cDim, fmt.Sprintf("%d%%", int(ws[i+1].weight*100+0.5)))
			right = fmt.Sprintf("  %s %s", name, pct)
		}
		line := left + strings.Repeat(" ", max(0, colW-vlen(left))) + "  " + right
		lines = append(lines, paint(cDim, "│")+lpad(line, innerW)+paint(cDim, "│"))
	}
	return lines
}

// ctxScore normalizes a context-window size (tokens) to a 0-100 score.
func ctxScore(w float64) float64 {
	if w <= 0 {
		return 50.0
	}
	s := w / 10000.0
	if s > 100 {
		return 100
	}
	return s
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// providerPrefix maps a provider selector to the model id prefix.
func providerPrefix(p string) string {
	if p == "copilot" {
		return "github-copilot/"
	}
	return "opencode/" // opencode (alias zen)
}

// presetProvider maps a preset block name to the provider scope used when picking
// its models. "free" => opencode (only free-backed provider), "copilot" => github-copilot,
// any other name (e.g. "mixed") => cross-provider.
func presetProvider(preset string) string {
	switch preset {
	case "free":
		return "opencode"
	case "copilot":
		return "copilot"
	default:
		return ""
	}
}

// targetPreset maps a provider selector to the OMO Slim preset block to write.
func targetPreset(p string) string {
	if p == "copilot" {
		return "copilot"
	}
	return "free" // opencode/zen writes the opencode-backed "free" preset block
}

// filterModels returns models for the provider, optionally restricted to free ones.
// provider "" or "all" returns models from every provider (cross-provider assessment).
func filterModels(provider string, freeOnly bool) []Model {
	prefix := ""
	if provider != "" && provider != "all" {
		prefix = providerPrefix(provider)
	}
	var out []Model
	for _, m := range loadModels() {
		if prefix != "" && !strings.HasPrefix(m.Id, prefix) {
			continue
		}
		if freeOnly && !m.Free {
			continue
		}
		out = append(out, m)
	}
	return out
}

func recommend(provider string, freeOnly bool) map[string]Model {
	inPreset := filterModels(provider, freeOnly)
	out := map[string]Model{}
	if len(inPreset) == 0 {
		return out
	}
	for _, role := range roles {
		best := inPreset[0]
		bestV := value(role, best)
		for _, m := range inPreset[1:] {
			if v := value(role, m); v > bestV {
				bestV = v
				best = m
			}
		}
		out[role] = best
	}
	return out
}

// metricShort maps a metric key to its column header label.
var metricShort = map[string]string{
	"coding":      "COD",
	"visual":      "VIS",
	"reasoning":   "REAS",
	"speed":       "SPD",
	"context":     "CTX",
	"tooluse":     "TOOL",
	"instruction": "INST",
	"multimodal":  "MULT",
}

// metricVal returns the formatted capability value (0–100 integer) for a model/metric.
func metricVal(m Model, name string) string {
	switch name {
	case "coding":
		return fmt.Sprintf("%d", int(m.Metrics.Coding))
	case "visual":
		return fmt.Sprintf("%d", int(m.Metrics.Visual))
	case "reasoning":
		return fmt.Sprintf("%d", int(m.Metrics.Reasoning))
	case "speed":
		return fmt.Sprintf("%d", int(m.Metrics.Speed))
	case "context":
		return fmt.Sprintf("%d", int(m.Metrics.Context))
	case "tooluse":
		return fmt.Sprintf("%d", int(m.Metrics.ToolUse))
	case "instruction":
		return fmt.Sprintf("%d", int(m.Metrics.Instruction))
	case "multimodal":
		return fmt.Sprintf("%d", int(m.Metrics.Multimodal))
	}
	return ""
}

// metricHead renders a metric column header for a role. Metrics the role weights are
// shown bold; unused metrics (weight 0) are dimmed; the dominant (highest-weight)
// metric gets a ★. With role=="" every metric is treated as active (used by the
// cross-role recommend view, where each row is a different role).
func metricHead(role, name string) string {
	w := 0.2
	if role != "" {
		w = roleWeights[role][name]
	}
	label := metricShort[name]
	if w <= 0 {
		return paint(cDim, label)
	}
	s := paint(cBold, label)
	if role != "" && isDominant(role, name) {
		s += paint(cYellow, "★")
	}
	return s
}

func isDominant(role, name string) bool {
	metrics := []string{"coding", "visual", "reasoning", "speed", "context", "tooluse", "instruction", "multimodal"}
	max := -1.0
	dom := ""
	for _, m := range metrics {
		if roleWeights[role][m] > max {
			max = roleWeights[role][m]
			dom = m
		}
	}
	return name == dom && max > 0
}

// legendLine returns a multi-line explanation of the column abbreviations used in
// the list and recommend tables — one line per abbreviation with a short description.
func legendLine() string {
	type item struct{ abbr, name, desc string }
	items := []item{
		{"COD", "coding", "raw coding/benchmark performance (SWE-bench style)"},
		{"VIS", "visual", "UI / visual design capability"},
		{"REAS", "reasoning", "complex problem-solving & architecture"},
		{"SPD", "speed", "latency / throughput (cheap, fast models)"},
		{"CTX", "context", "context-window size exposed by the provider"},
		{"TOOL", "tool-use", "reliability of calling tools / functions"},
		{"INST", "instruction", "how precisely it obeys instructions"},
		{"MULT", "multimodal", "image / audio / video understanding"},
		{"COST", "$/1M", "input + output price per 1M tokens"},
		{"V_COST", "eff-cost", "cost of the recommended model variant for the role"},
		{"BENCH", "role-merit", "weighted capability score for the role"},
		{"VALUE", "score", "bench / cost^exp — the ranking score"},
	}
	const wAbbr, wName = 8, 12
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = "  " + paint(cBold, lpad(it.abbr, wAbbr)) + " " +
			paint(cCyan, lpad(it.name, wName)) + " " + paint(cDim, it.desc)
	}
	return strings.Join(lines, "\n")
}

func cmdList(provider string, role string) {
	models := filterModels(provider, false)
	type row struct {
		m Model
		v float64
	}
	var rows []row
	for _, m := range models {
		if role != "" {
			rows = append(rows, row{m, value(role, m)})
		} else {
			rows = append(rows, row{m, value("", m)})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].v > rows[j].v })
	scope := provider
	if scope == "" || scope == "all" {
		scope = "all providers"
	}
	note := "value = unspecialised mean-metrics / cost"
	if role != "" {
		note = "value = role-specific (" + role + ") score"
	}
	for _, line := range boxedHeader("# Models", "for "+scope, note, role) {
		fmt.Println(line)
	}
	fmt.Println()

	// activeMetrics: which capability columns to render. With no role, show all
	// eight; with a role, show only the metrics that role actually weights (>0) to
	// narrow the table.
	allMetrics := []string{"coding", "visual", "reasoning", "speed", "context", "tooluse", "instruction", "multimodal"}
	activeMetrics := allMetrics
	if role != "" {
		activeMetrics = nil
		for _, name := range allMetrics {
			if roleWeights[role][name] > 0 {
				activeMetrics = append(activeMetrics, name)
			}
		}
	}

	type lrow struct {
		val, cost, eff, bench, id, why string
		metric                        map[string]string
	}
	var lrows []lrow
	wVal, wId, wCost, wEff, wBench := 5, 4, 5, 6, 5
	wMetric := map[string]int{}
	for _, name := range activeMetrics {
		w := len(metricShort[name])
		if role != "" && isDominant(role, name) {
			w++
		}
		wMetric[name] = w
	}
	for _, r := range rows {
		m := r.m
		idStr := m.Id
		if m.Free {
			idStr = paint(cYellow, m.Id) + paint(cDim, " (free)")
		} else {
			idStr = paint(cGreen, m.Id)
		}
		valStr := paint(valueColor(r.v), fmt.Sprintf("%.2f", r.v))
		ms := map[string]string{}
		for _, name := range activeMetrics {
			s := metricVal(m, name)
			ms[name] = s
			wMetric[name] = max(wMetric[name], vlen(s))
		}
		costStr := fmt.Sprintf("%.2f", cost(role, m))
		effStr := fmt.Sprintf("%.2f", effCost(role, m))
		benchStr := fmt.Sprintf("%.1f", bench(role, m))
		whyStr := ""
		if role != "" {
			whyStr = formatDrivers(role, m)
		}
		lrows = append(lrows, lrow{valStr, costStr, effStr, benchStr, idStr, whyStr, ms})
		wVal = max(wVal, vlen(valStr))
		wId = max(wId, vlen(idStr))
		wCost = max(wCost, vlen(costStr))
		wBench = max(wBench, vlen(benchStr))
		wEff = max(wEff, vlen(effStr))
	}
	var metricHeads []string
	for _, name := range activeMetrics {
		metricHeads = append(metricHeads, pad(metricHead(role, name), wMetric[name]))
	}
	header := lpad(paint(cBold, "ID"), wId) + "  " + strings.Join(metricHeads, " ") + "  " +
		pad(paint(cBold, "COST"), wCost) + "  " +
		pad(paint(cBold, "V_COST"), wEff) + "  " +
		pad(paint(cBold, "BENCH"), wBench) + "  " +
		pad(paint(cBold, "VALUE"), wVal)
	if role != "" {
		header += "  " + paint(cBold, "WHY")
	}
	fmt.Println(header)
	fmt.Println(paint(cDim, strings.Repeat("─", vlen(header))))
	for _, r := range lrows {
		var metricVals []string
		for _, name := range activeMetrics {
			metricVals = append(metricVals, pad(r.metric[name], wMetric[name]))
		}
		line := lpad(r.id, wId) + "  " + strings.Join(metricVals, " ") + "  " +
			pad(r.cost, wCost) + "  " +
			pad(r.eff, wEff) + "  " +
			pad(r.bench, wBench) + "  " +
			pad(r.val, wVal)
		if role != "" {
			line += "  " + r.why
		}
		fmt.Println(line)
	}
	fmt.Println()
	fmt.Println(legendLine())
}
func cmdRecommend(provider string) {
	rec := recommend(provider, false)
	scope := provider
	if scope == "" || scope == "all" {
		scope = "all providers"
	}
	for _, line := range boxedHeader("# Recommended models", "for "+scope, "cross-role recommendations", "") {
		fmt.Println(line)
	}
	fmt.Println()

	type rrow struct {
		role, model, variant, coding, visual, reasoning, speed, ctx, tooluse, instruction, multimodal, cost, eff, bench, val, why string
	}
	var rows []rrow
	wRole, wModel, wVar, wCod, wVis, wReas, wSpd, wCtx, wTool, wInst, wMult, wCost, wEff, wBench, wVal := 12, 6, 8, 3, 3, 4, 3, 3, 4, 4, 4, 5, 6, 5, 5
	for _, role := range roles {
		m := rec[role]
		if m.Id == "" {
			rows = append(rows, rrow{
				role:  paint(cBold, role),
				model: paint(cRed, "(no models match)"),
			})
			continue
		}
		modelStr := m.Id
		if m.Free {
			modelStr = paint(cYellow, m.Id) + paint(cDim, " (free)")
		} else {
			modelStr = paint(cGreen, m.Id)
		}
		v := value(role, m)
		valStr := paint(valueColor(v), fmt.Sprintf("%.1f", v))
		variantStr := paint(cBlue, roleVariant[role])
		whyStr := formatDrivers(role, m)
		effStr := paint(cDim, fmt.Sprintf("%.2f", effCost(role, m)))
		costStr := fmt.Sprintf("%.2f", cost(role, m))
		benchStr := fmt.Sprintf("%.1f", bench(role, m))
		codingStr := fmt.Sprintf("%d", int(m.Metrics.Coding))
		visualStr := fmt.Sprintf("%d", int(m.Metrics.Visual))
		reasoningStr := fmt.Sprintf("%d", int(m.Metrics.Reasoning))
		speedStr := fmt.Sprintf("%d", int(m.Metrics.Speed))
		ctxStr := fmt.Sprintf("%d", int(m.Metrics.Context))
		tooluseStr := fmt.Sprintf("%d", int(m.Metrics.ToolUse))
		instructionStr := fmt.Sprintf("%d", int(m.Metrics.Instruction))
		multimodalStr := fmt.Sprintf("%d", int(m.Metrics.Multimodal))
		rows = append(rows, rrow{
			role:        paint(cBold, role),
			model:       modelStr,
			variant:     variantStr,
			coding:      codingStr,
			visual:      visualStr,
			reasoning:   reasoningStr,
			speed:       speedStr,
			ctx:         ctxStr,
			tooluse:     tooluseStr,
			instruction: instructionStr,
			multimodal:  multimodalStr,
			cost:        costStr,
			val:         valStr,
			eff:         effStr,
			bench:       benchStr,
			why:         whyStr,
		})
		wRole = max(wRole, vlen(role))
		wModel = max(wModel, vlen(modelStr))
		wVar = max(wVar, vlen(variantStr))
		wCod = max(wCod, vlen(codingStr))
		wVis = max(wVis, vlen(visualStr))
		wReas = max(wReas, vlen(reasoningStr))
		wSpd = max(wSpd, vlen(speedStr))
		wCtx = max(wCtx, vlen(ctxStr))
		wTool = max(wTool, vlen(tooluseStr))
		wInst = max(wInst, vlen(instructionStr))
		wMult = max(wMult, vlen(multimodalStr))
		wCost = max(wCost, vlen(costStr))
		wBench = max(wBench, vlen(benchStr))
		wVal = max(wVal, vlen(valStr))
		wEff = max(wEff, vlen(effStr))
	}
	header := lpad(paint(cBold, "ROLE"), wRole) + "  " +
		lpad(paint(cBold, "MODEL"), wModel) + "  " +
		lpad(paint(cBold, "VARIANT"), wVar) + "  " +
		pad(metricHead("", "coding"), wCod) + " " +
		pad(metricHead("", "visual"), wVis) + " " +
		pad(metricHead("", "reasoning"), wReas) + " " +
		pad(metricHead("", "speed"), wSpd) + " " +
		pad(metricHead("", "context"), wCtx) + " " +
		pad(metricHead("", "tooluse"), wTool) + " " +
		pad(metricHead("", "instruction"), wInst) + " " +
		pad(metricHead("", "multimodal"), wMult) + "  " +
		pad(paint(cBold, "COST"), wCost) + "  " +
		pad(paint(cBold, "V_COST"), wEff) + "  " +
		pad(paint(cBold, "BENCH"), wBench) + "  " +
		pad(paint(cBold, "VALUE"), wVal) + "  " + paint(cBold, "WHY")
	fmt.Println(header)
	fmt.Println(paint(cDim, strings.Repeat("─", vlen(header))))
	for _, r := range rows {
		fmt.Println(lpad(r.role, wRole) + "  " +
			lpad(r.model, wModel) + "  " +
			lpad(r.variant, wVar) + "  " +
			pad(r.coding, wCod) + " " +
			pad(r.visual, wVis) + " " +
			pad(r.reasoning, wReas) + " " +
			pad(r.speed, wSpd) + " " +
			pad(r.ctx, wCtx) + " " +
			pad(r.tooluse, wTool) + " " +
			pad(r.instruction, wInst) + " " +
			pad(r.multimodal, wMult) + "  " +
			pad(r.cost, wCost) + "  " +
			pad(r.eff, wEff) + "  " +
			pad(r.bench, wBench) + "  " +
			pad(r.val, wVal) + "  " +
			r.why)
	}
	fmt.Println()
	fmt.Println(legendLine())
}

// findBlock returns the span of `"<key>": { ... }` (brace-matched) in text.
func findBlock(text, key string) (int, int, bool) {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:\s*\{`)
	m := re.FindStringIndex(text)
	if m == nil {
		return 0, 0, false
	}
	braceStart := m[1] - 1 // the '{'
	depth := 1
	i := braceStart + 1
	for ; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return m[0], i + 1, true
			}
		}
	}
	return 0, 0, false
}

// applyRole rewrites the "model" (and "variant") lines inside a single role block,
// preserving every other line (comments, skills, mcps) and its indentation. If the
// role has no "variant" line, one is inserted directly after the "model" line.
func applyRole(lines []string, role, modelId, variant string) []string {
	startPat := fmt.Sprintf(`"%s": {`, role)
	ri := -1
	for i, l := range lines {
		if strings.Contains(l, startPat) {
			ri = i
			break
		}
	}
	if ri < 0 {
		return lines
	}
	// The role block closes at the first "}," (or "}") after its opening line.
	ci := -1
	for j := ri + 1; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		if t == "}," || t == "}" {
			ci = j
			break
		}
	}
	if ci < 0 {
		return lines
	}
	modelLine, variantLine := -1, -1
	for k := ri + 1; k < ci; k++ {
		t := strings.TrimSpace(lines[k])
		if strings.HasPrefix(t, `"model":`) {
			modelLine = k
		}
		if strings.HasPrefix(t, `"variant":`) {
			variantLine = k
		}
	}
	if modelLine < 0 {
		return lines
	}
	indent := lines[modelLine][:len(lines[modelLine])-len(strings.TrimLeft(lines[modelLine], " "))]
	lines[modelLine] = indent + fmt.Sprintf(`"model": "%s",`, modelId)
	if variantLine >= 0 {
		lines[variantLine] = indent + fmt.Sprintf(`"variant": "%s",`, variant)
	} else {
		ins := indent + fmt.Sprintf(`"variant": "%s",`, variant)
		lines = append(lines[:modelLine+1], append([]string{ins}, lines[modelLine+1:]...)...)
	}
	return lines
}

func cmdApply(provider string, presetName string, configPath string, dryRun bool) {
	cfg := configPath
	if cfg == "" {
		home, _ := os.UserHomeDir()
		cfg = filepath.Join(home, ".config/opencode/oh-my-opencode-slim.jsonc")
	}
	data, err := os.ReadFile(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config not found: %v\n", err)
		os.Exit(1)
	}
	text := string(data)
	// --preset <name> targets a single OMO-Slim preset block (e.g. "mixed", "bob").
	// Without it, update the two built-in blocks ("free" and "copilot") as before.
	targets := []string{}
	if presetName != "" {
		targets = []string{presetName}
	} else {
		targets = []string{"free", "copilot"}
	}
	var applied []string
	for _, preset := range targets {
		// The "free" block must only ever contain free models; force freeOnly there.
		fo := preset == "free"
		// Scope the model search to the natural provider for this block unless the
		// user overrode it with --provider.
		p := provider
		if p == "" {
			p = presetProvider(preset)
		}
		rec := recommend(p, fo)
		start, end, ok := findBlock(text, preset)
		if !ok {
			fmt.Fprintf(os.Stderr, "preset '%s' not found in %s\n", preset, cfg)
			continue
		}
		block := text[start:end]
		lines := strings.Split(block, "\n")
		for _, role := range roles {
			m := rec[role]
			if m.Id == "" {
				continue
			}
			lines = applyRole(lines, role, m.Id, roleVariant[role])
		}
		newBlock := strings.Join(lines, "\n")
		text = text[:start] + newBlock + text[end:]
		applied = append(applied, preset)
	}
	if dryRun {
		fmt.Println(paint(cBold, paint(cYellow, "# dry-run")) +
			paint(cDim, ": would write presets ") +
			paint(cGreen, fmt.Sprint(applied)) +
			paint(cDim, " to ") + paint(cGray, cfg))
		fmt.Println(paint(cDim, strings.Repeat("─", 60)))
		fmt.Println(text)
		return
	}
	if err := os.Rename(cfg, cfg+".bak"); err != nil {
		fmt.Fprintf(os.Stderr, "backup failed: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(cfg, []byte(text), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(paint(cGreen, "Applied presets ") +
		paint(cBold, fmt.Sprint(applied)) +
		paint(cGreen, " in ") + paint(cGray, cfg) +
		paint(cGreen, "  (backup: ") + paint(cGray, cfg+".bak") + paint(cGreen, ")"))
}
// runFetch refreshes prices + context windows from models.dev and writes models.json.
// It returns the counts and any error (without exiting) so callers can warn instead of abort.
func runFetch(provider string) (int, int, error) {
	url := "https://models.dev/api.json"
	resp, err := http.Get(url)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, 0, fmt.Errorf("fetch failed: status %d", resp.StatusCode)
	}
	var api map[string]struct {
		Models map[string]struct {
			Cost struct {
				Input  float64 `json:"input"`
				Output float64 `json:"output"`
			} `json:"cost"`
			Limit struct {
				Context float64 `json:"context"`
			} `json:"limit"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&api); err != nil {
		return 0, 0, fmt.Errorf("parse models.dev api: %v", err)
	}
	var providers []string
	switch provider {
	case "opencode", "zen":
		providers = []string{"opencode"}
	case "copilot":
		providers = []string{"github-copilot"}
	default:
		providers = []string{"opencode", "github-copilot"}
	}
	ds := loadDataset()
	updated := 0
	ctxUpdated := 0
	for _, p := range providers {
		prov, ok := api[p]
		if !ok {
			continue
		}
		for mid, entry := range prov.Models {
			in, out := entry.Cost.Input, entry.Cost.Output
			norm := strings.ReplaceAll(mid, ".", "-")
			id := p + "/" + norm
			for i := range ds.Models {
				if ds.Models[i].Id != id {
					continue
				}
				if in != 0 || out != 0 {
					ds.Models[i].PriceIn = in
					ds.Models[i].PriceOut = out
					ds.Models[i].PriceSource = "models.dev"
					updated++
				}
				if entry.Limit.Context > 0 {
					ds.Models[i].Metrics.Context = ctxScore(entry.Limit.Context)
					ds.Models[i].Metrics.Source = "models.dev(context)+bench"
					ctxUpdated++
				}
			}
		}
	}
	if updated > 0 || ctxUpdated > 0 {
		ds.Updated = time.Now().Format("2006-01-02")
		out, _ := json.MarshalIndent(ds, "", "  ")
		if err := os.WriteFile("models.json", append(out, '\n'), 0644); err != nil {
			return 0, 0, fmt.Errorf("write models.json: %v", err)
		}
	}
	return updated, ctxUpdated, nil
}

func cmdFetch(provider string) {
	u, c, err := runFetch(provider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if u > 0 || c > 0 {
		fmt.Printf("Updated %d prices and %d context windows from models.dev\n", u, c)
	} else {
		fmt.Println("No updates applied (no matching models found).")
	}
}

// autoFetch refreshes pricing before a command unless disabled. It never aborts the
// command on failure - it only warns. Skipped for `fetch` itself and `--no-fetch`.
func autoFetch(noFetch bool, cmd string) {
	if cmd == "fetch" || noFetch {
		return
	}
	if u, c, err := runFetch("all"); err != nil {
		ds := loadDataset()
		fmt.Fprintln(os.Stderr, paint(cRed, fmt.Sprintf("Unable to refresh data, executing based on dataset from %s", ds.Updated)))
		fmt.Fprintln(os.Stderr, "")
	} else if u > 0 || c > 0 {
		fmt.Fprintln(os.Stderr, paint(cGreen, fmt.Sprintf("refreshed %d prices, %d contexts from models.dev", u, c)))
		fmt.Fprintln(os.Stderr, "")
	}
}

func parseCostbias(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --costbias: %s\n", s)
		os.Exit(1)
	}
	return v
}

// cmdConfig generates a default config file at the specified path.
func cmdConfig(configPathFlag string, overwrite bool, dryRun bool) {
	path := configPathFlag
	if path == "" {
		path = defaultConfigPath()
	}
	cfg := generateDefaultConfig()
	if dryRun {
		data, _ := yaml.Marshal(cfg)
		fmt.Println(string(data))
		return
	}
	if err := writeConfig(path, cfg, overwrite); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Created default config at %s\n", path)
}

func main() {
	// Load config early so all commands use configured values
	cfg := loadConfig()
	applyConfig(cfg)

	args := os.Args[1:]
	if len(args) == 0 {
		autoFetch(false, "")
		cmdRecommend("")
		return
	}

	// Check if first arg is a known command
	knownCommands := map[string]bool{"list": true, "recommend": true, "apply": true, "fetch": true, "config": true}
	firstArg := args[0]
	cmd := ""
	startIdx := 1

	if knownCommands[firstArg] {
		cmd = firstArg
	} else if strings.HasPrefix(firstArg, "-") {
		// Default to recommend; treat all args as flags
		cmd = "recommend"
		startIdx = 0
	} else {
		fmt.Fprintf(os.Stderr, "unknown command: %s (list|recommend|apply|fetch|config)\n", firstArg)
		os.Exit(1)
	}

	// Detect multiple commands (e.g., "recommend apply list")
	var allCmds []string
	if cmd != "" {
		allCmds = append(allCmds, cmd)
	}
	for i := startIdx; i < len(args); i++ {
		if knownCommands[args[i]] {
			allCmds = append(allCmds, args[i])
		}
	}
	if len(allCmds) > 1 {
		fmt.Fprintf(os.Stderr, "Multiple commands (%s) not supported\n", strings.Join(allCmds, ", "))
		os.Exit(1)
	}

	// Auto-generate config on first run (except for config command itself)
	if cmd != "config" && cfg == nil {
		defaultCfg := generateDefaultConfig()
		path := defaultConfigPath()
		if err := writeConfig(path, defaultCfg, false); err == nil {
			fmt.Fprintf(os.Stderr, "Created default config at %s\n", path)
			fmt.Fprintln(os.Stderr, "")
			// Reload and apply the newly created config
			cfg = loadConfig()
			applyConfig(cfg)
		}
	}

	provider := ""
	providerSet := false
	role := ""
	presetName := ""
	configPathFlag := ""
	dryRun := false
	noFetch := false
	overwrite := false
	for i := startIdx; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--provider" && i+1 < len(args):
			provider = args[i+1]
			providerSet = true
			i++
		case strings.HasPrefix(a, "--provider="):
			provider = a[len("--provider="):]
			providerSet = true
		case a == "--role" && i+1 < len(args):
			role = args[i+1]
			i++
		case strings.HasPrefix(a, "--role="):
			role = a[len("--role="):]
		case a == "--preset" && i+1 < len(args):
			presetName = args[i+1]
			i++
		case strings.HasPrefix(a, "--preset="):
			presetName = a[len("--preset="):]
		case a == "--costbias" && i+1 < len(args):
			costbias = parseCostbias(args[i+1])
			i++
		case strings.HasPrefix(a, "--costbias="):
			costbias = parseCostbias(a[len("--costbias="):])
		case a == "--no-fetch":
			noFetch = true
		case a == "--config" && i+1 < len(args):
			configPathFlag = args[i+1]
			i++
		case strings.HasPrefix(a, "--config="):
			configPathFlag = a[len("--config="):]
		case a == "--dry-run":
			dryRun = true
		case a == "--path" && i+1 < len(args):
			configPathFlag = args[i+1]
			i++
		case strings.HasPrefix(a, "--path="):
			configPathFlag = a[len("--path="):]
		case a == "--overwrite":
			overwrite = true
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n", a)
				os.Exit(1)
			}
		}
	}
	if provider == "zen" {
		provider = "opencode"
	}
	if provider != "" && provider != "all" && provider != "opencode" && provider != "copilot" {
		fmt.Fprintf(os.Stderr, "unknown provider: %s (opencode|copilot|all)\n", provider)
		os.Exit(1)
	}
	if role != "" {
		if _, ok := roleWeights[role]; !ok {
			fmt.Fprintf(os.Stderr, "unknown role: %s (orchestrator|oracle|council|librarian|explorer|designer|fixer)\n", role)
			os.Exit(1)
		}
	}
	if costbias < 0 {
		fmt.Fprintf(os.Stderr, "costbias must be >= 0 (got %v)\n", costbias)
		os.Exit(1)
	}
	autoFetch(noFetch, cmd)
	switch cmd {
	case "list":
		cmdList(provider, role)
	case "recommend":
		cmdRecommend(provider)
	case "apply":
		cmdApply(provider, presetName, configPathFlag, dryRun)
	case "fetch":
		if !providerSet {
			cmdFetch("all")
		} else {
			cmdFetch(provider)
		}
	case "config":
		cmdConfig(configPathFlag, overwrite, dryRun)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s (list|recommend|apply|fetch|config)\n", cmd)
		os.Exit(1)
	}
}
