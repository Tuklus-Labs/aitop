package graph

import (
	"strings"
	"testing"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

// Risk rows: GF-ID-1.
func TestCanonicalNodeIDs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		make  func() (string, error)
		want  string
	}{
		{
			name:  "claude-session",
			input: "root",
			make: func() (string, error) {
				return stringResult(ClaudeSessionID("root"))
			},
			want: "claude:session:root",
		},
		{
			name:  "claude-agent",
			input: "root,child",
			make: func() (string, error) {
				return stringResult(ClaudeAgentID("root", "child"))
			},
			want: "claude:agent:root:child",
		},
		{
			name:  "codex-thread",
			input: "thread",
			make: func() (string, error) {
				return stringResult(CodexThreadID("thread"))
			},
			want: "codex:thread:thread",
		},
		{
			name:  "grok-session",
			input: "session",
			make: func() (string, error) {
				return stringResult(GrokSessionID("session"))
			},
			want: "grok:session:session",
		},
		{
			name:  "local-unit-service-suffix",
			input: "hermes-qwen38.service",
			make: func() (string, error) {
				return stringResult(LocalUnitID("hermes-qwen38.service"))
			},
			want: "local:unit:hermes-qwen38",
		},
		{
			name:  "local-unit-no-suffix",
			input: "ollama",
			make: func() (string, error) {
				return stringResult(LocalUnitID("ollama"))
			},
			want: "local:unit:ollama",
		},
		{
			name:  "local-unit-removes-one-suffix",
			input: "worker.service.service",
			make: func() (string, error) {
				return stringResult(LocalUnitID("worker.service.service"))
			},
			want: "local:unit:worker.service",
		},
		{
			name:  "local-process",
			input: "pid=42,startTicks=99",
			make: func() (string, error) {
				return stringResult(LocalProcessID(42, 99))
			},
			want: "local:pid:42:99",
		},
		{
			name:  "passive-process",
			input: "pid=42,startTicks=99",
			make: func() (string, error) {
				return stringResult(PassiveProcessID(42, 99))
			},
			want: "proc:42:99",
		},
		{
			name:  "process-incarnation",
			input: "runtime=claude,pid=42,startTicks=99",
			make: func() (string, error) {
				return stringResult(ProcessIncarnation(types.RuntimeClaude, ProcessIdentity{PID: 42, StartTicks: 99}))
			},
			want: "claude:proc:42:99",
		},
		{
			name:  "invocation-incarnation",
			input: "runtime=codex,invocation=turn-7",
			make: func() (string, error) {
				return stringResult(InvocationIncarnation(types.RuntimeCodex, "turn-7"))
			},
			want: "codex:invocation:turn-7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.make()
			if err != nil || got != tt.want {
				t.Fatalf("canonical-graph-id invariant violated: rule=format name=%q input=%q result=%q want=%q error=%v", tt.name, tt.input, got, tt.want, err)
			}
		})
	}

	supported := []types.Runtime{
		types.RuntimeGrok,
		types.RuntimeClaude,
		types.RuntimeCodex,
		types.RuntimeHermes,
		types.RuntimeParlor,
		types.RuntimeForge,
		types.RuntimeLocal,
	}
	for _, runtime := range supported {
		got, err := ProcessIncarnation(runtime, ProcessIdentity{PID: 7, StartTicks: 11})
		want := IncarnationID(string(runtime) + ":proc:7:11")
		if err != nil || got != want {
			t.Fatalf("canonical-graph-id invariant violated: rule=runtime-set runtime=%q input={pid:7 startTicks:11} result=%q want=%q error=%v", runtime, got, want, err)
		}
	}
}

// Risk rows: GF-ID-1, GF-BOUND-1.
func TestCanonicalNodeIDsRejectEmptyControlAndOversizeComponents(t *testing.T) {
	oversize := strings.Repeat("x", 192)
	tests := []struct {
		name      string
		input     string
		make      func() (string, error)
		wantError string
	}{
		{
			name:  "empty-claude-session",
			input: "session=empty",
			make: func() (string, error) {
				return stringResult(ClaudeSessionID(""))
			},
			wantError: "empty",
		},
		{
			name:  "empty-claude-agent-root",
			input: "root=empty,agent=child",
			make: func() (string, error) {
				return stringResult(ClaudeAgentID("", "child"))
			},
			wantError: "empty",
		},
		{
			name:  "empty-claude-agent",
			input: "root=root,agent=empty",
			make: func() (string, error) {
				return stringResult(ClaudeAgentID("root", ""))
			},
			wantError: "empty",
		},
		{
			name:  "empty-codex-thread",
			input: "thread=empty",
			make: func() (string, error) {
				return stringResult(CodexThreadID(""))
			},
			wantError: "empty",
		},
		{
			name:  "empty-grok-session",
			input: "session=empty",
			make: func() (string, error) {
				return stringResult(GrokSessionID(""))
			},
			wantError: "empty",
		},
		{
			name:  "empty-local-unit",
			input: "unit=empty",
			make: func() (string, error) {
				return stringResult(LocalUnitID(""))
			},
			wantError: "empty",
		},
		{
			name:  "normalized-empty-local-unit",
			input: "unit=.service",
			make: func() (string, error) {
				return stringResult(LocalUnitID(".service"))
			},
			wantError: "empty",
		},
		{
			name:  "empty-invocation",
			input: "runtime=codex,invocation=empty",
			make: func() (string, error) {
				return stringResult(InvocationIncarnation(types.RuntimeCodex, ""))
			},
			wantError: "empty",
		},
		{
			name:  "nul",
			input: "session=bad\\x00value",
			make: func() (string, error) {
				return stringResult(ClaudeSessionID("bad\x00value"))
			},
			wantError: "control rune U+0000",
		},
		{
			name:  "delimiter",
			input: "thread=bad:value",
			make: func() (string, error) {
				return stringResult(CodexThreadID("bad:value"))
			},
			wantError: "delimiter ':'",
		},
		{
			name:  "oversize-final-id",
			input: "session-bytes=192",
			make: func() (string, error) {
				return stringResult(GrokSessionID(oversize))
			},
			wantError: "maximum is 192",
		},
		{
			name:  "unknown-process-runtime",
			input: "runtime=empty,pid=7,startTicks=11",
			make: func() (string, error) {
				return stringResult(ProcessIncarnation(types.RuntimeUnknown, ProcessIdentity{PID: 7, StartTicks: 11}))
			},
			wantError: "unknown or unsupported runtime",
		},
		{
			name:  "unsupported-invocation-runtime",
			input: "runtime=other,invocation=turn-7",
			make: func() (string, error) {
				return stringResult(InvocationIncarnation(types.Runtime("other"), "turn-7"))
			},
			wantError: "unknown or unsupported runtime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.make()
			if err == nil {
				t.Fatalf("canonical-graph-id invariant violated: rule=rejection name=%q input=%q result=%q error=%v", tt.name, tt.input, got, err)
			}
			if !strings.Contains(err.Error(), "canonical ID") || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("canonical-graph-id invariant violated: rule=error-context name=%q input=%q result=%q wantError=%q error=%v", tt.name, tt.input, got, tt.wantError, err)
			}
		})
	}
}

// Risk rows: GF-ID-1, GF-BOUND-1.
func TestCanonicalNodeIDAccepts192BytesRejects193(t *testing.T) {
	const prefix = "claude:session:"
	input192 := strings.Repeat("x", 192-len(prefix))
	got192, err192 := ClaudeSessionID(input192)
	want192 := NodeID(prefix + input192)
	if err192 != nil || got192 != want192 || len(got192) != 192 {
		t.Fatalf("canonical-graph-id invariant violated: rule=192-byte-boundary inputBytes=%d result=%q resultBytes=%d want=%q error=%v", len(input192), got192, len(got192), want192, err192)
	}

	input193 := strings.Repeat("x", 193-len(prefix))
	got193, err193 := ClaudeSessionID(input193)
	if err193 == nil {
		t.Fatalf("canonical-graph-id invariant violated: rule=193-byte-rejection inputBytes=%d result=%q resultBytes=%d error=%v", len(input193), got193, len(got193), err193)
	}
	if !strings.Contains(err193.Error(), "193 bytes") || !strings.Contains(err193.Error(), "maximum is 192") {
		t.Fatalf("canonical-graph-id invariant violated: rule=byte-limit-error inputBytes=%d result=%q resultBytes=%d error=%v", len(input193), got193, len(got193), err193)
	}

	multibyte192 := strings.Repeat("é", 88) + "x"
	multibyte193 := strings.Repeat("é", 89)
	if len(prefix+multibyte192) != 192 || len(prefix+multibyte193) != 193 {
		t.Fatalf("canonical-graph-id invariant violated: rule=multibyte-test-fixture-byte-count prefixBytes=%d component192Bytes=%d component193Bytes=%d complete192Bytes=%d complete193Bytes=%d", len(prefix), len(multibyte192), len(multibyte193), len(prefix+multibyte192), len(prefix+multibyte193))
	}

	gotMultibyte192, errMultibyte192 := ClaudeSessionID(multibyte192)
	wantMultibyte192 := NodeID(prefix + multibyte192)
	if errMultibyte192 != nil || gotMultibyte192 != wantMultibyte192 || len(gotMultibyte192) != 192 {
		t.Fatalf("canonical-graph-id invariant violated: rule=multibyte-192-byte-boundary inputBytes=%d result=%q resultBytes=%d want=%q error=%v", len(multibyte192), gotMultibyte192, len(gotMultibyte192), wantMultibyte192, errMultibyte192)
	}

	gotMultibyte193, errMultibyte193 := ClaudeSessionID(multibyte193)
	if gotMultibyte193 != "" || errMultibyte193 == nil || !strings.Contains(errMultibyte193.Error(), "193 bytes") || !strings.Contains(errMultibyte193.Error(), "maximum is 192") {
		t.Fatalf("canonical-graph-id invariant violated: rule=multibyte-193-byte-rejection inputBytes=%d result=%q resultBytes=%d error=%v", len(multibyte193), gotMultibyte193, len(gotMultibyte193), errMultibyte193)
	}
}

// Risk rows: GF-ID-1.
func TestCanonicalNodeIDsRejectInvalidUTF8AndControlRunes(t *testing.T) {
	invalidUTF8 := string([]byte{0xff, 'x'})
	tests := []struct {
		name      string
		input     string
		make      func() (string, error)
		wantError string
	}{
		{
			name:  "invalid-utf8",
			input: invalidUTF8,
			make: func() (string, error) {
				return stringResult(ClaudeSessionID(invalidUTF8))
			},
			wantError: "not valid UTF-8",
		},
		{
			name:  "newline-control",
			input: "root\\nchild",
			make: func() (string, error) {
				return stringResult(ClaudeAgentID("root\nchild", "agent"))
			},
			wantError: "control rune U+000A",
		},
		{
			name:  "tab-control",
			input: "thread\\tname",
			make: func() (string, error) {
				return stringResult(CodexThreadID("thread\tname"))
			},
			wantError: "control rune U+0009",
		},
		{
			name:  "unicode-control",
			input: "session\\u0085name",
			make: func() (string, error) {
				return stringResult(GrokSessionID("session\u0085name"))
			},
			wantError: "control rune U+0085",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.make()
			if err == nil || !strings.Contains(err.Error(), tt.wantError) || !strings.Contains(err.Error(), "canonical ID") {
				t.Fatalf("canonical-graph-id invariant violated: rule=encoding-rejection name=%q input=%q result=%q wantError=%q error=%v", tt.name, tt.input, got, tt.wantError, err)
			}
		})
	}
}

// Risk rows: GF-ID-1, GF-BOUND-1.
func TestProcessIdentityRejectsPartialIdentity(t *testing.T) {
	tests := []struct {
		name       string
		pid        int32
		startTicks uint64
		make       func() (string, error)
	}{
		{
			name:       "local-zero-pid",
			pid:        0,
			startTicks: 11,
			make: func() (string, error) {
				return stringResult(LocalProcessID(0, 11))
			},
		},
		{
			name:       "local-negative-pid",
			pid:        -7,
			startTicks: 11,
			make: func() (string, error) {
				return stringResult(LocalProcessID(-7, 11))
			},
		},
		{
			name:       "local-zero-start",
			pid:        7,
			startTicks: 0,
			make: func() (string, error) {
				return stringResult(LocalProcessID(7, 0))
			},
		},
		{
			name:       "passive-zero-pid",
			pid:        0,
			startTicks: 11,
			make: func() (string, error) {
				return stringResult(PassiveProcessID(0, 11))
			},
		},
		{
			name:       "passive-zero-start",
			pid:        7,
			startTicks: 0,
			make: func() (string, error) {
				return stringResult(PassiveProcessID(7, 0))
			},
		},
		{
			name:       "incarnation-zero-pid",
			pid:        0,
			startTicks: 11,
			make: func() (string, error) {
				return stringResult(ProcessIncarnation(types.RuntimeClaude, ProcessIdentity{PID: 0, StartTicks: 11}))
			},
		},
		{
			name:       "incarnation-zero-start",
			pid:        7,
			startTicks: 0,
			make: func() (string, error) {
				return stringResult(ProcessIncarnation(types.RuntimeClaude, ProcessIdentity{PID: 7, StartTicks: 0}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.make()
			wantField := "PID"
			if tt.pid > 0 {
				wantField = "StartTicks"
			}
			if err == nil || !strings.Contains(err.Error(), "process-identity rule") || !strings.Contains(err.Error(), "field="+wantField) || !strings.Contains(err.Error(), "class=nonpositive") {
				t.Fatalf("pid-start-pair safe-rejection invariant violated: rule=partial-identity name=%q pidClass=%s startTicksClass=%s result=%q wantField=%s error=%v", tt.name, classifyPID(tt.pid), classifyStartTicks(tt.startTicks), got, wantField, err)
			}
		})
	}
}

// Risk rows: GF-ID-1.
func TestProcessIdentityDistinguishesPIDReuse(t *testing.T) {
	const pid int32 = 42
	tests := []struct {
		name string
		make func(uint64) (string, error)
		want func(uint64) string
	}{
		{
			name: "local-process",
			make: func(startTicks uint64) (string, error) {
				return stringResult(LocalProcessID(pid, startTicks))
			},
			want: func(startTicks uint64) string {
				return "local:pid:42:" + decimalUint64(startTicks)
			},
		},
		{
			name: "passive-process",
			make: func(startTicks uint64) (string, error) {
				return stringResult(PassiveProcessID(pid, startTicks))
			},
			want: func(startTicks uint64) string {
				return "proc:42:" + decimalUint64(startTicks)
			},
		},
		{
			name: "runtime-process-incarnation",
			make: func(startTicks uint64) (string, error) {
				return stringResult(ProcessIncarnation(types.RuntimeClaude, ProcessIdentity{PID: pid, StartTicks: startTicks}))
			},
			want: func(startTicks uint64) string {
				return "claude:proc:42:" + decimalUint64(startTicks)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, firstErr := tt.make(100)
			second, secondErr := tt.make(101)
			if firstErr != nil || secondErr != nil || first != tt.want(100) || second != tt.want(101) || first == second {
				t.Fatalf("pid-start-pair invariant violated: rule=reuse-distinction name=%q input={pid:%d firstStartTicks:100 secondStartTicks:101} firstResult=%q firstError=%v secondResult=%q secondError=%v", tt.name, pid, first, firstErr, second, secondErr)
			}
		})
	}
}

func stringResult[T ~string](value T, err error) (string, error) {
	return string(value), err
}

func decimalUint64(value uint64) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
