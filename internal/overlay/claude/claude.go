package claude

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"aitop/internal/join"
	"aitop/internal/types"
)

// tailBytes is the most of any transcript we ever read. Transcripts run to many
// MB; the occupancy of the current turn lives in the last few records.
const tailBytes = 256 * 1024

// maxTail caps how far the window widens when the tail holds no assistant
// record (a 200 KB tool result can fill the whole first window). Past this we
// accept the miss; the previous parse's values stay via the cache merge.
const maxTail = 4 * 1024 * 1024

// maxLine is the scanner token ceiling. No line inside a tailBytes window can
// exceed it, so ErrTooLong is unreachable today; the ceiling is there for the
// day tailBytes grows.
const maxLine = 1024 * 1024

// syntheticModel marks records the CLI wrote on its own behalf (refusals, API
// errors). They carry an all-zero usage block, so letting one win would report
// a live session as model "<synthetic>" holding zero tokens.
const syntheticModel = "<synthetic>"

type sidecar struct {
	PID        json.RawMessage `json:"pid"`
	SessionID  string          `json:"sessionId"`
	CWD        string          `json:"cwd"`
	ProcStart  json.RawMessage `json:"procStart"`
	Kind       string          `json:"kind"`
	Entrypoint string          `json:"entrypoint"`
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	StartedAt  json.RawMessage `json:"startedAt"`
	UpdatedAt  json.RawMessage `json:"updatedAt"`
}

// transcript is one parse of a transcript tail. Plain values, never pointers:
// the cache hands out copies, so no row can mutate another row's numbers.
type transcript struct {
	model     string
	tokens    int64
	hasTokens bool
	title     string
	cwd       string
	branch    string
	effort    string
}

type cacheEntry struct {
	size  int64
	mtime time.Time
	res   transcript
}

// transcriptFile is the seam the collector reads transcripts through. Tests
// substitute a counting implementation to observe opens and bytes read.
type transcriptFile interface {
	io.Reader
	io.Seeker
	io.Closer
	Stat() (os.FileInfo, error)
}

// Collector holds the transcript parse cache. The overlay refresh runs about
// once a second; re-parsing every transcript on every tick is pure waste, so a
// path whose size and mtime are unchanged is served from the last parse.
type Collector struct {
	mu     sync.Mutex
	cache  map[string]cacheEntry
	openFn func(string) (transcriptFile, error)
	statFn func(string) (os.FileInfo, error)
}

func New() *Collector {
	return &Collector{
		cache:  map[string]cacheEntry{},
		openFn: openOS,
		statFn: os.Stat,
	}
}

func openOS(path string) (transcriptFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}

var shared = New()

// Collect reads every <claudeHome>/sessions/<pid>.json sidecar and left-joins
// the tail of each session transcript onto it. A missing or unreadable
// transcript is absence, not a dropped row: the sidecar-only overlay stands.
func Collect(claudeHome string) ([]types.Overlay, error) { return shared.Collect(claudeHome) }

func (c *Collector) Collect(claudeHome string) ([]types.Overlay, error) {
	dir := filepath.Join(claudeHome, "sessions")
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	live := make(map[string]bool, len(ents))
	var out []types.Overlay
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var s sidecar
		if json.Unmarshal(raw, &s) != nil {
			continue
		}
		pid, ok := parseUint32(s.PID)
		if !ok || pid == 0 {
			continue
		}
		start, _ := parseUint64(s.ProcStart)
		ov := types.Overlay{
			SessionID:   s.SessionID,
			PID:         int32(pid),
			StartTime:   start,
			Runtime:     types.RuntimeClaude,
			Project:     join.ProjectName(s.CWD),
			OverlayCWD:  s.CWD,
			SessionName: s.Name,
			Status:      strings.ToLower(strings.TrimSpace(s.Status)),
			StartedAt:   msTime(s.StartedAt),
			UpdatedAt:   msTime(s.UpdatedAt),
		}
		if path := transcriptPath(claudeHome, s); path != "" {
			live[path] = true
			if t, ok := c.transcript(path); ok {
				applyTranscript(&ov, t)
			}
		}
		out = append(out, ov)
	}
	c.sweep(live)
	return out, nil
}

// transcriptPath is where the CLI keeps this session's JSONL: the project dir
// is the launch cwd with every non-alphanumeric byte replaced.
func transcriptPath(claudeHome string, s sidecar) string {
	if s.SessionID == "" || s.CWD == "" {
		return ""
	}
	if strings.ContainsAny(s.SessionID, `/\`) || strings.Contains(s.SessionID, "..") {
		return ""
	}
	return filepath.Join(claudeHome, "projects", EncodeProjectDir(s.CWD), s.SessionID+".jsonl")
}

// EncodeProjectDir maps a cwd to the CLI's project directory name: every byte
// that is not [A-Za-z0-9] becomes '-'. Verified against the live directory,
// including the double dash a dotfile path produces.
func EncodeProjectDir(cwd string) string {
	b := []byte(cwd)
	for i := 0; i < len(b); i++ {
		ch := b[i]
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
		default:
			b[i] = '-'
		}
	}
	return string(b)
}

func (c *Collector) transcript(path string) (transcript, bool) {
	st, err := c.statFn(path)
	if err != nil || st.IsDir() {
		return transcript{}, false
	}
	c.mu.Lock()
	ent, hit := c.cache[path]
	c.mu.Unlock()
	if hit && ent.size == st.Size() && ent.mtime.Equal(st.ModTime()) {
		return ent.res, true
	}
	t, size, mtime, err := c.parse(path)
	if err != nil {
		return transcript{}, false
	}
	if hit {
		// Append-only file: a value the previous parse saw is still the last
		// one until a newer record replaces it. A window that happens to
		// hold only tool output must not blank a known model or token count.
		t = t.mergeOnto(ent.res)
	}
	c.mu.Lock()
	c.cache[path] = cacheEntry{size: size, mtime: mtime, res: t}
	c.mu.Unlock()
	return t, true
}

// mergeOnto fills fields this parse did not see from an older parse of the
// same file.
func (t transcript) mergeOnto(old transcript) transcript {
	if t.model == "" {
		t.model = old.model
	}
	if !t.hasTokens && old.hasTokens {
		t.tokens, t.hasTokens = old.tokens, true
	}
	if t.title == "" {
		t.title = old.title
	}
	if t.cwd == "" {
		t.cwd = old.cwd
	}
	if t.branch == "" {
		t.branch = old.branch
	}
	if t.effort == "" {
		t.effort = old.effort
	}
	return t
}

func (c *Collector) parse(path string) (transcript, int64, time.Time, error) {
	f, err := c.openFn(path)
	if err != nil {
		return transcript{}, 0, time.Time{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return transcript{}, 0, time.Time{}, err
	}
	size := st.Size()
	// Read the last window. If it holds no assistant record, widen and read
	// again from the same handle, up to maxTail.
	var t transcript
	for window := int64(tailBytes); ; window *= 4 {
		t = transcript{}
		var off int64
		if size > window {
			off = size - window
		}
		if _, err := f.Seek(off, io.SeekStart); err != nil {
			return transcript{}, 0, time.Time{}, err
		}
		data, err := io.ReadAll(f)
		if err != nil {
			return transcript{}, 0, time.Time{}, err
		}
		if off > 0 {
			// The window opened mid-record: everything up to and including
			// the first newline is the tail of a line whose head we never saw.
			if i := bytes.IndexByte(data, '\n'); i >= 0 {
				data = data[i+1:]
			} else {
				data = nil
			}
		}
		scan(data, &t)
		if t.model != "" || off == 0 || window >= maxTail {
			break
		}
	}
	return t, size, st.ModTime(), nil
}

// scan splits on newlines by hand: bufio.Scanner stops dead at a line over
// its ceiling, which would drop every record after one giant tool result.
// An oversized line is skipped on its own.
func scan(data []byte, t *transcript) {
	for len(data) > 0 {
		var line []byte
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i], data[i+1:]
		} else {
			line, data = data, nil
		}
		if len(line) > maxLine {
			continue
		}
		parseLine(line, t)
	}
}

func parseLine(line []byte, t *transcript) {
	if len(line) == 0 || line[0] != '{' {
		return
	}
	var rec struct {
		Type      string          `json:"type"`
		AITitle   string          `json:"aiTitle"`
		CWD       string          `json:"cwd"`
		GitBranch string          `json:"gitBranch"`
		Effort    string          `json:"effort"`
		Message   json.RawMessage `json:"message"`
	}
	// A line we cannot read is skipped, never fatal: a torn write at the end of
	// a live transcript must not cost us the records before it.
	if json.Unmarshal(line, &rec) != nil {
		return
	}
	// Last non-empty wins. The sidecar holds the launch cwd; the transcript
	// holds where the agent actually is.
	if rec.CWD != "" {
		t.cwd = rec.CWD
	}
	if rec.GitBranch != "" {
		t.branch = rec.GitBranch
	}
	switch rec.Type {
	case "ai-title":
		if rec.AITitle != "" {
			t.title = rec.AITitle
		}
	case "assistant":
		var msg struct {
			Model string `json:"model"`
			Usage *struct {
				Input     int64 `json:"input_tokens"`
				CacheCrea int64 `json:"cache_creation_input_tokens"`
				CacheRead int64 `json:"cache_read_input_tokens"`
			} `json:"usage"`
		}
		if len(rec.Message) == 0 || json.Unmarshal(rec.Message, &msg) != nil {
			return
		}
		if msg.Model == "" || msg.Model == syntheticModel {
			return
		}
		t.model = msg.Model
		if rec.Effort != "" {
			t.effort = rec.Effort
		}
		if msg.Usage == nil {
			return
		}
		// Context occupancy of the last turn, not a lifetime sum: the three
		// input families are what is resident in the window. output_tokens is
		// spend, not occupancy, and is excluded.
		n := msg.Usage.Input + msg.Usage.CacheCrea + msg.Usage.CacheRead
		if n > 0 {
			t.tokens = n
			t.hasTokens = true
		}
	}
}

func applyTranscript(ov *types.Overlay, t transcript) {
	if t.model != "" {
		ov.Model = t.model
	}
	if t.hasTokens {
		n := t.tokens
		ov.TokensUsed = &n
	}
	if t.title != "" {
		ov.Title = t.title
	}
	if t.cwd != "" {
		ov.OverlayCWD = t.cwd
		ov.Project = join.ProjectName(t.cwd)
	}
	if t.branch != "" {
		ov.Branch = t.branch
	}
	if t.effort != "" {
		ov.Effort = t.effort
	}
	// ContextWindow and CostUSD stay nil. No file here carries either, and a
	// window guessed from a model name is a number we made up.
}

// sweep drops cache entries for sessions that no longer have a sidecar, so a
// long-lived TUI does not accumulate parses of dead sessions.
func (c *Collector) sweep(live map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for p := range c.cache {
		if !live[p] {
			delete(c.cache, p)
		}
	}
}

func msTime(raw json.RawMessage) time.Time {
	ms, ok := parseUint64(raw)
	if !ok || ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms))
}

func parseUint32(raw json.RawMessage) (uint32, bool) {
	v, ok := parseUint64(raw)
	return uint32(v), ok
}

func parseUint64(raw json.RawMessage) (uint64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n uint64
	if json.Unmarshal(raw, &n) == nil {
		return n, true
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		n, err := strconv.ParseUint(s, 10, 64)
		return n, err == nil
	}
	return 0, false
}
