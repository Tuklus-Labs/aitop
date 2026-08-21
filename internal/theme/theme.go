// Package theme loads btop color themes so aitop draws in the same ink as the
// btop running next to it.
//
// Everything here deals in "#RRGGBB" strings. No terminal library is imported;
// rendering is the UI layer's problem.
package theme

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Gradient3 is start, mid, end. Mid is synthesized as the midpoint when the
// file omits it, so index 1 is never empty.
type Gradient3 [3]string

// Theme is a btop palette. Every field is "#RRGGBB" (uppercase hex), never empty.
type Theme struct {
	Name   string // file basename without .theme, or "nightfable" for the built-in
	Source string // human-readable origin: "built-in", "flag <path>", "$AITOP_THEME <path>", "btop.conf <path>"

	MainBg, MainFg, Title, HiFg, SelectedBg, SelectedFg, InactiveFg, GraphText, MeterBg, ProcMisc string

	Box, DivLine string // Box = proc_box (fall back cpu_box, then mem_box)

	CPU, Temp, Used, Free, Cached, Available, Download, Upload, Process Gradient3
}

// Nightfable is the built-in fallback: the palette aitop ships with, matching
// ~/.config/btop/themes/nightfable.theme.
func Nightfable() Theme {
	return Theme{
		Name:   "nightfable",
		Source: "built-in",

		MainBg:     "#0E0B09",
		MainFg:     "#C2B49A",
		Title:      "#E8DCC4",
		HiFg:       "#C8502E",
		SelectedBg: "#201913",
		SelectedFg: "#C8502E",
		InactiveFg: "#847862",
		GraphText:  "#C2B49A",
		MeterBg:    "#201913",
		ProcMisc:   "#748CA6",

		Box:     "#33291F",
		DivLine: "#201913",

		Temp:      Gradient3{"#5E8C7B", "#C9963C", "#E0563A"},
		CPU:       Gradient3{"#5E8C7B", "#C9963C", "#E0563A"},
		Free:      Gradient3{"#201913", "#33291F", "#847862"},
		Cached:    Gradient3{"#748CA6", "#748CA6", "#5E8C7B"},
		Available: Gradient3{"#33291F", "#847862", "#C2B49A"},
		Used:      Gradient3{"#847862", "#C9963C", "#C8502E"},
		Download:  Gradient3{"#3E5E52", "#5E8C7B", "#7BA897"},
		Upload:    Gradient3{"#8A3A22", "#C8502E", "#E0563A"},
		Process:   Gradient3{"#5E8C7B", "#C9963C", "#C8502E"},
	}
}

// scalarFields binds every single-color btop key to its Theme field. Box is not
// here: it has a fallback chain of its own.
var scalarFields = []struct {
	key string
	ptr func(*Theme) *string
}{
	{"main_bg", func(t *Theme) *string { return &t.MainBg }},
	{"main_fg", func(t *Theme) *string { return &t.MainFg }},
	{"title", func(t *Theme) *string { return &t.Title }},
	{"hi_fg", func(t *Theme) *string { return &t.HiFg }},
	{"selected_bg", func(t *Theme) *string { return &t.SelectedBg }},
	{"selected_fg", func(t *Theme) *string { return &t.SelectedFg }},
	{"inactive_fg", func(t *Theme) *string { return &t.InactiveFg }},
	{"graph_text", func(t *Theme) *string { return &t.GraphText }},
	{"meter_bg", func(t *Theme) *string { return &t.MeterBg }},
	{"proc_misc", func(t *Theme) *string { return &t.ProcMisc }},
	{"div_line", func(t *Theme) *string { return &t.DivLine }},
}

// gradientFields binds every btop <prefix>_start/_mid/_end triple to its field.
var gradientFields = []struct {
	prefix string
	ptr    func(*Theme) *Gradient3
}{
	{"cpu", func(t *Theme) *Gradient3 { return &t.CPU }},
	{"temp", func(t *Theme) *Gradient3 { return &t.Temp }},
	{"used", func(t *Theme) *Gradient3 { return &t.Used }},
	{"free", func(t *Theme) *Gradient3 { return &t.Free }},
	{"cached", func(t *Theme) *Gradient3 { return &t.Cached }},
	{"available", func(t *Theme) *Gradient3 { return &t.Available }},
	{"download", func(t *Theme) *Gradient3 { return &t.Download }},
	{"upload", func(t *Theme) *Gradient3 { return &t.Upload }},
	{"process", func(t *Theme) *Gradient3 { return &t.Process }},
}

// boxKeys is the Box fallback chain, most specific first.
var boxKeys = []string{"proc_box", "cpu_box", "mem_box"}

// ParseBtop reads theme[key]=value lines out of a btop theme file.
//
// Keys are lowercased; unknown ones are kept, because a caller may want them and
// dropping them here would be a silent edit. Values are normalized to uppercase
// "#RRGGBB". Anything that is not a color btop would accept is dropped without
// an error: an empty value means "use the default" in btop, and a malformed one
// is a typo in someone's theme file, not a reason to refuse the whole palette.
// The only error returned is a read error from r.
func ParseBtop(r io.Reader) (map[string]string, error) {
	out := make(map[string]string)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		key, raw, ok := splitThemeLine(sc.Text())
		if !ok {
			continue
		}
		if color, ok := normalizeColor(raw); ok {
			out[key] = color
		}
	}
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("theme: read: %w", err)
	}
	return out, nil
}

// splitThemeLine pulls key and unvalidated value out of one `theme[key]=value` line.
func splitThemeLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), "\r"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	const prefix = "theme["
	if !strings.HasPrefix(line, prefix) {
		return "", "", false
	}
	rest := line[len(prefix):]
	end := strings.IndexByte(rest, ']')
	if end < 0 {
		return "", "", false
	}
	key = strings.ToLower(strings.TrimSpace(rest[:end]))
	if key == "" {
		return "", "", false
	}
	rest = strings.TrimSpace(rest[end+1:])
	if !strings.HasPrefix(rest, "=") {
		return "", "", false
	}
	return key, unquote(rest[1:]), true
}

// unquote takes the value half of a `key=value` line and strips whatever
// quoting and trailing commentary a real theme file wrapped it in.
func unquote(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if q := raw[0]; q == '"' || q == '\'' {
		if end := strings.IndexByte(raw[1:], q); end >= 0 {
			return raw[1 : 1+end]
		}
		return raw[1:] // unterminated quote: take what is there
	}
	// Unquoted. The value itself starts with '#', so a comment can only be
	// separated by whitespace; the first field is the value.
	if fields := strings.Fields(raw); len(fields) > 0 {
		return fields[0]
	}
	return ""
}

// normalizeColor accepts what btop accepts and returns uppercase "#RRGGBB".
//
// Two forms are valid. "#RRGGBB" is the common one. "#GG" is btop's greyscale
// shorthand, used by themes btop itself ships (adapta.theme sets
// theme[title]="#ff"); verified against btop 1.4.7 by running it against a probe
// theme and reading the emitted SGR: theme[main_fg]="#90" rendered as
// 38;2;144;144;144. The same probe showed btop does NOT expand a 3-digit "#RGB",
// so neither do we. Everything else, including the empty value that means
// "terminal default" to btop, is not a color and is reported as such.
func normalizeColor(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '#' {
		return "", false
	}
	body := s[1:]
	for i := 0; i < len(body); i++ {
		if !isHexDigit(body[i]) {
			return "", false
		}
	}
	switch len(body) {
	case 2:
		g := strings.ToUpper(body)
		return "#" + g + g + g, true
	case 6:
		return "#" + strings.ToUpper(body), true
	}
	return "", false
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// FromMap starts from Nightfable and overrides every field the map supplies.
// Name and Source are carried from the built-in; LoadFile and Resolve set them.
func FromMap(m map[string]string) Theme {
	t := Nightfable()

	for _, f := range scalarFields {
		if v, ok := m[f.key]; ok && v != "" {
			*f.ptr(&t) = v
		}
	}

	for _, k := range boxKeys {
		if v, ok := m[k]; ok && v != "" {
			t.Box = v
			break
		}
	}

	for _, g := range gradientFields {
		*g.ptr(&t) = mergeGradient(*g.ptr(&t), m, g.prefix)
	}

	return t
}

// mergeGradient overlays the file's start/mid/end onto the fallback gradient.
//
// btop interpolates start -> end when a theme omits the mid, so when the file
// moved either endpoint and said nothing about the mid, the fallback's mid no
// longer belongs to this gradient and the midpoint is computed instead. Keeping
// it would blend two unrelated palettes at exactly the value the meters spend
// most of their time near.
func mergeGradient(fallback Gradient3, m map[string]string, prefix string) Gradient3 {
	start, hasStart := lookup(m, prefix+"_start")
	mid, hasMid := lookup(m, prefix+"_mid")
	end, hasEnd := lookup(m, prefix+"_end")

	out := fallback
	if hasStart {
		out[0] = start
	}
	if hasEnd {
		out[2] = end
	}
	switch {
	case hasMid:
		out[1] = mid
	case hasStart || hasEnd:
		out[1] = Lerp(out[0], out[2], 0.5)
	}
	return out
}

func lookup(m map[string]string, key string) (string, bool) {
	v, ok := m[key]
	return v, ok && v != ""
}

// LoadFile reads one btop .theme file. Name comes from the basename; Source is
// provisional and Resolve overwrites it with the tier that won.
func LoadFile(path string) (Theme, error) {
	f, err := os.Open(path)
	if err != nil {
		return Nightfable(), fmt.Errorf("theme: open %s: %w", path, err)
	}
	defer f.Close()

	m, err := ParseBtop(f)
	if err != nil {
		return Nightfable(), fmt.Errorf("theme: parse %s: %w", path, err)
	}
	if len(m) == 0 {
		// Not a theme file, or every value in it was malformed. Say so, so the
		// caller can fall through instead of silently painting the built-in and
		// calling it the user's theme.
		return Nightfable(), fmt.Errorf("theme: %s: no usable theme[...] colors", path)
	}

	t := FromMap(m)
	t.Name = strings.TrimSuffix(filepath.Base(path), ".theme")
	t.Source = "file " + path
	return t, nil
}

// BtopConfigTheme reads color_theme out of a btop.conf and resolves it to a file
// path. It returns "" with no error when btop is on its built-in palette, i.e.
// the key is absent, empty, "Default", or "TTY".
//
// $HOME, ${HOME}, ~, $XDG_CONFIG_HOME and ${XDG_CONFIG_HOME} are expanded. A
// bare name (no slash) is looked up the way btop looks it up: the config file's
// own themes/ directory first, then the system theme directories.
func BtopConfigTheme(confPath string) (string, error) {
	f, err := os.Open(confPath)
	if err != nil {
		return "", fmt.Errorf("theme: open %s: %w", confPath, err)
	}
	defer f.Close()

	raw := ""
	found := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sc.Text()), "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "color_theme") {
			continue
		}
		rest := strings.TrimSpace(line[len("color_theme"):])
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		raw, found = unquote(rest[1:]), true // last assignment wins, as in btop
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("theme: read %s: %w", confPath, err)
	}
	if !found {
		return "", nil
	}

	name := strings.TrimSpace(raw)
	switch strings.ToLower(name) {
	case "", "default", "tty":
		return "", nil
	}

	name = expandPath(name)
	if strings.ContainsRune(name, filepath.Separator) {
		return name, nil
	}

	file := name
	if !strings.HasSuffix(file, ".theme") {
		file += ".theme"
	}
	dirs := []string{
		filepath.Join(filepath.Dir(confPath), "themes"),
		"/usr/share/btop/themes",
		"/usr/local/share/btop/themes",
	}
	for _, d := range dirs {
		p := filepath.Join(d, file)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("theme: %s: color_theme %q not found in %s", confPath, raw, strings.Join(dirs, ", "))
}

// expandPath expands the shell-isms btop's own config writes into color_theme.
func expandPath(p string) string {
	if p == "" {
		return ""
	}
	home := homeDir()
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		p = filepath.Join(home, p[2:])
	}
	p = strings.ReplaceAll(p, "${HOME}", home)
	p = strings.ReplaceAll(p, "$HOME", home)
	xdg := configHome()
	p = strings.ReplaceAll(p, "${XDG_CONFIG_HOME}", xdg)
	p = strings.ReplaceAll(p, "$XDG_CONFIG_HOME", xdg)
	return p
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

func configHome() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return x
	}
	return filepath.Join(homeDir(), ".config")
}

// Resolve picks the theme aitop draws with: explicit path (--theme) beats
// $AITOP_THEME beats btop.conf's color_theme beats the built-in. A failure at
// any tier falls through to the next, and Theme.Source names the tier that won.
//
// configDir defaults to $XDG_CONFIG_HOME, then ~/.config. Never panics, never
// returns an empty field.
func Resolve(explicitPath, configDir string) Theme {
	if configDir == "" {
		configDir = configHome()
	}

	if explicitPath != "" {
		if t, err := LoadFile(explicitPath); err == nil {
			t.Source = "flag " + explicitPath
			return t
		}
	}

	if env := os.Getenv("AITOP_THEME"); env != "" {
		if t, err := LoadFile(env); err == nil {
			t.Source = "$AITOP_THEME " + env
			return t
		}
	}

	conf := filepath.Join(configDir, "btop", "btop.conf")
	if path, err := BtopConfigTheme(conf); err == nil && path != "" {
		if t, err := LoadFile(path); err == nil {
			t.Source = "btop.conf " + path
			return t
		}
	}

	return Nightfable()
}

// Lerp blends two "#RRGGBB" colors in sRGB at t in [0,1] (clamped), rounding
// half up. Output is uppercase "#RRGGBB". An unparseable endpoint yields to the
// other one; if neither parses the result is black, because this function's
// callers are drawing and have nothing to do with an error.
func Lerp(a, b string, t float64) string {
	ar, ag, ab, aok := parseRGB(a)
	br, bg, bb, bok := parseRGB(b)
	switch {
	case !aok && !bok:
		return "#000000"
	case !aok:
		return formatRGB(br, bg, bb)
	case !bok:
		return formatRGB(ar, ag, ab)
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return formatRGB(mix(ar, br, t), mix(ag, bg, t), mix(ab, bb, t))
}

func mix(a, b int, t float64) int {
	v := float64(a) + (float64(b)-float64(a))*t
	return int(v + 0.5) // round half up: 127.5 -> 128
}

func parseRGB(s string) (r, g, b int, ok bool) {
	norm, ok := normalizeColor(s)
	if !ok {
		return 0, 0, 0, false
	}
	return hexByte(norm[1:3]), hexByte(norm[3:5]), hexByte(norm[5:7]), true
}

func hexByte(s string) int {
	return hexVal(s[0])<<4 | hexVal(s[1])
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}

func formatRGB(r, g, b int) string {
	return fmt.Sprintf("#%02X%02X%02X", clamp255(r), clamp255(g), clamp255(b))
}

func clamp255(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// At returns the gradient color at t in [0,1] (clamped): start -> mid over the
// first half, mid -> end over the second.
func (g Gradient3) At(t float64) string {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	if t <= 0.5 {
		return Lerp(g[0], g[1], t*2)
	}
	return Lerp(g[1], g[2], (t-0.5)*2)
}

// Steps returns n+1 colors, index i being i/n of the way along the gradient.
// n=100 is what the UI uses, which puts the mid exactly at index 50.
func (g Gradient3) Steps(n int) []string {
	if n < 0 {
		n = 0
	}
	out := make([]string, n+1)
	if n == 0 {
		out[0] = g.At(0)
		return out
	}
	for i := 0; i <= n; i++ {
		out[i] = g.At(float64(i) / float64(n))
	}
	return out
}
