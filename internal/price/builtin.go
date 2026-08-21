package price

// builtin is the shipped price table, USD per million tokens, list prices as
// published by each vendor on the date in Source. Windows are the default
// context window; a "[1m]" model id overrides to 1M at lookup time.
//
// Keep this honest: every entry names where it came from. Anything not here
// costs "—" until ~/.config/aitop/prices.json supplies it.
var builtin = map[string]Price{
	// Anthropic, https://platform.claude.com/docs/en/about-claude/pricing, read 2026-08-21.
	// Cache read is 0.1x input; cache write uses the 1h tier (2x input), which is
	// what Claude Code's ephemeral_1h_input_tokens reports on this box. Windows:
	// "Claude 4.6 and later models include the full 1M token context window at
	// standard pricing" (same page); 4.5 and earlier are 200k. Live sessions here
	// holding 550k tokens agree.
	"claude-fable-5":    {In: 10, Out: 50, CacheRead: 1, CacheWrite: 20, Window: 1_000_000, Source: anthropic},
	"claude-mythos-5":   {In: 10, Out: 50, CacheRead: 1, CacheWrite: 20, Window: 1_000_000, Source: anthropic},
	"claude-opus-5":     {In: 5, Out: 25, CacheRead: 0.5, CacheWrite: 10, Window: 1_000_000, Source: anthropic},
	"claude-opus-4-8":   {In: 5, Out: 25, CacheRead: 0.5, CacheWrite: 10, Window: 1_000_000, Source: anthropic},
	"claude-opus-4-7":   {In: 5, Out: 25, CacheRead: 0.5, CacheWrite: 10, Window: 1_000_000, Source: anthropic},
	"claude-opus-4-6":   {In: 5, Out: 25, CacheRead: 0.5, CacheWrite: 10, Window: 1_000_000, Source: anthropic},
	"claude-opus-4-5":   {In: 5, Out: 25, CacheRead: 0.5, CacheWrite: 10, Window: 200_000, Source: anthropic},
	"claude-opus-4-1":   {In: 15, Out: 75, CacheRead: 1.5, CacheWrite: 30, Window: 200_000, Source: anthropic},
	"claude-sonnet-5":   {In: 2, Out: 10, CacheRead: 0.2, CacheWrite: 4, Window: 1_000_000, Source: anthropic},
	"claude-sonnet-4-6": {In: 3, Out: 15, CacheRead: 0.3, CacheWrite: 6, Window: 1_000_000, Source: anthropic},
	"claude-sonnet-4-5": {In: 3, Out: 15, CacheRead: 0.3, CacheWrite: 6, Window: 200_000, Source: anthropic},
	"claude-haiku-4-5":  {In: 1, Out: 5, CacheRead: 0.1, CacheWrite: 2, Window: 200_000, Source: anthropic},

	// OpenAI, https://developers.openai.com/api/docs/pricing, read 2026-08-21.
	// Cached input is 0.1x; no cache-write charge. Windows come from the Codex
	// rollout itself (model_context_window), so none are asserted here. The
	// daybreak aliases point at sol (blue) and cyber (red).
	"gpt-5.6-sol":              {In: 4, Out: 20, CacheRead: 0.4, Source: openai},
	"gpt-5.6-terra":            {In: 2, Out: 12, CacheRead: 0.2, Source: openai},
	"gpt-5.6-luna":             {In: 0.2, Out: 1.2, CacheRead: 0.02, Source: openai},
	"gpt-5.6-cyber":            {In: 12.5, Out: 75, CacheRead: 1.25, Source: openai},
	"gpt-5.5":                  {In: 5, Out: 30, CacheRead: 0.5, Source: openai},
	"gpt-5.5-cyber":            {In: 12.5, Out: 75, CacheRead: 1.25, Source: openai},
	"gpt-5":                    {In: 1.25, Out: 10, CacheRead: 0.125, Source: openai},
	"gpt-5.3-codex":            {In: 1.75, Out: 14, CacheRead: 0.175, Source: openai},
	"gpt-daybreak-blue-latest": {In: 4, Out: 20, CacheRead: 0.4, Source: openai + " (alias of gpt-5.6-sol)"},
	"gpt-daybreak-red-latest":  {In: 12.5, Out: 75, CacheRead: 1.25, Source: openai + " (alias of gpt-5.6-cyber)"},
	"daybreak-blue-latest":     {In: 4, Out: 20, CacheRead: 0.4, Source: openai + " (alias of gpt-5.6-sol)"},
	"daybreak-red-latest":      {In: 12.5, Out: 75, CacheRead: 1.25, Source: openai + " (alias of gpt-5.6-cyber)"},

	// xAI, https://docs.x.ai/docs/models, read 2026-08-21. Base (<200k) tier;
	// xAI doubles the rate past 200k prompt tokens and that tier is not modelled.
	// Grok Build writes no usage totals, so these price nothing today; the
	// 500k window matches signals.json contextWindowTokens.
	"grok-4.6":       {In: 2, Out: 6, CacheRead: 0.5, CacheWrite: 2, Window: 500_000, Source: xai},
	"grok-4.5":       {In: 2, Out: 6, CacheRead: 0.3, CacheWrite: 2, Window: 500_000, Source: xai},
	"grok-4.3":       {In: 1.25, Out: 2.5, CacheRead: 0.2, CacheWrite: 1.25, Window: 1_000_000, Source: xai},
	"grok-build-0.1": {In: 1, Out: 2, CacheRead: 0.2, CacheWrite: 1, Window: 256_000, Source: xai},
}

const (
	anthropic = "anthropic list 2026-08-21"
	xai       = "xai list 2026-08-21"
	openai    = "openai list 2026-08-21"
)
