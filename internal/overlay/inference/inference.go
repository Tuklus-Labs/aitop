// Package inference asks local model servers what they are doing. A local
// backend has no session file; its tokens live behind HTTP:
//
//   - llama-server: /slots (per-slot context, on by default on this build),
//     /props (alias, model path, whether /metrics is enabled), /metrics
//     (lifetime prompt/predicted tokens, only with --metrics).
//   - vLLM: /metrics (Prometheus, on by default: requests running, KV cache
//     fill, lifetime prompt/generation tokens), /v1/models (id, max_model_len).
//
// A busy llama-server answered /slots in >2s while generating, so probes run
// on their own goroutine and publish the last good answer; the overlay
// refresh never waits on a model server.
package inference

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aitop/internal/types"
)

// Server is one local model server found in /proc.
type Server struct {
	PID       int32
	StartTime uint64
	Kind      string // "llama" | "vllm"
	Host      string
	Port      int
}

// Discover scans procRoot for llama-server and vLLM processes and reads
// host/port from their argv. Defaults: llama 127.0.0.1:8080, vllm 127.0.0.1:8000.
func Discover(procRoot string) []Server {
	if procRoot == "" {
		procRoot = "/proc"
	}
	ents, err := os.ReadDir(procRoot)
	if err != nil {
		return nil
	}
	var out []Server
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "cmdline"))
		if err != nil || len(raw) == 0 {
			continue
		}
		argv := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		kind := KindOf(argv)
		if kind == "" {
			continue
		}
		host, port := HostPort(argv, kind)
		s := Server{PID: int32(pid), Kind: kind, Host: host, Port: port}
		if st, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "stat")); err == nil {
			if r := strings.LastIndexByte(string(st), ')'); r >= 0 {
				f := strings.Fields(string(st)[r+1:])
				if len(f) > 19 {
					s.StartTime, _ = strconv.ParseUint(f[19], 10, 64)
				}
			}
		}
		out = append(out, s)
	}
	return out
}

// KindOf recognizes a model server from argv: "llama", "vllm", or "".
func KindOf(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	base := path.Base(argv[0])
	switch {
	case base == "llama-server":
		return "llama"
	case base == "vllm" || strings.HasPrefix(base, "vllm"):
		return "vllm"
	}
	for _, a := range argv[1:] {
		if path.Base(a) == "vllm" || strings.Contains(a, "vllm.entrypoints") {
			return "vllm"
		}
		if path.Base(a) == "llama-server" {
			return "llama"
		}
	}
	return ""
}

// HostPort reads --host/--port (also --host=x, --port=N). A wildcard bind
// is probed on loopback.
func HostPort(argv []string, kind string) (string, int) {
	host := "127.0.0.1"
	port := 8080
	if kind == "vllm" {
		port = 8000
	}
	for i, a := range argv {
		switch {
		case a == "--host" && i+1 < len(argv):
			host = argv[i+1]
		case strings.HasPrefix(a, "--host="):
			host = strings.TrimPrefix(a, "--host=")
		case a == "--port" && i+1 < len(argv):
			port, _ = strconv.Atoi(argv[i+1])
		case strings.HasPrefix(a, "--port="):
			port, _ = strconv.Atoi(strings.TrimPrefix(a, "--port="))
		}
	}
	if host == "0.0.0.0" || host == "::" || host == "" || host == "*" {
		host = "127.0.0.1"
	}
	return host, port
}

// Poller probes servers on its own clock and keeps the last good overlays.
type Poller struct {
	ProcRoot string
	Client   *http.Client
	Interval time.Duration
	Discover func(string) []Server // tests inject

	latest atomic.Pointer[[]types.Overlay]
	mu     sync.Mutex
	static map[int32]staticInfo // per pid: things that do not change while it lives
}

type staticInfo struct {
	start     uint64
	model     string
	title     string // model file and quant, for servers with no unit description
	window    int64
	metricsOK bool
	checked   time.Time
}

func NewPoller(procRoot string) *Poller {
	return &Poller{
		ProcRoot: procRoot,
		Client:   &http.Client{Timeout: 1500 * time.Millisecond},
		Interval: time.Second,
		Discover: Discover,
		static:   map[int32]staticInfo{},
	}
}

// Latest returns the overlays from the most recent completed poll.
func (p *Poller) Latest() []types.Overlay {
	if v := p.latest.Load(); v != nil {
		return *v
	}
	return nil
}

// Start runs Poll on its own goroutine forever.
func (p *Poller) Start() {
	go func() {
		for {
			p.Poll()
			time.Sleep(p.Interval)
		}
	}()
}

// Poll probes every server once and publishes. Servers that do not answer
// keep whatever they last said for this poll only; a server gone from
// /proc is dropped.
func (p *Poller) Poll() {
	servers := p.Discover(p.ProcRoot)
	prev := map[int32]types.Overlay{}
	for _, o := range p.Latest() {
		prev[o.PID] = o
	}
	var out []types.Overlay
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, s := range servers {
		wg.Add(1)
		go func(s Server) {
			defer wg.Done()
			ov, ok := p.probe(s)
			if !ok {
				if old, had := prev[s.PID]; had {
					ov, ok = old, true
				}
			}
			if ok {
				mu.Lock()
				out = append(out, ov)
				mu.Unlock()
			}
		}(s)
	}
	wg.Wait()
	p.latest.Store(&out)
}

func (p *Poller) probe(s Server) (types.Overlay, bool) {
	base := "http://" + s.Host + ":" + strconv.Itoa(s.Port)
	ov := types.Overlay{PID: s.PID, StartTime: s.StartTime, Runtime: types.RuntimeLocal}
	switch s.Kind {
	case "llama":
		return p.probeLlama(base, s, ov)
	case "vllm":
		return p.probeVLLM(base, s, ov)
	}
	return ov, false
}

func (p *Poller) getJSON(url string, v any) bool {
	resp, err := p.Client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return false
	}
	return json.Unmarshal(body, v) == nil
}

// ---- llama-server ----

type llamaSlot struct {
	NCtx         int64 `json:"n_ctx"`
	IsProcessing bool  `json:"is_processing"`
	NPrompt      int64 `json:"n_prompt_tokens"`
	NextToken    []struct {
		NDecoded int64 `json:"n_decoded"`
	} `json:"next_token"`
}

type llamaProps struct {
	TotalSlots      int    `json:"total_slots"`
	ModelAlias      string `json:"model_alias"`
	ModelPath       string `json:"model_path"`
	ModelFtype      string `json:"model_ftype"`
	BuildInfo       string `json:"build_info"`
	EndpointMetrics bool   `json:"endpoint_metrics"`
	Defaults        struct {
		NCtx int64 `json:"n_ctx"`
	} `json:"default_generation_settings"`
}

func (p *Poller) probeLlama(base string, s Server, ov types.Overlay) (types.Overlay, bool) {
	st := p.staticFor(s)
	if st.model == "" || st.checked.IsZero() {
		var pr llamaProps
		if p.getJSON(base+"/props", &pr) {
			st.model = pr.ModelAlias
			if st.model == "" {
				st.model = modelBase(pr.ModelPath)
			}
			st.window = pr.Defaults.NCtx * int64(max(pr.TotalSlots, 1))
			st.title = modelBase(pr.ModelPath)
			if pr.ModelFtype != "" {
				st.title += " · " + pr.ModelFtype
			}
			if pr.BuildInfo != "" {
				st.title += " · llama.cpp " + pr.BuildInfo
			}
			st.metricsOK = pr.EndpointMetrics
			st.checked = time.Now()
			p.setStatic(s, st)
		}
	}
	var slots []llamaSlot
	if !p.getJSON(base+"/slots", &slots) {
		return ov, false
	}
	var used, window int64
	busy := 0
	for _, sl := range slots {
		n := sl.NPrompt
		if len(sl.NextToken) > 0 {
			n += sl.NextToken[0].NDecoded
		}
		used += n
		window += sl.NCtx
		if sl.IsProcessing {
			busy++
		}
	}
	if window == 0 {
		window = st.window
	}
	ov.Model = st.model
	ov.Title = st.title
	ov.TokensUsed = &used
	if window > 0 {
		w := window
		ov.ContextWindow = &w
	}
	ov.SubagentLive = busy
	ov.SubagentDeclared = len(slots)
	if busy > 0 {
		ov.Status = "busy"
	} else {
		ov.Status = "idle"
	}
	if st.metricsOK {
		if m, ok := p.metrics(base + "/metrics"); ok {
			ov.Usage = types.Usage{
				Input:  int64(m["llamacpp:prompt_tokens_total"]),
				Output: int64(m["llamacpp:tokens_predicted_total"]),
				Known:  true,
			}
		}
	}
	return ov, true
}

// ---- vLLM ----

type vllmModels struct {
	Data []struct {
		ID          string `json:"id"`
		MaxModelLen int64  `json:"max_model_len"`
	} `json:"data"`
}

func (p *Poller) probeVLLM(base string, s Server, ov types.Overlay) (types.Overlay, bool) {
	st := p.staticFor(s)
	if st.checked.IsZero() {
		var vm vllmModels
		if p.getJSON(base+"/v1/models", &vm) && len(vm.Data) > 0 {
			st.model = vm.Data[0].ID
			st.window = vm.Data[0].MaxModelLen
			st.checked = time.Now()
			p.setStatic(s, st)
		}
	}
	m, ok := p.metrics(base + "/metrics")
	if !ok {
		return ov, false
	}
	ov.Model = st.model
	if st.model != "" {
		ov.Title = "vLLM · " + st.model
	}
	if st.window > 0 {
		w := st.window
		ov.ContextWindow = &w
	}
	if fill, has := m["vllm:gpu_cache_usage_perc"]; has {
		f := fill
		ov.ContextFill = &f
	}
	running := int(m["vllm:num_requests_running"])
	ov.SubagentLive = running
	if running > 0 {
		ov.Status = "busy"
	} else {
		ov.Status = "idle"
	}
	in, hasIn := m["vllm:prompt_tokens_total"]
	out, hasOut := m["vllm:generation_tokens_total"]
	if hasIn || hasOut {
		ov.Usage = types.Usage{Input: int64(in), Output: int64(out), Known: true}
	}
	return ov, true
}

// metrics parses a Prometheus exposition into name -> value, summing series
// that share a name (labels dropped). Counters with no samples are absent.
func (p *Poller) metrics(url string) (map[string]float64, bool) {
	resp, err := p.Client.Get(url)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, false
	}
	out := map[string]float64{}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 4<<20))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		name := line
		if i := strings.IndexAny(line, "{ "); i >= 0 {
			name = line[:i]
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, err := strconv.ParseFloat(f[len(f)-1], 64)
		if err != nil {
			// "name{labels} value [timestamp]" with a timestamp: value is second-last
			if len(f) >= 3 {
				v, err = strconv.ParseFloat(f[len(f)-2], 64)
			}
			if err != nil {
				continue
			}
		}
		out[name] += v
	}
	return out, true
}

func (p *Poller) staticFor(s Server) staticInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	st, ok := p.static[s.PID]
	if !ok || st.start != s.StartTime {
		return staticInfo{start: s.StartTime}
	}
	return st
}

func (p *Poller) setStatic(s Server, st staticInfo) {
	st.start = s.StartTime
	p.mu.Lock()
	p.static[s.PID] = st
	p.mu.Unlock()
}

func modelBase(pth string) string {
	b := path.Base(pth)
	for _, ext := range []string{".gguf", ".safetensors", ".bin"} {
		b = strings.TrimSuffix(b, ext)
	}
	return b
}
