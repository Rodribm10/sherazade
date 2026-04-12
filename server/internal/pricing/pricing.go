// Package pricing converts LLM token usage into USD cents for per-agent
// budget tracking. Prices are baked in at build time so the daemon doesn't
// need to hit an external service. Update the table below when providers
// change their pricing or add new models.
//
// Prices are in USD cents per 1,000,000 tokens. For example, Claude Sonnet
// 4.5 at $3 / $15 per MTok becomes 300 / 1500 cents per MTok.
package pricing

import "strings"

// ModelPrice is the cost structure for a single model, expressed in
// cents per million tokens. Zero values mean "not billed separately".
type ModelPrice struct {
	InputCentsPerMTok      int64
	OutputCentsPerMTok     int64
	CacheReadCentsPerMTok  int64
	CacheWriteCentsPerMTok int64
}

// Usage mirrors daemon TaskUsageEntry / handler TaskUsagePayload, kept
// local so this package has no cross-package dependencies.
type Usage struct {
	Provider         string
	Model            string
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

// Prices as of 2026-04. Keys are lowercased model names; longest-prefix
// match wins so "claude-sonnet-4-5-20260101" hits the "claude-sonnet-4-5"
// entry. Add new entries at the top so they're matched first.
var modelPrices = map[string]ModelPrice{
	// ——— Anthropic ———
	// Claude Opus 4 / 4.6: $15 / $75 per MTok
	"claude-opus-4":   {1500, 7500, 150, 1875},
	"claude-opus-4-1": {1500, 7500, 150, 1875},
	"claude-opus-4-5": {1500, 7500, 150, 1875},
	"claude-opus-4-6": {1500, 7500, 150, 1875},
	// Claude Sonnet 4 / 4.5 / 4.6: $3 / $15 per MTok
	"claude-sonnet-4":   {300, 1500, 30, 375},
	"claude-sonnet-4-5": {300, 1500, 30, 375},
	"claude-sonnet-4-6": {300, 1500, 30, 375},
	// Claude Haiku 4.5: $1 / $5 per MTok
	"claude-haiku-4":   {100, 500, 10, 125},
	"claude-haiku-4-5": {100, 500, 10, 125},

	// ——— OpenAI ———
	// GPT-5 / o3: rough estimates from public pricing, adjust as needed.
	"gpt-5":      {1000, 3000, 100, 0},
	"gpt-5-mini": {25, 200, 3, 0},
	"o3":         {1000, 4000, 100, 0},
	"o3-mini":    {110, 440, 11, 0},
	"gpt-4o":     {500, 1500, 250, 0},
	"gpt-4o-mini": {15, 60, 8, 0},
}

// ComputeCostCents returns the total USD-cents cost for the usage entry.
// Unknown models resolve to 0 cents — we never want cost tracking to block
// a task, so silent fallback is preferred over a hard error.
func ComputeCostCents(u Usage) int64 {
	price := lookupPrice(u.Model)
	const denom int64 = 1_000_000
	cost := u.InputTokens*price.InputCentsPerMTok +
		u.OutputTokens*price.OutputCentsPerMTok +
		u.CacheReadTokens*price.CacheReadCentsPerMTok +
		u.CacheWriteTokens*price.CacheWriteCentsPerMTok
	return cost / denom
}

// lookupPrice finds the best-matching price entry for a model name.
// We check: (1) exact match, then (2) longest key that is a prefix of
// the model name. This lets date-suffixed model IDs map cleanly.
func lookupPrice(model string) ModelPrice {
	lc := strings.ToLower(model)
	if p, ok := modelPrices[lc]; ok {
		return p
	}
	var bestKey string
	var best ModelPrice
	for k, p := range modelPrices {
		if strings.HasPrefix(lc, k) && len(k) > len(bestKey) {
			bestKey = k
			best = p
		}
	}
	return best
}
