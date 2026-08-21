package theme

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// hexRe is the shape every color field in a resolved Theme must have.
var hexRe = regexp.MustCompile(`^#[0-9A-F]{6}$`)

// colorFieldCount is every "#RRGGBB" slot in Theme: 12 scalars plus 9 gradients
// of 3. It is asserted, not computed, so that adding a field to Theme reds this
// suite and forces whoever added it to decide what the loader does with it.
// Change it deliberately, never to make a red go away.
const colorFieldCount = 12 + 9*3

// colorFields walks a Theme by reflection and returns every color slot with a
// dotted name. It deliberately does not share the implementation's field tables:
// a list maintained in one place fails in the same direction as the defect it
// exists to catch.
func colorFields(t Theme) map[string]string {
	out := make(map[string]string)
	v := reflect.ValueOf(t)
	rt := v.Type()
	for i := 0; i < rt.NumField(); i++ {
		name := rt.Field(i).Name
		if name == "Name" || name == "Source" {
			continue
		}
		f := v.Field(i)
		switch f.Kind() {
		case reflect.String:
			out[name] = f.String()
		case reflect.Array:
			for j := 0; j < f.Len(); j++ {
				out[name+"["+string(rune('0'+j))+"]"] = f.Index(j).String()
			}
		}
	}
	return out
}

func parseFixture(t *testing.T, name string) map[string]string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture-is-readable violated: %s: %v", name, err)
	}
	defer f.Close()
	m, err := ParseBtop(f)
	if err != nil {
		t.Fatalf("parse-of-a-well-formed-theme-does-not-error violated: %s: %v", name, err)
	}
	if len(m) == 0 {
		t.Fatalf("parse-of-a-nonempty-theme-yields-keys violated: %s parsed to 0 keys (empty is not quiet: a dead parser looks exactly like this)", name)
	}
	return m
}

// writeTheme drops a one-key theme file and hands back its path.
func writeTheme(t *testing.T, dir, name, title string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("fixture-dir-is-writable violated: %v", err)
	}
	body := "theme[title]=\"" + title + "\"\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("fixture-is-writable violated: %s: %v", p, err)
	}
	return p
}

// ---------------------------------------------------------------- 1

// The built-in palette and the .theme file it was transcribed from are two
// independent encodings of the same thing. They must agree, or the fallback ink
// is not the ink btop is using.
func TestBuiltinNightfableMatchesTheThemeFile(t *testing.T) {
	got := FromMap(parseFixture(t, "nightfable.theme"))
	want := Nightfable()

	gv, wv := reflect.ValueOf(got), reflect.ValueOf(want)
	rt := gv.Type()
	diffs := 0
	for i := 0; i < rt.NumField(); i++ {
		if !reflect.DeepEqual(gv.Field(i).Interface(), wv.Field(i).Interface()) {
			diffs++
			t.Errorf("builtin-nightfable-matches-the-theme-file violated: field %s parsed=%v builtin=%v",
				rt.Field(i).Name, gv.Field(i).Interface(), wv.Field(i).Interface())
		}
	}
	if diffs > 0 {
		t.Errorf("builtin-nightfable-matches-the-theme-file violated: %d of %d fields differ between testdata/nightfable.theme and Nightfable()",
			diffs, rt.NumField())
	}
}

// ---------------------------------------------------------------- 2

func TestSparseThemeFallsBackAndInterpolatesTheMissingMid(t *testing.T) {
	got := FromMap(parseFixture(t, "sparse.theme"))
	nf := Nightfable()

	if got.MainFg != "#DDEEFF" {
		t.Errorf("theme-key-present-in-the-file-wins violated: MainFg=%q want #DDEEFF", got.MainFg)
	}

	// Everything the sparse file is silent about comes from the built-in.
	for _, c := range []struct{ name, got, want string }{
		{"MainBg", got.MainBg, nf.MainBg},
		{"Title", got.Title, nf.Title},
		{"HiFg", got.HiFg, nf.HiFg},
		{"SelectedBg", got.SelectedBg, nf.SelectedBg},
		{"InactiveFg", got.InactiveFg, nf.InactiveFg},
		{"MeterBg", got.MeterBg, nf.MeterBg},
		{"ProcMisc", got.ProcMisc, nf.ProcMisc},
		{"Box", got.Box, nf.Box},
		{"DivLine", got.DivLine, nf.DivLine},
	} {
		if c.got != c.want {
			t.Errorf("theme-missing-key-falls-back-to-nightfable violated: %s=%q want %q", c.name, c.got, c.want)
		}
	}
	if got.Temp != nf.Temp {
		t.Errorf("theme-missing-gradient-falls-back-to-nightfable violated: Temp=%v want %v", got.Temp, nf.Temp)
	}

	// cpu_start/cpu_end given, cpu_mid omitted: the mid is the midpoint of the
	// two the file DID give. Expected value is hand-computed, not produced by
	// Lerp, so a broken Lerp cannot move both sides of this comparison at once.
	if got.CPU[0] != "#204060" || got.CPU[2] != "#80A0C0" {
		t.Fatalf("gradient-endpoints-come-from-the-file violated: CPU=%v want start #204060 end #80A0C0", got.CPU)
	}
	if got.CPU[1] != "#507090" {
		t.Errorf("omitted-mid-is-the-interpolated-midpoint violated: CPU mid=%q want #507090 (midpoint of #204060 and #80A0C0)", got.CPU[1])
	}

	// Only used_start was given. The end still comes from the built-in, and the
	// mid is the midpoint of THOSE two, not the built-in's own mid: the file
	// moved an endpoint, so the built-in's mid no longer belongs to this ramp.
	if got.Used[2] != nf.Used[2] {
		t.Fatalf("half-overridden-gradient-keeps-the-fallback-endpoint violated: Used=%v want end %q", got.Used, nf.Used[2])
	}
	if got.Used[1] != "#642817" {
		t.Errorf("half-overridden-gradient-recomputes-the-mid violated: Used mid=%q want #642817 (midpoint of #000000 and %s), got gradient %v",
			got.Used[1], nf.Used[2], got.Used)
	}

	// The never-empty contract, at the one place a mid could plausibly go blank.
	for name, g := range map[string]Gradient3{"CPU": got.CPU, "Used": got.Used, "Temp": got.Temp} {
		if g[1] == "" {
			t.Errorf("gradient-mid-is-never-empty violated: %s=%v", name, g)
		}
	}
}

// ---------------------------------------------------------------- 3

func TestQuotingAndCaseDoNotChangeTheColor(t *testing.T) {
	cases := []struct {
		name, line, key, want string
	}{
		{"single quotes, lowercase", `theme[hi_fg]='#ff0000'`, "hi_fg", "#FF0000"},
		{"unquoted with trailing comment", `theme[hi_fg]=#FF0000  # comment`, "hi_fg", "#FF0000"},
		{"double quoted with trailing comment", `theme[hi_fg]="#ff0000" # comment`, "hi_fg", "#FF0000"},
		{"already uppercase", `theme[hi_fg]="#FF0000"`, "hi_fg", "#FF0000"},
		{"mixed case", `theme[hi_fg]="#Ff0000"`, "hi_fg", "#FF0000"},
		{"spaces around the equals", `  theme[hi_fg]   =   "#ff0000"  `, "hi_fg", "#FF0000"},
		{"CRLF line ending", "theme[hi_fg]=\"#ff0000\"\r", "hi_fg", "#FF0000"},
		{"uppercase key", `theme[HI_FG]="#ff0000"`, "hi_fg", "#FF0000"},
	}
	if len(cases) == 0 {
		t.Fatalf("quote-variant-table-is-not-empty violated: 0 cases would pass silently")
	}
	for _, c := range cases {
		m, err := ParseBtop(strings.NewReader(c.line))
		if err != nil {
			t.Errorf("parsing-a-quote-variant-does-not-error violated: %s: %v", c.name, err)
			continue
		}
		got, ok := m[c.key]
		if !ok {
			t.Errorf("theme-value-quoting-does-not-hide-the-key violated: %s: line %q parsed to %v, key %q absent", c.name, c.line, m, c.key)
			continue
		}
		if got != c.want {
			t.Errorf("theme-value-quoting-does-not-change-the-color violated: %s: line %q got %q want %q", c.name, c.line, got, c.want)
		}
	}

	// Same claim through the whole loader, not just the parser.
	th := FromMap(parseFixture(t, "quotes.theme"))
	for _, c := range []struct{ name, got, want string }{
		{"MainFg", th.MainFg, "#FF0000"},
		{"Title", th.Title, "#00FF00"},
		{"HiFg", th.HiFg, "#0000FF"},
		{"ProcMisc", th.ProcMisc, "#ABCDEF"},
		{"DivLine", th.DivLine, "#FFFFFF"}, // btop greyscale shorthand "#ff"
	} {
		if c.got != c.want {
			t.Errorf("theme-value-quoting-does-not-change-the-color violated: testdata/quotes.theme %s=%q want %q", c.name, c.got, c.want)
		}
	}
}

// ---------------------------------------------------------------- 4

func TestInvalidColorsAreDroppedAndTheFallbackStands(t *testing.T) {
	for _, bad := range []string{"#GGG", "red", "#12345", "#1234567", "rgb(1,2,3)", "", "#", "#1", "0xFF0000", "#FF 0000"} {
		m, err := ParseBtop(strings.NewReader(`theme[hi_fg]="` + bad + `"`))
		if err != nil {
			t.Errorf("an-invalid-color-is-not-an-error violated: %q: %v", bad, err)
		}
		if got, ok := m["hi_fg"]; ok {
			t.Errorf("invalid-color-is-dropped violated: %q was accepted as %q", bad, got)
		}
	}

	th := FromMap(parseFixture(t, "invalid.theme"))
	nf := Nightfable()
	if th.MainFg != "#A1B2C3" {
		t.Errorf("a-valid-color-beside-invalid-ones-still-lands violated: MainFg=%q want #A1B2C3", th.MainFg)
	}
	for _, c := range []struct{ name, got, want string }{
		{"Title", th.Title, nf.Title},
		{"HiFg", th.HiFg, nf.HiFg},
		{"SelectedBg", th.SelectedBg, nf.SelectedBg},
		{"ProcMisc", th.ProcMisc, nf.ProcMisc},
	} {
		if c.got != c.want {
			t.Errorf("invalid-color-leaves-the-fallback-standing violated: %s=%q want %q", c.name, c.got, c.want)
		}
	}
	// Both cpu endpoints were malformed, so nothing about the CPU ramp moved,
	// mid included: no half-interpolated hybrid of two palettes.
	if th.CPU != nf.CPU {
		t.Errorf("invalid-gradient-leaves-the-fallback-standing violated: CPU=%v want %v", th.CPU, nf.CPU)
	}
}

// ---------------------------------------------------------------- real shape

// Mocks match reality or they match nothing: this fixture is byte-identical to
// a theme shipped by btop 1.4.7 (/usr/share/btop/themes/adapta.theme), which
// carries an empty value, btop's 2-digit greyscale shorthand, comments between
// every key, and keys aitop has no field for.
func TestAShippedBtopThemeLoads(t *testing.T) {
	m := parseFixture(t, "adapta-shipped.theme")

	if _, ok := m["main_bg"]; ok {
		t.Errorf("an-empty-value-means-absent violated: main_bg=\"\" in the file was parsed as %q", m["main_bg"])
	}
	// theme[title]="#ff": btop renders 2-digit values as greyscale. Verified
	// against btop 1.4.7 with a probe theme: main_fg="#90" emitted SGR
	// 38;2;144;144;144.
	if got := m["title"]; got != "#FFFFFF" {
		t.Errorf("btop-greyscale-shorthand-expands violated: title=%q want #FFFFFF", got)
	}
	if got := m["div_line"]; got != "#505050" {
		t.Errorf("btop-greyscale-shorthand-expands violated: div_line=%q want #505050", got)
	}
	if got := m["main_fg"]; got != "#CFD8DC" {
		t.Errorf("six-digit-color-normalizes-to-uppercase violated: main_fg=%q want #CFD8DC", got)
	}

	th := FromMap(m)
	if th.MainBg != Nightfable().MainBg {
		t.Errorf("an-empty-value-falls-back violated: MainBg=%q want %q", th.MainBg, Nightfable().MainBg)
	}
	if th.Box != "#00BCD4" {
		t.Errorf("box-comes-from-proc-box violated: Box=%q want #00BCD4", th.Box)
	}
	if th.CPU != (Gradient3{"#00BCD4", "#D4D400", "#FF0040"}) {
		t.Errorf("a-fully-specified-gradient-is-adopted-verbatim violated: CPU=%v", th.CPU)
	}
}

func TestUnknownKeysSurviveTheParseAndAreIgnoredByFromMap(t *testing.T) {
	m, err := ParseBtop(strings.NewReader("theme[proc_banner_bg]=\"#010203\"\ntheme[main_fg]=\"#040506\"\n"))
	if err != nil {
		t.Fatalf("parsing-unknown-keys-does-not-error violated: %v", err)
	}
	if got := m["proc_banner_bg"]; got != "#010203" {
		t.Errorf("unknown-keys-are-kept-in-the-map violated: proc_banner_bg=%q want #010203 (map=%v)", got, m)
	}
	th := FromMap(m)
	if th.MainFg != "#040506" {
		t.Errorf("an-unknown-key-does-not-block-a-known-one violated: MainFg=%q want #040506", th.MainFg)
	}
}

func TestBoxFallsBackThroughTheBoxChain(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"proc_box wins", "theme[proc_box]=\"#111111\"\ntheme[cpu_box]=\"#222222\"\ntheme[mem_box]=\"#333333\"", "#111111"},
		{"cpu_box is next", "theme[cpu_box]=\"#222222\"\ntheme[mem_box]=\"#333333\"", "#222222"},
		{"mem_box is last", "theme[mem_box]=\"#333333\"", "#333333"},
		{"none means nightfable", "theme[main_fg]=\"#444444\"", Nightfable().Box},
	} {
		m, err := ParseBtop(strings.NewReader(c.src))
		if err != nil {
			t.Errorf("parse-does-not-error violated: %s: %v", c.name, err)
			continue
		}
		if got := FromMap(m).Box; got != c.want {
			t.Errorf("box-falls-back-proc-then-cpu-then-mem violated: %s: Box=%q want %q", c.name, got, c.want)
		}
	}
}

// ---------------------------------------------------------------- 5

func TestBtopConfigThemeResolvesTheThemePath(t *testing.T) {
	t.Run("expands $HOME", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		want := filepath.Join(home, ".config", "btop", "themes", "nightfable.theme")
		if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
			t.Fatalf("fixture-dir-is-writable violated: %v", err)
		}
		if err := os.WriteFile(want, []byte("theme[main_fg]=\"#010101\"\n"), 0o644); err != nil {
			t.Fatalf("fixture-is-writable violated: %v", err)
		}
		conf := filepath.Join(t.TempDir(), "btop.conf")
		if err := os.WriteFile(conf, []byte("#? Config file\ncolor_theme = \"$HOME/.config/btop/themes/nightfable.theme\"\n"), 0o644); err != nil {
			t.Fatalf("fixture-is-writable violated: %v", err)
		}
		got, err := BtopConfigTheme(conf)
		if err != nil {
			t.Fatalf("btop-conf-color-theme-resolves-to-a-file-path violated: %v", err)
		}
		if got != want {
			t.Fatalf("btop-conf-expands-$HOME violated: got %q want %q", got, want)
		}
		if _, err := os.Stat(got); err != nil {
			t.Fatalf("btop-conf-expands-$HOME violated: resolved path does not exist: %v", err)
		}
	})

	t.Run("expands tilde and XDG_CONFIG_HOME", func(t *testing.T) {
		home := t.TempDir()
		xdg := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", xdg)
		conf := filepath.Join(t.TempDir(), "btop.conf")

		if err := os.WriteFile(conf, []byte("color_theme = \"~/themes/a.theme\"\n"), 0o644); err != nil {
			t.Fatalf("fixture-is-writable violated: %v", err)
		}
		got, err := BtopConfigTheme(conf)
		if err != nil || got != filepath.Join(home, "themes", "a.theme") {
			t.Errorf("btop-conf-expands-tilde violated: got %q err=%v want %q", got, err, filepath.Join(home, "themes", "a.theme"))
		}

		if err := os.WriteFile(conf, []byte("color_theme = \"${XDG_CONFIG_HOME}/btop/themes/b.theme\"\n"), 0o644); err != nil {
			t.Fatalf("fixture-is-writable violated: %v", err)
		}
		got, err = BtopConfigTheme(conf)
		if err != nil || got != filepath.Join(xdg, "btop", "themes", "b.theme") {
			t.Errorf("btop-conf-expands-XDG_CONFIG_HOME violated: got %q err=%v want %q", got, err, filepath.Join(xdg, "btop", "themes", "b.theme"))
		}
	})

	t.Run("Default and TTY mean no file", func(t *testing.T) {
		dir := t.TempDir()
		for _, v := range []string{"Default", "default", "TTY", "tty", ""} {
			conf := filepath.Join(dir, "btop.conf")
			if err := os.WriteFile(conf, []byte("color_theme = \""+v+"\"\n"), 0o644); err != nil {
				t.Fatalf("fixture-is-writable violated: %v", err)
			}
			got, err := BtopConfigTheme(conf)
			if err != nil {
				t.Errorf("btop-builtin-palette-is-not-an-error violated: color_theme=%q: %v", v, err)
			}
			if got != "" {
				t.Errorf("btop-builtin-palette-resolves-to-no-file violated: color_theme=%q resolved to %q", v, got)
			}
		}
	})

	t.Run("absent color_theme means no file", func(t *testing.T) {
		conf := filepath.Join(t.TempDir(), "btop.conf")
		if err := os.WriteFile(conf, []byte("#? Config file\nupdate_ms = 2000\n"), 0o644); err != nil {
			t.Fatalf("fixture-is-writable violated: %v", err)
		}
		got, err := BtopConfigTheme(conf)
		if err != nil || got != "" {
			t.Errorf("absent-color-theme-resolves-to-no-file violated: got %q err=%v", got, err)
		}
	})

	t.Run("a bare name resolves inside the config dir", func(t *testing.T) {
		cfg := t.TempDir()
		want := filepath.Join(cfg, "themes", "mytheme.theme")
		if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
			t.Fatalf("fixture-dir-is-writable violated: %v", err)
		}
		if err := os.WriteFile(want, []byte("theme[main_fg]=\"#020202\"\n"), 0o644); err != nil {
			t.Fatalf("fixture-is-writable violated: %v", err)
		}
		conf := filepath.Join(cfg, "btop.conf")
		if err := os.WriteFile(conf, []byte("color_theme = \"mytheme\"\n"), 0o644); err != nil {
			t.Fatalf("fixture-is-writable violated: %v", err)
		}
		got, err := BtopConfigTheme(conf)
		if err != nil {
			t.Fatalf("bare-name-resolves-in-the-config-themes-dir violated: %v", err)
		}
		if got != want {
			t.Fatalf("bare-name-resolves-in-the-config-themes-dir violated: got %q want %q", got, want)
		}
	})

	t.Run("an unresolvable bare name is loud", func(t *testing.T) {
		cfg := t.TempDir()
		conf := filepath.Join(cfg, "btop.conf")
		if err := os.WriteFile(conf, []byte("color_theme = \"no-such-theme-anywhere-9f3a\"\n"), 0o644); err != nil {
			t.Fatalf("fixture-is-writable violated: %v", err)
		}
		got, err := BtopConfigTheme(conf)
		if err == nil {
			t.Fatalf("an-unresolvable-theme-name-is-an-error violated: got %q with no error", got)
		}
		if got != "" {
			t.Errorf("an-unresolvable-theme-name-yields-no-path violated: got %q", got)
		}
	})

	t.Run("a missing conf is an error not a silent default", func(t *testing.T) {
		got, err := BtopConfigTheme(filepath.Join(t.TempDir(), "does-not-exist.conf"))
		if err == nil {
			t.Fatalf("a-missing-btop-conf-is-an-error violated: got %q with no error", got)
		}
	})
}

// ---------------------------------------------------------------- 6

func TestResolvePrecedenceIsFlagThenEnvThenConfThenBuiltin(t *testing.T) {
	cfg := t.TempDir()
	flagPath := writeTheme(t, cfg, "flag.theme", "#333333")
	envPath := writeTheme(t, cfg, "env.theme", "#222222")
	confPath := writeTheme(t, cfg, "conf.theme", "#111111")

	btopConf := filepath.Join(cfg, "btop", "btop.conf")
	if err := os.MkdirAll(filepath.Dir(btopConf), 0o755); err != nil {
		t.Fatalf("fixture-dir-is-writable violated: %v", err)
	}
	if err := os.WriteFile(btopConf, []byte("color_theme = \""+confPath+"\"\n"), 0o644); err != nil {
		t.Fatalf("fixture-is-writable violated: %v", err)
	}

	emptyCfg := t.TempDir()
	missing := filepath.Join(cfg, "no-such-file.theme")

	cases := []struct {
		name       string
		env        string
		explicit   string
		configDir  string
		wantTitle  string
		wantSource string
	}{
		{"flag beats env and conf", envPath, flagPath, cfg, "#333333", "flag " + flagPath},
		{"env beats conf", envPath, "", cfg, "#222222", "$AITOP_THEME " + envPath},
		{"conf beats built-in", "", "", cfg, "#111111", "btop.conf " + confPath},
		{"built-in when nothing else answers", "", "", emptyCfg, Nightfable().Title, "built-in"},
		{"a missing flag path falls through to env", envPath, missing, cfg, "#222222", "$AITOP_THEME " + envPath},
		{"a missing env path falls through to conf", filepath.Join(cfg, "nope.theme"), "", cfg, "#111111", "btop.conf " + confPath},
		{"a missing flag and env fall through to built-in", filepath.Join(cfg, "nope.theme"), missing, emptyCfg, Nightfable().Title, "built-in"},
	}
	if len(cases) == 0 {
		t.Fatalf("precedence-table-is-not-empty violated: 0 cases would pass silently")
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("AITOP_THEME", c.env)
			got := Resolve(c.explicit, c.configDir)
			if got.Title != c.wantTitle {
				t.Errorf("resolve-precedence-is-flag-then-env-then-btop-conf-then-builtin violated: Title=%q want %q (flag=%q env=%q configDir=%q source=%q)",
					got.Title, c.wantTitle, c.explicit, c.env, c.configDir, got.Source)
			}
			if got.Source != c.wantSource {
				t.Errorf("resolve-source-names-the-tier-that-won violated: Source=%q want %q", got.Source, c.wantSource)
			}
			if got.Name == "" {
				t.Errorf("resolved-theme-is-named violated: Name is empty (source=%q)", got.Source)
			}
		})
	}

	t.Run("name comes from the file basename", func(t *testing.T) {
		t.Setenv("AITOP_THEME", "")
		if got := Resolve(flagPath, cfg); got.Name != "flag" {
			t.Errorf("theme-name-is-the-basename-without-.theme violated: Name=%q want %q", got.Name, "flag")
		}
	})

	t.Run("empty configDir falls back to XDG_CONFIG_HOME", func(t *testing.T) {
		t.Setenv("AITOP_THEME", "")
		t.Setenv("XDG_CONFIG_HOME", cfg)
		got := Resolve("", "")
		if got.Source != "btop.conf "+confPath {
			t.Errorf("empty-configdir-means-XDG_CONFIG_HOME violated: Source=%q want %q", got.Source, "btop.conf "+confPath)
		}
	})
}

// ---------------------------------------------------------------- 7

func TestLerpBlendsInSRGB(t *testing.T) {
	const a, b = "#204060", "#80A0C0"

	if got := Lerp(a, b, 0); got != a {
		t.Errorf("lerp-at-0-is-exactly-the-first-color violated: got %q want %q", got, a)
	}
	if got := Lerp(a, b, 1); got != b {
		t.Errorf("lerp-at-1-is-exactly-the-second-color violated: got %q want %q", got, b)
	}
	if got := Lerp("#000000", "#FFFFFF", 0.5); got != "#808080" {
		t.Errorf("lerp-rounds-half-up violated: midpoint of #000000 and #FFFFFF is %q, want #808080 (127.5 rounds to 128)", got)
	}
	if got := Lerp(a, b, 0.5); got != "#507090" {
		t.Errorf("lerp-halfway-is-the-channelwise-mean violated: got %q want #507090", got)
	}
	if got := Lerp(a, b, -3); got != a {
		t.Errorf("lerp-clamps-t-below-0 violated: t=-3 gave %q want %q", got, a)
	}
	if got := Lerp(a, b, 7.5); got != b {
		t.Errorf("lerp-clamps-t-above-1 violated: t=7.5 gave %q want %q", got, b)
	}
	if got := Lerp("#ff0000", "#00ff00", 0.25); got != "#BF4000" {
		t.Errorf("lerp-normalizes-lowercase-input violated: got %q want #BF4000", got)
	}
	// Rounding, at the quarter point of an odd span: 0 + 255*0.25 = 63.75 -> 64.
	if got := Lerp("#000000", "#FFFFFF", 0.25); got != "#404040" {
		t.Errorf("lerp-rounds-half-up violated: quarter point gave %q want #404040", got)
	}
	// A drawing function has no error channel; a junk endpoint yields to the good one.
	if got := Lerp("not-a-color", b, 0.5); got != b {
		t.Errorf("lerp-yields-to-the-parseable-endpoint violated: got %q want %q", got, b)
	}
	if got := Lerp("not-a-color", "also-junk", 0.5); !hexRe.MatchString(got) {
		t.Errorf("lerp-always-returns-a-color violated: got %q", got)
	}
}

// ---------------------------------------------------------------- 8

func TestStepsWalksStartToMidToEnd(t *testing.T) {
	g := Gradient3{"#204060", "#507090", "#80A0C0"}
	steps := g.Steps(100)

	if len(steps) != 101 {
		t.Fatalf("steps-n-returns-n-plus-1-colors violated: Steps(100) returned %d colors", len(steps))
	}
	if steps[0] != g[0] {
		t.Errorf("steps-starts-at-the-gradient-start violated: steps[0]=%q want %q", steps[0], g[0])
	}
	if steps[50] != g[1] {
		t.Errorf("steps-index-50-of-100-is-the-mid violated: steps[50]=%q want %q", steps[50], g[1])
	}
	if steps[100] != g[2] {
		t.Errorf("steps-ends-at-the-gradient-end violated: steps[100]=%q want %q", steps[100], g[2])
	}
	bad := 0
	for i, s := range steps {
		if !hexRe.MatchString(s) {
			bad++
			if bad <= 5 {
				t.Errorf("every-step-is-a-valid-uppercase-hex-color violated: steps[%d]=%q", i, s)
			}
		}
	}
	if bad > 0 {
		t.Errorf("every-step-is-a-valid-uppercase-hex-color violated: %d of %d steps malformed", bad, len(steps))
	}

	// At and Steps are the same function sampled two ways.
	for _, i := range []int{0, 1, 25, 49, 50, 51, 75, 99, 100} {
		if got, want := g.At(float64(i)/100), steps[i]; got != want {
			t.Errorf("At-and-Steps-agree violated: At(%v)=%q steps[%d]=%q", float64(i)/100, got, i, want)
		}
	}

	// A ramp whose halves differ tells apart "walks start->mid->end" from
	// "walks start->end and ignores the mid".
	skew := Gradient3{"#000000", "#FF0000", "#FFFFFF"}
	if got := skew.At(0.25); got != "#800000" {
		t.Errorf("steps-first-half-runs-start-to-mid violated: At(0.25)=%q want #800000", got)
	}
	if got := skew.At(0.75); got != "#FF8080" {
		t.Errorf("steps-second-half-runs-mid-to-end violated: At(0.75)=%q want #FF8080", got)
	}

	if got := g.Steps(0); len(got) != 1 || got[0] != g[0] {
		t.Errorf("steps-0-returns-just-the-start violated: got %v", got)
	}
	if got := g.Steps(-5); len(got) != 1 {
		t.Errorf("steps-of-a-negative-n-does-not-panic-or-return-nothing violated: got %v", got)
	}
	if got := g.At(-1); got != g[0] {
		t.Errorf("At-clamps-below-0 violated: got %q want %q", got, g[0])
	}
	if got := g.At(2); got != g[2] {
		t.Errorf("At-clamps-above-1 violated: got %q want %q", got, g[2])
	}
}

// ---------------------------------------------------------------- 9

// The never-empty contract, exercised through every path that can produce a
// Theme. Checking only the no-config case would certify nothing: that path
// returns the built-in literal and cannot fail.
func TestResolvedThemeNeverCarriesAnEmptyOrMalformedField(t *testing.T) {
	abs := func(name string) string {
		p, err := filepath.Abs(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("fixture-path-resolves violated: %v", err)
		}
		return p
	}

	cfg := t.TempDir()
	btopConf := filepath.Join(cfg, "btop", "btop.conf")
	if err := os.MkdirAll(filepath.Dir(btopConf), 0o755); err != nil {
		t.Fatalf("fixture-dir-is-writable violated: %v", err)
	}
	if err := os.WriteFile(btopConf, []byte("color_theme = \""+abs("adapta-shipped.theme")+"\"\n"), 0o644); err != nil {
		t.Fatalf("fixture-is-writable violated: %v", err)
	}

	cases := []struct{ name, explicit, env, configDir string }{
		{"nothing configured", "", "", t.TempDir()},
		{"sparse theme by flag", abs("sparse.theme"), "", t.TempDir()},
		{"all-invalid theme by env", "", abs("invalid.theme"), t.TempDir()},
		{"shipped btop theme via btop.conf", "", "", cfg},
		{"unreadable paths everywhere", "/nonexistent/a.theme", "/nonexistent/b.theme", "/nonexistent"},
	}
	if len(cases) == 0 {
		t.Fatalf("resolve-case-table-is-not-empty violated: 0 cases would pass silently")
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("AITOP_THEME", c.env)
			th := Resolve(c.explicit, c.configDir)

			fields := colorFields(th)
			if len(fields) != colorFieldCount {
				t.Fatalf("theme-color-field-enumeration-is-complete violated: walked %d color fields, expected %d (a walk that finds nothing passes every check below silently)",
					len(fields), colorFieldCount)
			}
			for name, v := range fields {
				if v == "" {
					t.Errorf("resolved-theme-never-carries-an-empty-field violated: %s is empty (source=%q)", name, th.Source)
					continue
				}
				if !hexRe.MatchString(v) {
					t.Errorf("resolved-theme-carries-only-uppercase-RRGGBB violated: %s=%q (source=%q)", name, v, th.Source)
				}
			}
			if th.Name == "" {
				t.Errorf("resolved-theme-is-named violated: Name is empty (source=%q)", th.Source)
			}
			if th.Source == "" {
				t.Errorf("resolved-theme-names-its-origin violated: Source is empty (name=%q)", th.Name)
			}
		})
	}
}

// The canary for the reflective walk: it must find the fields it claims to
// check, on the built-in as well as on a loaded theme. A walk over zero fields
// is byte-identical in output to a walk over a perfect palette.
func TestCanaryColorFieldWalkFindsEveryField(t *testing.T) {
	for _, c := range []struct {
		name string
		th   Theme
	}{
		{"built-in", Nightfable()},
		{"loaded", FromMap(parseFixture(t, "nightfable.theme"))},
		{"zero value", Theme{}},
	} {
		got := colorFields(c.th)
		if len(got) != colorFieldCount {
			t.Errorf("theme-color-field-enumeration-is-complete violated: %s walked %d fields, expected %d", c.name, len(got), colorFieldCount)
		}
	}
	// And it must actually see emptiness when emptiness is there, or its use in
	// the never-empty test proves nothing.
	empties := 0
	for _, v := range colorFields(Theme{}) {
		if v == "" {
			empties++
		}
	}
	if empties != colorFieldCount {
		t.Errorf("the-field-walk-can-see-an-empty-field violated: a zero Theme showed %d empty fields, expected %d", empties, colorFieldCount)
	}
}
