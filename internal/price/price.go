// Package price estimates session cost from lifetime usage and a price table.
//
// No runtime on this box writes USD, so every number here is an ESTIMATE and
// is labelled as one (Overlay.CostSource). The joiner never sees this package;
// prices are applied to overlays off-tick, and an unknown model stays absent.
package price

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

// Price is USD per million tokens. Window is the model's context window in
// tokens (0 = unknown).
type Price struct {
	In         float64 `json:"in"`
	Out        float64 `json:"out"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
	Window     int64   `json:"window,omitempty"`
	Source     string  `json:"source,omitempty"` // where the numbers came from
}

// Table maps normalized model ids to prices.
type Table struct {
	entries map[string]Price
	origin  map[string]string // model -> "builtin" | "user"
}

func New() *Table {
	return &Table{entries: map[string]Price{}, origin: map[string]string{}}
}

// Builtin returns the shipped table. Every entry carries its source; a model
// absent here costs "—" until ~/.config/aitop/prices.json supplies it.
func Builtin() *Table {
	t := New()
	for k, v := range builtin {
		t.entries[Normalize(k)] = v
		t.origin[Normalize(k)] = "builtin"
	}
	return t
}

// UserPath is where the override file lives: $AITOP_PRICES, else
// $XDG_CONFIG_HOME/aitop/prices.json, else ~/.config/aitop/prices.json.
func UserPath() string {
	if p := os.Getenv("AITOP_PRICES"); p != "" {
		return p
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "aitop", "prices.json")
}

// LoadUser merges a JSON object {model: Price} over the table. A missing
// file is not an error; a malformed one is, and the builtin stays intact.
func (t *Table) LoadUser(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var m map[string]Price
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("prices %s: %w", path, err)
	}
	for k, v := range m {
		if v.Source == "" {
			v.Source = "user:" + filepath.Base(path)
		}
		t.entries[Normalize(k)] = v
		t.origin[Normalize(k)] = "user"
	}
	return nil
}

var (
	dateSuffix = regexp.MustCompile(`-\d{8}$`)
	oneM       = regexp.MustCompile(`\[1m\]$`)
)

// Normalize strips provider prefixes, the [1m] context marker, and dated
// suffixes so "anthropic/claude-opus-5[1m]" and "claude-opus-5-20260301"
// both resolve to "claude-opus-5".
func Normalize(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndexByte(m, '/'); i >= 0 {
		m = m[i+1:]
	}
	m = oneM.ReplaceAllString(m, "")
	m = dateSuffix.ReplaceAllString(m, "")
	return m
}

// Lookup finds a price by exact normalized id, then by the longest table key
// that is a prefix of the id (so "grok-4.6-fast" falls back to "grok-4.6").
func (t *Table) Lookup(model string) (Price, string, bool) {
	if t == nil || model == "" {
		return Price{}, "", false
	}
	n := Normalize(model)
	if p, ok := t.entries[n]; ok {
		return p, t.origin[n], true
	}
	keys := make([]string, 0, len(t.entries))
	for k := range t.entries {
		if strings.HasPrefix(n, k+"-") {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return Price{}, "", false
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	return t.entries[keys[0]], t.origin[keys[0]], true
}

// Cost prices lifetime usage. False when the model is unknown or the runtime
// never reported usage.
func (t *Table) Cost(model string, u types.Usage) (float64, string, bool) {
	if !u.Known {
		return 0, "", false
	}
	p, origin, ok := t.Lookup(model)
	if !ok {
		return 0, "", false
	}
	usd := (float64(u.Input)*p.In + float64(u.Output)*p.Out +
		float64(u.CacheRead)*p.CacheRead + float64(u.CacheWrite)*p.CacheWrite) / 1e6
	return usd, "table:" + origin, true
}

// Window returns the context window for a model. A "[1m]" marker on the id
// means the 1M-context variant regardless of the table.
func (t *Table) Window(model string) (int64, bool) {
	if oneM.MatchString(strings.ToLower(strings.TrimSpace(model))) {
		return 1_000_000, true
	}
	p, _, ok := t.Lookup(model)
	if !ok || p.Window <= 0 {
		return 0, false
	}
	return p.Window, true
}

// Apply stamps estimated cost and table windows onto overlays that carry a
// model. Runtime-reported values are never overwritten.
func (t *Table) Apply(ovs []types.Overlay) {
	if t == nil {
		return
	}
	for i := range ovs {
		o := &ovs[i]
		if o.Model == "" {
			continue
		}
		if o.CostUSD == nil {
			if usd, src, ok := t.Cost(o.Model, o.Usage); ok {
				v := usd
				o.CostUSD = &v
				o.CostSource = src
			}
		}
		if o.ContextWindow == nil {
			if w, ok := t.Window(o.Model); ok {
				win := w
				o.ContextWindow = &win
				o.WindowSource = "table"
			}
		}
	}
}
