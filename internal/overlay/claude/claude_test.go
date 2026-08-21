package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aitop/internal/types"
)

// Values the fixture pair must yield. Sourced from real redacted records, so a
// drift here is a drift against the shape the CLI actually writes.
const (
	fixturePID     = 2890787
	fixtureSession = "aef10889-693e-4768-98d5-26a21d5d6f0c"
	fixtureCWD     = "/home/aegis"
	wantModel      = "claude-fable-5"
	wantTokens     = 115728 // 326 input + 12284 cache_creation + 103118 cache_read
	wantTitle      = "aitop polish and UI refinement"
	wantProject    = "aitop"
	wantOverlayCWD = "/home/aegis/Projects/aitop"
	wantEffort     = "xhigh"
	wantBranch     = "HEAD"
	wantName       = "aegis-48"
	wantStatus     = "idle"
	wantStartedMS  = 1787257499899
	wantUpdatedMS  = 1787260488343
)

func TestCollectReadsPIDSidecar(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `{"pid":3706024,"sessionId":"0eaefa73","cwd":"/home/aegis/Projects/theia","procStart":111,"kind":"interactive","entrypoint":"cli","name":"aegis-75"}`
	if err := os.WriteFile(filepath.Join(dir, "3706024.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	ovs, err := Collect(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(ovs) != 1 || ovs[0].PID != 3706024 || ovs[0].StartTime != 111 {
		t.Fatalf("claude-pid-sidecar join fields violated: %+v", ovs)
	}
	if ovs[0].ProvenName == "Heph" {
		t.Fatalf("claude-sidecar-does-not-prove-Heph violated: %q", ovs[0].ProvenName)
	}
	if ovs[0].Project != "theia" {
		t.Fatalf("claude-project-from-sidecar-cwd violated: %q", ovs[0].Project)
	}
	if ovs[0].CostUSD != nil {
		t.Fatalf("never-invent-cost violated")
	}
}

func TestCollectProcStartString(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `{"pid":"35037","sessionId":"19a35c8c","cwd":"/home/aegis","procStart":"28229"}`
	if err := os.WriteFile(filepath.Join(dir, "35037.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	ovs, err := Collect(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(ovs) != 1 || ovs[0].PID != 35037 || ovs[0].StartTime != 28229 {
		t.Fatalf("claude-procStart-string invariant violated: %+v", ovs)
	}
}

// The whole join, end to end, against the real sidecar and the real transcript
// records: every field the TUI renders for a Claude row.
func TestCollectJoinsTranscriptOntoSidecar(t *testing.T) {
	home := newHome(t)
	writeSidecar(t, home)
	writeTranscript(t, home, readFixture(t))

	ov := collectOne(t, New(), home)

	if ov.PID != fixturePID {
		t.Fatalf("claude-pid-comes-from-sidecar violated: got %d want %d", ov.PID, fixturePID)
	}
	if ov.SessionID != fixtureSession {
		t.Fatalf("claude-sessionid-comes-from-sidecar violated: got %q want %q", ov.SessionID, fixtureSession)
	}
	if ov.Runtime != types.RuntimeClaude {
		t.Fatalf("claude-rows-are-runtime-claude violated: got %q", ov.Runtime)
	}
	if ov.Model != wantModel {
		t.Fatalf("claude-model-comes-from-last-real-assistant violated: got %q want %q", ov.Model, wantModel)
	}
	if ov.TokensUsed == nil {
		t.Fatalf("claude-tokens-are-last-turn-occupancy violated: got nil want %d", wantTokens)
	}
	if *ov.TokensUsed != wantTokens {
		t.Fatalf("claude-tokens-are-last-turn-occupancy violated: got %d want %d (input+cache_creation+cache_read, output excluded)", *ov.TokensUsed, wantTokens)
	}
	if ov.Title != wantTitle {
		t.Fatalf("claude-title-comes-from-ai-title-record violated: got %q want %q", ov.Title, wantTitle)
	}
	if ov.SessionName != wantName {
		t.Fatalf("claude-sidecar-name-is-session-name violated: got %q want %q", ov.SessionName, wantName)
	}
	if ov.ProvenName != "" {
		t.Fatalf("claude-session-name-is-a-label-not-an-identity violated: ProvenName %q", ov.ProvenName)
	}
	if ov.OverlayCWD != wantOverlayCWD {
		t.Fatalf("claude-transcript-cwd-overrides-launch-cwd violated: got %q want %q", ov.OverlayCWD, wantOverlayCWD)
	}
	if ov.Project != wantProject {
		t.Fatalf("claude-project-follows-transcript-cwd violated: got %q want %q", ov.Project, wantProject)
	}
	if ov.Effort != wantEffort {
		t.Fatalf("claude-effort-comes-from-winning-assistant-record violated: got %q want %q", ov.Effort, wantEffort)
	}
	if ov.Branch != wantBranch {
		t.Fatalf("claude-branch-comes-from-transcript violated: got %q want %q", ov.Branch, wantBranch)
	}
	if ov.Status != wantStatus {
		t.Fatalf("claude-status-passes-through-lowercase violated: got %q want %q", ov.Status, wantStatus)
	}
	if !ov.StartedAt.Equal(time.UnixMilli(wantStartedMS)) {
		t.Fatalf("claude-startedAt-is-ms-epoch violated: got %v want %v", ov.StartedAt, time.UnixMilli(wantStartedMS))
	}
	if !ov.UpdatedAt.Equal(time.UnixMilli(wantUpdatedMS)) {
		t.Fatalf("claude-updatedAt-is-ms-epoch violated: got %v want %v", ov.UpdatedAt, time.UnixMilli(wantUpdatedMS))
	}
	if ov.ContextWindow != nil {
		t.Fatalf("unknown-is-absent violated: ContextWindow %d, no claude file carries a window", *ov.ContextWindow)
	}
	if ov.CostUSD != nil {
		t.Fatalf("never-invent-cost violated: CostUSD %v", *ov.CostUSD)
	}
	if ov.ContextFill != nil {
		t.Fatalf("unknown-is-absent violated: ContextFill %v with no window to divide by", *ov.ContextFill)
	}
}

// A refusal or API error writes a "<synthetic>" record with an all-zero usage
// block. Live transcripts really do end on one, and letting it win reports a
// working session as model "<synthetic>" holding zero tokens.
func TestSyntheticAssistantDoesNotClobberRealOne(t *testing.T) {
	lines := fixtureLines(t)
	if len(lines) != 4 {
		t.Fatalf("fixture-shape violated: got %d lines want 4", len(lines))
	}
	real, synth := lines[1], lines[2]

	// Ammunition check: the synthetic record must really be a parseable
	// assistant record naming <synthetic>, or this test proves nothing.
	var probe struct {
		Type    string `json:"type"`
		Message struct {
			Model string `json:"model"`
		} `json:"message"`
	}
	if json.Unmarshal(synth, &probe) != nil || probe.Type != "assistant" || probe.Message.Model != syntheticModel {
		t.Fatalf("synthetic-fixture-must-be-live-ammunition violated: type %q model %q", probe.Type, probe.Message.Model)
	}

	home := newHome(t)
	writeSidecar(t, home)
	writeTranscript(t, home, bytes.Join([][]byte{real, synth, nil}, []byte("\n")))

	ov := collectOne(t, New(), home)
	if ov.Model != wantModel {
		t.Fatalf("claude-synthetic-records-never-win-the-model violated: got %q want %q", ov.Model, wantModel)
	}
	if ov.TokensUsed == nil || *ov.TokensUsed != wantTokens {
		t.Fatalf("claude-synthetic-records-never-zero-the-tokens violated: got %v want %d", derefTokens(ov.TokensUsed), wantTokens)
	}
}

// Transcripts run to many MB. We read the last tailBytes only, the window opens
// mid-record, and that first partial line must be thrown away.
//
// The fixture is laid out so the window opens exactly on the first byte of a
// poison ai-title record embedded mid-line, and so that NO other ai-title record
// survives into the window. Fields are last-wins, so a poison at the head of the
// window is only detectable when nothing later overwrites it: keep the discard
// and the title is empty, drop the discard and the poison is the title.
func TestTailWindowDiscardsPartialFirstLine(t *testing.T) {
	poison := `{"type":"ai-title","aiTitle":"PARTIAL LINE POISON"}`
	if !json.Valid([]byte(poison)) {
		t.Fatalf("planted-poison-must-be-live-ammunition violated: the straddled fragment does not parse, so a missing discard would be invisible")
	}
	headPoison := `{"type":"ai-title","aiTitle":"HEAD POISON BEFORE WINDOW"}` + "\n"
	const straddle = 4096

	// Everything but the real ai-title record: the poison must be the only
	// title anywhere the parser can reach.
	var bodyBuf bytes.Buffer
	for _, ln := range fixtureLines(t) {
		if bytes.Contains(ln, []byte(`"type":"ai-title"`)) {
			continue
		}
		bodyBuf.Write(ln)
		bodyBuf.WriteByte('\n')
	}
	body := bodyBuf.Bytes()
	if bytes.Contains(body, []byte("ai-title")) {
		t.Fatalf("tail-fixture-carries-no-title-of-its-own violated: a real ai-title in the window would mask the poison")
	}
	want := tailBytes - len(poison) - 1
	fillerLen := want - len(body) - 1
	if fillerLen < 16 {
		t.Fatalf("tail-window-fixture-must-fit violated: filler length %d", fillerLen)
	}
	const fillerWrap = `{"pad":""}`
	filler := `{"pad":"` + strings.Repeat("x", fillerLen-len(fillerWrap)) + `"}`
	if len(filler) != fillerLen {
		t.Fatalf("tail-window-fixture-must-fit violated: filler %d want %d", len(filler), fillerLen)
	}

	var buf bytes.Buffer
	buf.WriteString(headPoison)
	buf.WriteString(strings.Repeat("Z", straddle))
	buf.WriteString(poison)
	buf.WriteByte('\n')
	buf.WriteString(filler)
	buf.WriteByte('\n')
	buf.Write(body)
	data := buf.Bytes()

	poisonAt := int64(len(headPoison) + straddle)
	if int64(len(data))-tailBytes != poisonAt {
		t.Fatalf("tail-window-opens-on-the-poison violated: window opens at %d, poison starts at %d", int64(len(data))-tailBytes, poisonAt)
	}

	home := newHome(t)
	writeSidecar(t, home)
	writeTranscript(t, home, data)

	c := New()
	sp := installSpy(c)
	ov := collectOne(t, c, home)

	if sp.read > tailBytes {
		t.Fatalf("claude-reads-only-the-tail violated: read %d bytes of a %d byte transcript, cap is %d", sp.read, len(data), tailBytes)
	}
	if ov.Title != "" {
		t.Fatalf("claude-discards-the-partial-first-line violated: got title %q, a record straddling the window edge was parsed as if it were whole", ov.Title)
	}
	if ov.Model != wantModel {
		t.Fatalf("claude-tail-still-parses-the-real-records violated: got %q want %q", ov.Model, wantModel)
	}
	if ov.TokensUsed == nil || *ov.TokensUsed != wantTokens {
		t.Fatalf("claude-tail-still-parses-the-real-records violated: tokens %v want %d", derefTokens(ov.TokensUsed), wantTokens)
	}
	if ov.OverlayCWD != wantOverlayCWD {
		t.Fatalf("claude-tail-still-parses-the-real-records violated: cwd %q want %q", ov.OverlayCWD, wantOverlayCWD)
	}
}

// Left join: a session with no transcript on disk is a row with absent fields,
// never a dropped row.
func TestMissingTranscriptStillYieldsSidecarOverlay(t *testing.T) {
	home := newHome(t)
	writeSidecar(t, home)

	ov := collectOne(t, New(), home)
	if ov.PID != fixturePID || ov.SessionName != wantName || ov.Status != wantStatus {
		t.Fatalf("claude-missing-transcript-is-absence-not-a-dropped-row violated: %+v", ov)
	}
	if ov.Model != "" {
		t.Fatalf("unknown-is-absent violated: model %q with no transcript on disk", ov.Model)
	}
	if ov.TokensUsed != nil {
		t.Fatalf("unknown-is-absent violated: tokens %d with no transcript on disk", *ov.TokensUsed)
	}
	if ov.Title != "" {
		t.Fatalf("claude-title-comes-from-ai-title-record violated: got %q with no transcript on disk", ov.Title)
	}
	if ov.OverlayCWD != fixtureCWD || ov.Project != "~" {
		t.Fatalf("claude-falls-back-to-sidecar-cwd violated: cwd %q project %q", ov.OverlayCWD, ov.Project)
	}
}

// An unreadable transcript is the same story as a missing one.
func TestUnreadableTranscriptStillYieldsSidecarOverlay(t *testing.T) {
	home := newHome(t)
	writeSidecar(t, home)
	// A directory where the .jsonl should be: open succeeds on Linux, reads do not.
	dir := filepath.Join(home, "projects", EncodeProjectDir(fixtureCWD), fixtureSession+".jsonl")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	ov := collectOne(t, New(), home)
	if ov.PID != fixturePID || ov.SessionName != wantName {
		t.Fatalf("claude-unreadable-transcript-is-absence-not-a-dropped-row violated: %+v", ov)
	}
	if ov.Model != "" || ov.TokensUsed != nil {
		t.Fatalf("unknown-is-absent violated: model %q tokens %v from an unreadable transcript", ov.Model, derefTokens(ov.TokensUsed))
	}
}

func TestEncodeProjectDir(t *testing.T) {
	cases := []struct{ cwd, want string }{
		{"/home/aegis", "-home-aegis"},
		{"/home/aegis/.config/superpowers/worktrees/parlor", "-home-aegis--config-superpowers-worktrees-parlor"},
		{"/home/aegis/.grok/sessions/%2Fhome%2Faegis", "-home-aegis--grok-sessions--2Fhome-2Faegis"},
	}
	for _, tc := range cases {
		if got := EncodeProjectDir(tc.cwd); got != tc.want {
			t.Fatalf("claude-project-dir-encoding violated: %q -> %q want %q", tc.cwd, got, tc.want)
		}
	}
}

// The collector runs about once a second. An unchanged transcript must not be
// reopened, and a changed one must be.
func TestTranscriptCacheReparsesOnlyOnChange(t *testing.T) {
	home := newHome(t)
	writeSidecar(t, home)
	path := writeTranscript(t, home, readFixture(t))

	c := New()
	sp := installSpy(c)

	first := collectOne(t, c, home)
	if sp.opens != 1 {
		t.Fatalf("claude-transcript-is-opened-once-per-parse violated: opens %d want 1", sp.opens)
	}
	second := collectOne(t, c, home)
	if sp.opens != 1 {
		t.Fatalf("claude-cache-serves-unchanged-transcripts violated: opens %d want 1 after a second collect with no file change", sp.opens)
	}
	if second.Title != first.Title || second.Model != first.Model {
		t.Fatalf("claude-cache-returns-the-same-parse violated: %q/%q then %q/%q", first.Title, first.Model, second.Title, second.Model)
	}
	if first.TokensUsed == nil || second.TokensUsed == nil || *first.TokensUsed != *second.TokensUsed {
		t.Fatalf("claude-cache-returns-the-same-parse violated: tokens %v then %v", derefTokens(first.TokensUsed), derefTokens(second.TokensUsed))
	}
	if first.TokensUsed == second.TokensUsed {
		t.Fatalf("claude-cache-hands-out-fresh-pointers violated: two rows share one *int64, a write through one would rewrite the other")
	}

	appendLine(t, path, `{"type":"ai-title","aiTitle":"second title after append"}`)

	third := collectOne(t, c, home)
	if sp.opens != 2 {
		t.Fatalf("claude-cache-reparses-changed-transcripts violated: opens %d want 2 after the file grew", sp.opens)
	}
	if third.Title != "second title after append" {
		t.Fatalf("claude-cache-reparses-changed-transcripts violated: title %q want %q", third.Title, "second title after append")
	}
}

// No file here carries a context window. A model name that advertises 1M is
// still not a measurement.
func TestContextWindowStaysNilForBracketedModel(t *testing.T) {
	orig := readFixture(t)
	swapped := bytes.ReplaceAll(orig, []byte(`"model": "claude-fable-5"`), []byte(`"model": "claude-opus-5[1m]"`))
	if bytes.Equal(orig, swapped) {
		t.Fatalf("fixture-swap-must-bite violated: model string not found in the fixture, the test would pass on the wrong record")
	}

	home := newHome(t)
	writeSidecar(t, home)
	writeTranscript(t, home, swapped)

	ov := collectOne(t, New(), home)
	if ov.Model != "claude-opus-5[1m]" {
		t.Fatalf("claude-model-is-passed-through-verbatim violated: got %q", ov.Model)
	}
	if ov.ContextWindow != nil {
		t.Fatalf("no-window-is-inferred-from-a-model-name violated: ContextWindow %d", *ov.ContextWindow)
	}
	if ov.ContextFill != nil {
		t.Fatalf("no-window-is-inferred-from-a-model-name violated: ContextFill %v", *ov.ContextFill)
	}
	if ov.CostUSD != nil {
		t.Fatalf("never-invent-cost violated: CostUSD %v", *ov.CostUSD)
	}
}

// A torn final write is the normal state of a transcript being appended to.
func TestMalformedLinesAreSkippedNotFatal(t *testing.T) {
	body := readFixture(t)
	body = append(body, []byte("this is not json at all\n")...)
	body = append(body, []byte(`{"type":"assistant","message":{"model":"claude-torn-`)...)

	home := newHome(t)
	writeSidecar(t, home)
	writeTranscript(t, home, body)

	ov := collectOne(t, New(), home)
	if ov.Model != wantModel {
		t.Fatalf("claude-torn-lines-are-skipped-not-fatal violated: model %q want %q", ov.Model, wantModel)
	}
	if ov.Title != wantTitle {
		t.Fatalf("claude-torn-lines-are-skipped-not-fatal violated: title %q want %q", ov.Title, wantTitle)
	}
	if ov.TokensUsed == nil || *ov.TokensUsed != wantTokens {
		t.Fatalf("claude-torn-lines-are-skipped-not-fatal violated: tokens %v want %d", derefTokens(ov.TokensUsed), wantTokens)
	}
}

// The live sessions dir holds <pid>.key beside every <pid>.json.
func TestSessionKeyFilesAreNotSessions(t *testing.T) {
	home := newHome(t)
	writeSidecar(t, home)
	key := filepath.Join(home, "sessions", "2890787.69288170cf99a2bd0e74a7a0e7152cf8b9c3d880e904e454fb18eed33c94b046.key")
	if err := os.WriteFile(key, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	ovs, err := New().Collect(home)
	if err != nil {
		t.Fatalf("claude-collect-survives-a-live-sessions-dir violated: %v", err)
	}
	if len(ovs) != 1 {
		t.Fatalf("claude-only-json-sidecars-are-sessions violated: got %d rows, want 1: %+v", len(ovs), ovs)
	}
}

// The fixtures are verbatim production captures, but a capture cannot prove the
// path encoding still finds files on a live tree. This runs the collector
// against the real ~/.claude when there is one. The canary: if there are live
// sessions and not one of them resolved a transcript, the collector has gone
// blind, and blind looks exactly like a quiet machine.
func TestLiveClaudeHomeCanary(t *testing.T) {
	home := filepath.Join(os.Getenv("HOME"), ".claude")
	if _, err := os.Stat(filepath.Join(home, "sessions")); err != nil {
		t.Skipf("no live sessions dir at %s: live canary not run (this is a skip, not a pass)", filepath.Join(home, "sessions"))
	}
	ovs, err := New().Collect(home)
	if err != nil {
		t.Fatalf("claude-collect-survives-the-live-tree violated: %v", err)
	}
	if len(ovs) == 0 {
		t.Skip("live sessions dir holds no sidecars: live canary not run (this is a skip, not a pass)")
	}
	withTranscript := 0
	for _, ov := range ovs {
		if ov.Model == syntheticModel {
			t.Fatalf("claude-synthetic-records-never-win-the-model violated on live data: pid %d session %q", ov.PID, ov.SessionID)
		}
		if ov.TokensUsed != nil && *ov.TokensUsed <= 0 {
			t.Fatalf("claude-tokens-are-last-turn-occupancy violated on live data: pid %d tokens %d", ov.PID, *ov.TokensUsed)
		}
		if ov.ContextWindow != nil {
			t.Fatalf("unknown-is-absent violated on live data: pid %d ContextWindow %d", ov.PID, *ov.ContextWindow)
		}
		if ov.Model != "" {
			withTranscript++
		}
	}
	if withTranscript == 0 {
		t.Errorf("live-transcript-canary violated: %d live sidecars and not one resolved a transcript; the project-dir encoding or the tail parse has gone blind", len(ovs))
	}
	t.Logf("live canary: %d sidecars, %d resolved a transcript", len(ovs), withTranscript)
}

// ---- helpers ----

type readSpy struct {
	opens int
	read  int64
}

type countingFile struct {
	transcriptFile
	sp *readSpy
}

func (f *countingFile) Read(p []byte) (int, error) {
	n, err := f.transcriptFile.Read(p)
	f.sp.read += int64(n)
	return n, err
}

func installSpy(c *Collector) *readSpy {
	sp := &readSpy{}
	c.openFn = func(path string) (transcriptFile, error) {
		f, err := openOS(path)
		if err != nil {
			return nil, err
		}
		sp.opens++
		return &countingFile{transcriptFile: f, sp: sp}, nil
	}
	return sp
}

func newHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "sessions"), 0755); err != nil {
		t.Fatal(err)
	}
	return home
}

func writeSidecar(t *testing.T, home string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "sidecar.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "sessions", "2890787.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
}

func writeTranscript(t *testing.T, home string, body []byte) string {
	t.Helper()
	dir := filepath.Join(home, "projects", EncodeProjectDir(fixtureCWD))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fixtureSession+".jsonl")
	if err := os.WriteFile(path, body, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func readFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "transcript.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		t.Fatalf("fixture-transcript-ends-with-a-newline violated: byte math in the tail test depends on it")
	}
	return raw
}

func fixtureLines(t *testing.T) [][]byte {
	t.Helper()
	return bytes.Split(bytes.TrimRight(readFixture(t), "\n"), []byte("\n"))
}

func collectOne(t *testing.T, c *Collector, home string) types.Overlay {
	t.Helper()
	ovs, err := c.Collect(home)
	if err != nil {
		t.Fatalf("claude-collect-does-not-error-on-a-well-formed-home violated: %v", err)
	}
	if len(ovs) != 1 {
		t.Fatalf("claude-one-row-per-sidecar violated: got %d rows want 1: %+v", len(ovs), ovs)
	}
	return ovs[0]
}

func derefTokens(p *int64) any {
	if p == nil {
		return "nil"
	}
	return *p
}
