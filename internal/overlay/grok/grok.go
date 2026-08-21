package grok

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aitop/internal/join"
	"aitop/internal/types"
)

type activeEntry struct {
	SessionID string `json:"session_id"`
	PID       int32  `json:"pid"`
	CWD       string `json:"cwd"`
	OpenedAt  string `json:"opened_at"`
}

func Collect(grokHome string) ([]types.Overlay, error) {
	raw, err := os.ReadFile(filepath.Join(grokHome, "active_sessions.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var active []activeEntry
	if err := json.Unmarshal(raw, &active); err != nil {
		// map form?
		var m map[string]activeEntry
		if err2 := json.Unmarshal(raw, &m); err2 != nil {
			return nil, err
		}
		for _, e := range m {
			active = append(active, e)
		}
	}

	var out []types.Overlay
	seen := map[string]bool{}
	for _, e := range active {
		if e.SessionID == "" {
			continue
		}
		ov, children, err := loadSession(grokHome, e)
		if err != nil {
			continue
		}
		out = append(out, ov)
		out = append(out, children...)
		seen[e.SessionID] = true
	}
	_ = seen
	return out, nil
}

func loadSession(grokHome string, e activeEntry) (types.Overlay, []types.Overlay, error) {
	enc := encodeCWD(e.CWD)
	dir := filepath.Join(grokHome, "sessions", enc, e.SessionID)
	sumRaw, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		return types.Overlay{}, nil, err
	}
	var sum struct {
		Info struct {
			ID  string `json:"id"`
			CWD string `json:"cwd"`
		} `json:"info"`
		GeneratedTitle string `json:"generated_title"`
		CurrentModel   string `json:"current_model_id"`
		AgentName      string `json:"agent_name"`
		SessionKind    string `json:"session_kind"`
		LastActive     string `json:"last_active_at"`
	}
	if err := json.Unmarshal(sumRaw, &sum); err != nil {
		return types.Overlay{}, nil, err
	}
	cwd := sum.Info.CWD
	if cwd == "" {
		cwd = e.CWD
	}
	ov := types.Overlay{
		SessionID:   e.SessionID,
		PID:         e.PID,
		Runtime:     types.RuntimeGrok,
		Project:     join.ProjectName(cwd),
		Model:       sum.CurrentModel,
		Title:       sum.GeneratedTitle,
		SessionPath: dir,
		OverlayCWD:  cwd,
	}
	if t, err := time.Parse(time.RFC3339Nano, sum.LastActive); err == nil {
		ov.UpdatedAt = t
	}
	if (sum.SessionKind == "" || !strings.HasPrefix(sum.SessionKind, "subagent")) &&
		strings.HasPrefix(sum.AgentName, "grok-build") {
		ov.ProvenName = "Grok"
	}
	if sigRaw, err := os.ReadFile(filepath.Join(dir, "signals.json")); err == nil {
		var sig struct {
			Tokens  int64   `json:"contextTokensUsed"`
			Window  int64   `json:"contextWindowTokens"`
			Usage   float64 `json:"contextWindowUsage"`
			Primary string  `json:"primaryModelId"`
		}
		if json.Unmarshal(sigRaw, &sig) == nil {
			if sig.Tokens != 0 {
				v := sig.Tokens
				ov.TokensUsed = &v
			}
			if sig.Window != 0 {
				w := sig.Window
				ov.ContextWindow = &w
			}
			if sig.Usage != 0 {
				f := sig.Usage / 100.0
				ov.ContextFill = &f
			}
			if ov.Model == "" {
				ov.Model = sig.Primary
			}
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, e.OpenedAt); err == nil {
		ov.StartedAt = t
	}
	subs, err := os.ReadDir(filepath.Join(dir, "subagents"))
	var children []types.Overlay
	if err == nil {
		for _, s := range subs {
			if !s.IsDir() {
				continue
			}
			mb, err := os.ReadFile(filepath.Join(dir, "subagents", s.Name(), "meta.json"))
			if err != nil {
				continue
			}
			var meta struct {
				ID        string `json:"subagent_id"`
				Parent    string `json:"parent_session_id"`
				Child     string `json:"child_session_id"`
				Status    string `json:"status"`
				Desc      string `json:"description"`
				Model     string `json:"effective_model_id"`
				Type      string `json:"subagent_type"`
				Started   string `json:"started_at"`
				Completed string `json:"completed_at"`
			}
			if json.Unmarshal(mb, &meta) != nil {
				continue
			}
			ov.SubagentDeclared++
			if meta.Status == "running" {
				ov.SubagentLive++
			}
			c := types.Overlay{
				SessionID:      meta.Child,
				Runtime:        types.RuntimeGrok,
				ParentSession:  e.SessionID,
				SubagentID:     meta.ID,
				SubagentStatus: meta.Status,
				SubagentType:   meta.Type,
				Title:          meta.Desc,
				Model:          meta.Model,
			}
			if t, err := time.Parse(time.RFC3339Nano, meta.Started); err == nil {
				c.StartedAt = t
			}
			if t, err := time.Parse(time.RFC3339Nano, meta.Completed); err == nil {
				c.CompletedAt = t
			}
			children = append(children, c)
		}
	}
	return ov, children, nil
}

func encodeCWD(cwd string) string {
	if cwd == "" {
		return ""
	}
	return strings.ReplaceAll(cwd, "/", "%2F")
}
