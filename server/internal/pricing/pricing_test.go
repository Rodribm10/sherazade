package pricing

import "testing"

func TestComputeCostCents(t *testing.T) {
	tests := []struct {
		name  string
		usage Usage
		want  int64
	}{
		{
			// 1M input at $3 per MTok = 300 cents.
			name:  "sonnet 1M input only",
			usage: Usage{Model: "claude-sonnet-4-5", InputTokens: 1_000_000},
			want:  300,
		},
		{
			// 1M input + 1M output at $3/$15 = 300 + 1500 = 1800 cents.
			name: "sonnet 1M in + 1M out",
			usage: Usage{
				Model:        "claude-sonnet-4-5",
				InputTokens:  1_000_000,
				OutputTokens: 1_000_000,
			},
			want: 1800,
		},
		{
			// Dated model id should resolve via prefix match.
			name: "dated sonnet id prefix match",
			usage: Usage{
				Model:        "claude-sonnet-4-5-20260101",
				InputTokens:  500_000,
				OutputTokens: 100_000,
			},
			// 500k * 300 / 1M = 150, 100k * 1500 / 1M = 150, total 300.
			want: 300,
		},
		{
			// Opus is 5x sonnet on input, 5x on output.
			name: "opus 100k in + 10k out",
			usage: Usage{
				Model:        "claude-opus-4-6",
				InputTokens:  100_000,
				OutputTokens: 10_000,
			},
			// 100k * 1500 / 1M = 150, 10k * 7500 / 1M = 75, total 225.
			want: 225,
		},
		{
			// Unknown model must return 0 (silent fallback).
			name: "unknown model",
			usage: Usage{
				Model:        "some-random-model-9000",
				InputTokens:  1_000_000,
				OutputTokens: 1_000_000,
			},
			want: 0,
		},
		{
			// Cache read is 10x cheaper than input on claude.
			name: "sonnet cache read",
			usage: Usage{
				Model:           "claude-sonnet-4-5",
				CacheReadTokens: 10_000_000,
			},
			// 10M * 30 / 1M = 300.
			want: 300,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ComputeCostCents(tt.usage); got != tt.want {
				t.Errorf("ComputeCostCents() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLookupPricePrefixMatch(t *testing.T) {
	// "claude-sonnet-4-5-exp" should match "claude-sonnet-4-5",
	// not the shorter "claude-sonnet-4".
	p := lookupPrice("claude-sonnet-4-5-exp")
	if p.InputCentsPerMTok != 300 {
		t.Errorf("prefix match failed: got %+v", p)
	}
}
