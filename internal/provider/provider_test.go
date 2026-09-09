package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

// flexInt absorbs every line shape Sonnet has emitted so far. The accepted
// separators (first "-", then "..", then ",") are the contract; a range takes
// its first number because the start of the change is the best anchor.
func TestFlexInt_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr string
	}{
		{name: "plain int", raw: `42`, want: 42},
		{name: "int as string", raw: `"80"`, want: 80},
		{name: "hyphen range takes the start", raw: `"541-543"`, want: 541},
		{name: "dot-dot range takes the start", raw: `"12..14"`, want: 12},
		{name: "comma list takes the first", raw: `"7,9"`, want: 7},
		{name: "surrounding whitespace is trimmed", raw: `" 33 "`, want: 33},
		{name: "space before the separator", raw: `"100 - 104"`, want: 100},
		{name: "leading hyphen is not a separator", raw: `"-5"`, want: -5},
		{name: "non-numeric string", raw: `"abc"`, wantErr: "not coercible to int"},
		{name: "empty string", raw: `""`, wantErr: "not coercible to int"},
		{name: "boolean", raw: `true`, wantErr: "not int or string"},
		// encoding/json hands null to UnmarshalJSON, and the int branch
		// accepts it as a no-op; a null line lands at 0 rather than erroring.
		{name: "null decodes as zero", raw: `null`, want: 0},
		{name: "object", raw: `{"line":4}`, wantErr: "not int or string"},
		{name: "float", raw: `4.5`, wantErr: "not int or string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got flexInt
			err := json.Unmarshal([]byte(tt.raw), &got)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Unmarshal(%s) = %d, want error containing %q", tt.raw, got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal(%s): %v", tt.raw, err)
			}
			if int(got) != tt.want {
				t.Errorf("Unmarshal(%s) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

// The registry is the only way callers get a provider; the two error paths
// must say what went wrong rather than hand back a nil interface.
func TestNew(t *testing.T) {
	for _, name := range []string{"", "stub"} {
		p, err := New(name, "")
		if err != nil {
			t.Fatalf("New(%q): %v", name, err)
		}
		if p.Name() != "stub" {
			t.Errorf("New(%q).Name() = %q, want stub", name, p.Name())
		}
	}
	// deepseek was advertised as a placeholder and removed in #30; it is now
	// just another unknown name.
	for _, name := range []string{"deepseek", "openai"} {
		if _, err := New(name, ""); err == nil || !strings.Contains(err.Error(), "unknown provider") {
			t.Errorf("New(%s) = %v, want an unknown-provider error", name, err)
		}
	}
	p, err := New("anthropic", "")
	if err != nil {
		t.Fatalf("New(anthropic): %v", err)
	}
	if p.Name() != "anthropic-claude-haiku-4-5" {
		t.Errorf("New(anthropic, \"\").Name() = %q, want the Haiku default", p.Name())
	}
	if _, err := New("anthropic", "claude-unpriced-9"); err == nil {
		t.Error("New(anthropic, unpriced model) should fail closed on priceTable")
	}
}
