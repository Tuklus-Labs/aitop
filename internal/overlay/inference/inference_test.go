package inference

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

// Real shapes captured from the live llama-server (b10669) on 2026-08-22.
const slotsBusy = `[{"id":0,"n_ctx":32768,"speculative":false,"is_processing":true,"id_task":514,"n_prompt_tokens":516,"n_prompt_tokens_processed":0,"n_prompt_tokens_cache":0,"params":{},"next_token":[{"has_next_token":false,"has_new_line":false,"n_remain":512,"n_decoded":40}]},{"id":1,"n_ctx":32768,"is_processing":false,"n_prompt_tokens":1000,"next_token":[{"n_decoded":0}]}]`
const props = `{"default_generation_settings":{"n_ctx":32768,"params":{}},"total_slots":2,"model_alias":"iris-vllm","model_ftype":"MXFP4 MoE","model_path":"/home/aegis/Models/DeepSeek-V4-Flash.gguf","endpoint_slots":true,"endpoint_props":false,"endpoint_metrics":false,"build_info":"b10669-a1d323f05","is_sleeping":false}`
const llamaMetrics = "# HELP llamacpp:prompt_tokens_total Number of prompt tokens processed.\n# TYPE llamacpp:prompt_tokens_total counter\nllamacpp:prompt_tokens_total 123456\nllamacpp:tokens_predicted_total 7890\nllamacpp:kv_cache_tokens 556\n"
const vllmMetrics = `# HELP vllm:num_requests_running Number of requests currently running on GPU.
vllm:num_requests_running{model_name="Qwen/Qwen3-32B"} 2.0
vllm:gpu_cache_usage_perc{model_name="Qwen/Qwen3-32B"} 0.37
vllm:prompt_tokens_total{model_name="Qwen/Qwen3-32B"} 5000000.0
vllm:generation_tokens_total{model_name="Qwen/Qwen3-32B"} 250000.0
`
const vllmModelsJSON = `{"object":"list","data":[{"id":"Qwen/Qwen3-32B","object":"model","max_model_len":40960}]}`

func serverOf(t *testing.T, srv *httptest.Server, kind string, pid int32) Server {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	return Server{PID: pid, StartTime: 7, Kind: kind, Host: u.Hostname(), Port: port}
}

func pollerWith(servers ...Server) *Poller {
	p := NewPoller("")
	p.Discover = func(string) []Server { return servers }
	return p
}

func TestLlamaSlotsGiveContextOccupancyAndStatus(t *testing.T) {
	var metricsHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slots":
			w.Write([]byte(slotsBusy))
		case "/props":
			w.Write([]byte(props))
		case "/metrics":
			metricsHits.Add(1)
			w.WriteHeader(501)
			w.Write([]byte(`{"error":{"code":501,"message":"This server does not support metrics endpoint"}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	p := pollerWith(serverOf(t, srv, "llama", 1298775))
	p.Poll(context.Background())
	ovs := p.Latest()
	o, slots := splitServerSlots(ovs)
	if o.PID != 1298775 {
		t.Fatalf("llama-server-gets-one-overlay violated: server=%+v n=%d", o, len(ovs))
	}
	if o.TokensUsed == nil || *o.TokensUsed != 516+40+1000 {
		t.Fatalf("llama-tokens-are-prompt-plus-decoded-over-slots violated: %v", o.TokensUsed)
	}
	if o.ContextWindow == nil || *o.ContextWindow != 65536 {
		t.Fatalf("llama-window-is-sum-of-slot-n-ctx violated: %v", o.ContextWindow)
	}
	if o.Status != "busy" || o.SubagentLive != 1 || o.SubagentDeclared != 2 {
		t.Fatalf("llama-status-from-is-processing violated: status=%q live=%d slots=%d", o.Status, o.SubagentLive, o.SubagentDeclared)
	}
	if o.Model != "iris-vllm" {
		t.Fatalf("llama-model-is-alias violated: %q", o.Model)
	}
	if o.Title != "DeepSeek-V4-Flash · MXFP4 MoE · llama.cpp b10669-a1d323f05" {
		t.Fatalf("llama-title-is-model-file-quant-build violated: %q", o.Title)
	}
	if o.Usage.Known {
		t.Fatal("llama-no-metrics-means-no-lifetime violated: usage invented without --metrics")
	}
	if metricsHits.Load() != 0 {
		t.Fatalf("llama-respects-endpoint_metrics-false violated: /metrics probed %d times", metricsHits.Load())
	}
	if o.Runtime != types.RuntimeLocal || o.PID != 1298775 {
		t.Fatalf("overlay-targets-the-server-pid violated: %+v", o)
	}
	if len(slots) != o.SubagentDeclared || len(slots) != 2 {
		t.Fatalf("llama-emits-one-overlay-per-declared-slot violated: slots=%d declared=%d", len(slots), o.SubagentDeclared)
	}
	wantParent := "local-pid:1298775"
	for i, sl := range slots {
		if sl.PID != 0 || sl.Kind != "slot" || sl.ParentSession != wantParent || sl.SlotIndex == nil {
			t.Fatalf("slot-overlay-is-child-not-server violated: i=%d %+v", i, sl)
		}
		if sl.TokPerSec != nil {
			t.Fatalf("tok-per-sec-first-sample-is-absent violated: i=%d v=%v", i, *sl.TokPerSec)
		}
	}
}

func splitServerSlots(ovs []types.Overlay) (types.Overlay, []types.Overlay) {
	var server types.Overlay
	var slots []types.Overlay
	for _, o := range ovs {
		if o.Kind == "slot" {
			slots = append(slots, o)
			continue
		}
		if o.PID != 0 {
			server = o
		}
	}
	return server, slots
}

func TestLlamaSlotTokPerSecIsDecodeDelta(t *testing.T) {
	var decoded atomic.Int64
	decoded.Store(40)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slots":
			n := decoded.Load()
			fmt.Fprintf(w, `[{"id":0,"n_ctx":32768,"is_processing":true,"n_prompt_tokens":516,"next_token":[{"n_decoded":%d}]},{"id":1,"n_ctx":32768,"is_processing":false,"n_prompt_tokens":1000,"next_token":[{"n_decoded":0}]}]`, n)
		case "/props":
			w.Write([]byte(props))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	p := pollerWith(serverOf(t, srv, "llama", 10))
	t0 := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return t0 }
	p.Poll(context.Background())
	_, slots := splitServerSlots(p.Latest())
	if len(slots) != 2 {
		t.Fatalf("slot-count-before-tok-per-sec violated: %d", len(slots))
	}
	busy := slotByIndex(slots, 0)
	if busy.TokPerSec != nil {
		t.Fatalf("tok-per-sec-first-sample-is-absent violated: %v", *busy.TokPerSec)
	}
	decoded.Store(140)
	p.now = func() time.Time { return t0.Add(time.Second) }
	p.Poll(context.Background())
	_, slots = splitServerSlots(p.Latest())
	busy = slotByIndex(slots, 0)
	if busy.TokPerSec == nil || *busy.TokPerSec <= 0 {
		t.Fatalf("tok-per-sec-is-decode-delta violated: %+v", busy.TokPerSec)
	}
	idle := slotByIndex(slots, 1)
	if idle.TokPerSec != nil {
		t.Fatalf("idle-slot-tok-per-sec-stays-absent violated: %v", *idle.TokPerSec)
	}
}

func slotByIndex(slots []types.Overlay, want int) types.Overlay {
	for _, sl := range slots {
		if sl.SlotIndex != nil && *sl.SlotIndex == want {
			return sl
		}
	}
	return types.Overlay{}
}

func TestLlamaMetricsGiveLifetimeWhenEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slots":
			w.Write([]byte(slotsBusy))
		case "/props":
			w.Write([]byte(`{"default_generation_settings":{"n_ctx":32768},"total_slots":2,"model_alias":"","model_path":"/m/Qwen3.8-27B-Q4_K_M.gguf","endpoint_metrics":true}`))
		case "/metrics":
			w.Write([]byte(llamaMetrics))
		}
	}))
	defer srv.Close()
	p := pollerWith(serverOf(t, srv, "llama", 10))
	p.Poll(context.Background())
	o, _ := splitServerSlots(p.Latest())
	if !o.Usage.Known || o.Usage.Input != 123456 || o.Usage.Output != 7890 {
		t.Fatalf("llama-metrics-lifetime violated: %+v", o.Usage)
	}
	if o.Model != "Qwen3.8-27B-Q4_K_M" {
		t.Fatalf("llama-model-falls-back-to-path-basename violated: %q", o.Model)
	}
}

func TestVLLMMetricsAndModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metrics":
			w.Write([]byte(vllmMetrics))
		case "/v1/models":
			w.Write([]byte(vllmModelsJSON))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	p := pollerWith(serverOf(t, srv, "vllm", 20))
	p.Poll(context.Background())
	o, _ := splitServerSlots(p.Latest())
	if o.Model != "Qwen/Qwen3-32B" || o.ContextWindow == nil || *o.ContextWindow != 40960 {
		t.Fatalf("vllm-model-and-window-from-v1-models violated: %q %v", o.Model, o.ContextWindow)
	}
	if o.ContextFill == nil || *o.ContextFill != 0.37 {
		t.Fatalf("vllm-ctx-fill-is-kv-cache-usage violated: %v", o.ContextFill)
	}
	if o.Status != "busy" || o.SubagentLive != 2 {
		t.Fatalf("vllm-busy-from-requests-running violated: %q %d", o.Status, o.SubagentLive)
	}
	if !o.Usage.Known || o.Usage.Input != 5000000 || o.Usage.Output != 250000 {
		t.Fatalf("vllm-lifetime-from-counters violated: %+v", o.Usage)
	}
	if o.TokensUsed != nil {
		t.Fatal("vllm-has-no-per-token-occupancy violated: TokensUsed invented")
	}
}

func TestSlowServerKeepsLastAnswerAndDoesNotBlockLatest(t *testing.T) {
	var slow atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slow.Load() {
			time.Sleep(400 * time.Millisecond)
			return
		}
		switch r.URL.Path {
		case "/slots":
			w.Write([]byte(slotsBusy))
		case "/props":
			w.Write([]byte(props))
		}
	}))
	defer srv.Close()
	p := pollerWith(serverOf(t, srv, "llama", 30))
	p.Client.Timeout = 100 * time.Millisecond
	p.Poll(context.Background())
	first, firstSlots := splitServerSlots(p.Latest())
	if first.PID != 30 || first.TokensUsed == nil || len(firstSlots) != 2 {
		t.Fatalf("setup: first poll failed: server=%+v slots=%d", first, len(firstSlots))
	}
	slow.Store(true)
	t0 := time.Now()
	p.Poll(context.Background())
	if d := time.Since(t0); d > 300*time.Millisecond {
		t.Fatalf("inference-poll-bounded-by-client-timeout violated: %v", d)
	}
	kept, keptSlots := splitServerSlots(p.Latest())
	if kept.TokensUsed == nil || len(keptSlots) != 2 {
		t.Fatalf("inference-slow-server-keeps-last-answer violated: server=%+v slots=%d", kept, len(keptSlots))
	}
	t1 := time.Now()
	_ = p.Latest()
	if time.Since(t1) > 10*time.Millisecond {
		t.Fatal("inference-latest-never-blocks violated")
	}
	// A server gone from /proc is dropped, not kept forever.
	p.Discover = func(string) []Server { return nil }
	p.Poll(context.Background())
	if len(p.Latest()) != 0 {
		t.Fatal("inference-dead-server-dropped violated")
	}
}

func TestKindAndHostPortFromArgv(t *testing.T) {
	llama := []string{"/home/aegis/Projects/llama-cpp-turboquant/build-sync/bin/llama-server", "-m", "x.gguf", "-a", "iris-vllm", "--port", "18099", "--host", "127.0.0.1"}
	if KindOf(llama) != "llama" {
		t.Fatal("kind-llama violated")
	}
	if h, p := HostPort(llama, "llama"); h != "127.0.0.1" || p != 18099 {
		t.Fatalf("hostport-llama violated: %s %d", h, p)
	}
	vllm := []string{"/opt/venv/bin/python", "/opt/venv/bin/vllm", "serve", "Qwen/Qwen3-32B", "--host=0.0.0.0", "--port=8123"}
	if KindOf(vllm) != "vllm" {
		t.Fatal("kind-vllm-via-console-script violated")
	}
	if h, p := HostPort(vllm, "vllm"); h != "127.0.0.1" || p != 8123 {
		t.Fatalf("hostport-vllm-wildcard-probes-loopback violated: %s %d", h, p)
	}
	if h, p := HostPort([]string{"vllm", "serve", "m"}, "vllm"); h != "127.0.0.1" || p != 8000 {
		t.Fatalf("hostport-vllm-default-8000 violated: %s %d", h, p)
	}
	if KindOf([]string{"python", "-m", "vllm.entrypoints.openai.api_server"}) != "vllm" {
		t.Fatal("kind-vllm-via-module violated")
	}
	if KindOf([]string{"claude"}) != "" {
		t.Fatal("kind-not-a-server violated")
	}
}

func TestInferencePollerCancelsInFlightRequest(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-started:
		default:
			close(started)
		}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	p := pollerWith(serverOf(t, srv, "llama", 40))
	p.Client.Timeout = 0
	ctx, cancel := context.WithCancel(context.Background())
	pollDone := make(chan struct{})
	go func() {
		p.Poll(ctx)
		close(pollDone)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatalf("inference-poller-cancels-in-flight-request rule violated: handler never saw request timeout=5s clientTimeout=%s", p.Client.Timeout)
	}
	cancel()
	select {
	case <-pollDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("inference-poller-cancels-in-flight-request rule violated: Poll did not return after cancel timeout=5s clientTimeout=%s", p.Client.Timeout)
	}
}
