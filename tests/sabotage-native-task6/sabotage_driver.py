#!/usr/bin/env python3
"""Sabotage driver for Task 6 (the graph pane).

Protocol, identical to Tasks 1 through 5: the code is committed FIRST, each
cycle plants an exact-match mutation, proves the plant is present (replace count
== 1 per patch and a non-empty git diff restricted to the sources under test),
runs the focused test with -count=1, restores with `git checkout --`, proves the
restore (porcelain empty), and re-runs the focused test green. A cycle that does
not match its prediction stops the run for inspection rather than being recorded
and moved past.

Mutations are chosen by CLAIM: every assertion in the new tests has at least one
plant aimed at IT, and the weakening column names every site that has to be
relaxed before the plant passes. Three cycles are worth reading before the rest.

S-T6-30 is the one that matters most. It applies the ghost-dimming plant AND
removes the colour pin from TestMain, and it is predicted GREEN. Without the pin
lipgloss renders every style as the bare word, so `Dim.Render(x)` and
`Hi.Render(x)` are the same bytes and every dim assertion in this suite
certifies nothing. That is the state the package was in before this task.

S-T6-19 removes the visited check and is predicted RED via "stack overflow"
rather than a rule phrase: a cycle-safety plant does not fail an assertion, it
fails to terminate, and Go's own stack limit is what catches it.

S-T6-26 is a recorded GREEN with a reason. Dropping the name column's padding
leaves every rendered line exactly terminal-width anyway, because boxLine fits
the whole line. The geometry test therefore does not pin column alignment, and
saying so is better than pretending it does.
"""
import os
import subprocess
import sys
import time

REPO = "/home/aegis/Projects/aitop/.worktrees/native-provenance"
LOG = os.path.join(REPO, "tests/sabotage-native-task6/sabotage_runs_native_task6.log")

# Go's own per-run bound, shorter than the subprocess timeout below. A cycle
# plant CAN fail to terminate, and Go killing the run leaves a stack naming the
# blocked goroutine; the harness killing it would have to abandon the restore.
GO_TEST_TIMEOUT = "90s"

GRA = "internal/ui/graph.go"
VIE = "internal/ui/view.go"
MOD = "internal/ui/model.go"
TST = "internal/ui/view_test.go"

SOURCES = [GRA, VIE, MOD, TST]

P_UI = "./internal/ui"

T_FOREST = "^TestGraphPaneRendersSpawnForest$"
T_MARKS = "^TestGraphPaneMarksGhostAndPartial$"
T_EMPTY = "^TestGraphPaneEmptyShowsCanary$"
T_KEYS = "^TestGraphPaneKeysToggle$"
T_CYCLE = "^TestGraphPaneCycleSafe$"
T_NOACT = "^TestGraphPaneSelectionCarriesNoActions$"
T_GEOM = "^TestGraphPaneKeepsFrameGeometry$"
T_ORDER = "^TestGraphPaneOrdersRuntimeGroupsThenName$"
T_MSG = "^TestGraphPaneCapsMessageEdgesAtThree$"

# --- the forest's shape -----------------------------------------------------

# The indentation join dropped: children still sit under their parent but carry
# no rail, so the forest reads as a flat list. This is the brief's own first
# plant.
P_NO_RAIL = (VIE,
             "\tseg := paint(s.Div, rail) + s.runtimeGlyph(string(n.Runtime), selected) + paint(s.Text, \" \")",
             "\tseg := s.runtimeGlyph(string(n.Runtime), selected) + paint(s.Text, \" \")")
# Post-order rather than pre-order: every child is emitted ABOVE its parent.
# Two patches, because the append has to move rather than vanish.
P_POSTORDER_A = (GRA,
                 "\t\tvisited[id] = true\n\t\tout = append(out, graphLine{kind: graphNodeLine, node: n, name: graphNodeName(n), depth: depth, last: last})\n\t\tfor i, e := range side[id] {",
                 "\t\tvisited[id] = true\n\t\tfor i, e := range side[id] {")
P_POSTORDER_B = (GRA,
                 "\t\tks := kids[id]\n\t\tfor i, k := range ks {\n\t\t\twalk(k, depth+1, i == len(ks)-1)\n\t\t}\n\t}\n\n\tvar roots []graph.NodeID",
                 "\t\tks := kids[id]\n\t\tfor i, k := range ks {\n\t\t\twalk(k, depth+1, i == len(ks)-1)\n\t\t}\n\t\tout = append(out, graphLine{kind: graphNodeLine, node: n, name: graphNodeName(n), depth: depth, last: last})\n\t}\n\n\tvar roots []graph.NodeID")
# Roots entered one level down, so the top of every tree carries a rail into
# nothing. The natural off-by-one for anyone who thinks depth counts nodes.
P_ROOT_DEPTH = (GRA,
                "\torder(roots)\n\tfor _, id := range roots {\n\t\twalk(id, 0, true)\n\t}",
                "\torder(roots)\n\tfor _, id := range roots {\n\t\twalk(id, 1, true)\n\t}")
# The ID-tail fallback returning the whole node id. Every name assertion in the
# forest test is a substring, so this is invisible to all of them but one.
P_NAME_WHOLE_ID = (GRA,
                   "\tif i := strings.LastIndex(id, \":\"); i >= 0 && i+1 < len(id) {\n\t\treturn id[i+1:]\n\t}",
                   "\tif i := strings.LastIndex(id, \":\"); i >= 0 && i+1 < len(id) {\n\t\treturn id\n\t}")
# No fallback at all: an unnamed node paints as an empty name. Written to keep
# the LastIndex call, because deleting it orphans the strings import and the
# cycle would measure the compiler instead of the rule.
P_NAME_NO_FALLBACK = (GRA,
                      "\tif i := strings.LastIndex(id, \":\"); i >= 0 && i+1 < len(id) {\n\t\treturn id[i+1:]\n\t}\n\treturn id",
                      "\tif i := strings.LastIndex(id, \":\"); i >= 0 && i+1 < len(id) {\n\t\treturn \"\"\n\t}\n\treturn id")

# --- the header border ------------------------------------------------------

# The pane's tab renamed. Aimed at the assertion that reads the BORDER line:
# the frame still contains the word "graph" in the footer's own key tab, so a
# whole-frame assertion cannot see this.
P_TAB_RENAMED = (VIE,
                 "\tleft := []string{s.tab(s.Title.Render(\" graph \"))}",
                 "\tleft := []string{s.tab(s.Title.Render(\" forest \"))}")
# A copy-paste slip in the counts: nodes reported where edges belong.
P_COUNTS_SLIP = (VIE,
                 "\t\tcounts = fmt.Sprintf(\" %d nodes · %d edges · %d gaps · topo r%d \",\n\t\t\tlen(g.Nodes), len(g.Edges), len(g.Gaps), g.TopologyRevision)",
                 "\t\tcounts = fmt.Sprintf(\" %d nodes · %d edges · %d gaps · topo r%d \",\n\t\t\tlen(g.Nodes), len(g.Nodes), len(g.Gaps), g.TopologyRevision)")

# --- ghost, partial, stale --------------------------------------------------

# The brief's second plant: ghost dimming skipped. The dagger still paints, so
# the blast radius is exactly the colour claim.
P_NO_GHOST_DIM = (VIE,
                  "\tif graphIsGhost(n) {\n\t\tnameStyle, stateStyle = s.Dim, s.Dim\n\t}",
                  "\tif false {\n\t\tnameStyle, stateStyle = s.Dim, s.Dim\n\t}")
# The dagger dropped, dimming kept. The other half of the same row's marking.
P_NO_DAGGER = (VIE,
               "\tif graphIsGhost(n) {\n\t\tseg += paint(s.Dim, \"✝ \")\n\t}\n",
               "")
# Every row a ghost. The marks are only worth anything if an unmarked row can
# be told apart from a marked one.
P_GHOST_ALWAYS = (GRA,
                  "func graphIsGhost(n graph.Node) bool { return n.GhostExpiresAt != nil }",
                  "func graphIsGhost(n graph.Node) bool { return n.ID != \"\" }")
P_NO_PARTIAL = (VIE,
                "\tname := l.name\n\tif n.Partial {\n\t\tname += \"?\"\n\t}",
                "\tname := l.name\n\tif false {\n\t\tname += \"?\"\n\t}")
P_PARTIAL_ALWAYS = (VIE,
                    "\tname := l.name\n\tif n.Partial {\n\t\tname += \"?\"\n\t}",
                    "\tname := l.name\n\tif true {\n\t\tname += \"?\"\n\t}")
P_NO_STALE_DIM = (VIE,
                  "\tif n.State.Stale {\n\t\tstateStyle = s.Dim\n\t}",
                  "\tif false {\n\t\tstateStyle = s.Dim\n\t}")
P_STALE_ALWAYS = (VIE,
                  "\tif n.State.Stale {\n\t\tstateStyle = s.Dim\n\t}",
                  "\tif true {\n\t\tstateStyle = s.Dim\n\t}")

# --- the empty pane ---------------------------------------------------------

# The brief's third plant: an empty graph painted as a blank box, which is
# byte-identical to a pane that never ran.
P_EMPTY_BLANK = (VIE,
                 "\t\t\tline := \"\"\n\t\t\tif i == mid {\n\t\t\t\tline = centre(s.Hi.Render(snapshot.Canary), w-2)\n\t\t\t}",
                 "\t\t\tline := \"\"\n\t\t\tif i == mid && false {\n\t\t\t\tline = centre(s.Hi.Render(snapshot.Canary), w-2)\n\t\t\t}")
# The canary on every body line. A presence check cannot see this; only a count
# can, which is why the assertion counts.
P_CANARY_EVERYWHERE = (VIE,
                       "\t\t\tline := \"\"\n\t\t\tif i == mid {",
                       "\t\t\tline := \"\"\n\t\t\tif i == mid || true {")

# --- geometry ---------------------------------------------------------------

# The classic off-by-one: the body claims a line the borders already own.
P_BODY_TOO_TALL = (VIE,
                   "\tbodyH := h - 2\n\tif bodyH < 1 {\n\t\tbodyH = 1\n\t}\n\tinner := w - 2 - 1 // borders and one cell of left margin",
                   "\tbodyH := h - 1\n\tif bodyH < 1 {\n\t\tbodyH = 1\n\t}\n\tinner := w - 2 - 1 // borders and one cell of left margin")
# The blank-line fill dropped, so a short forest leaves a short frame.
P_NO_BODY_FILL = (VIE,
                  "\t\tfor i := end - scroll; i < bodyH; i++ {\n\t\t\tb.WriteString(s.boxLine(\"\", w))\n\t\t\tb.WriteByte('\\n')\n\t\t}",
                  "\t\tfor i := end - scroll; i < 0; i++ {\n\t\t\tb.WriteString(s.boxLine(\"\", w))\n\t\t\tb.WriteByte('\\n')\n\t\t}")
# The top border built one cell narrow. boxLine guarantees the BODY's width, so
# only a border can move it, and this is what the width assertion is for.
P_BORDER_NARROW = (VIE,
                   "\tb.WriteString(s.boxTop(w, left, []string{s.tab(s.Dim.Render(counts))}))",
                   "\tb.WriteString(s.boxTop(w-1, left, []string{s.tab(s.Dim.Render(counts))}))")
# The name column left unpadded. Predicted GREEN: boxLine fits the whole line,
# so every line is still exactly terminal-width and only the COLUMNS move.
P_NO_NAME_PAD = (VIE,
                 "\t\tcase cur < w:\n\t\t\treturn seg + paint(s.Text, strings.Repeat(\" \", w-cur))",
                 "\t\tcase cur < w:\n\t\t\treturn seg")

# --- cycles -----------------------------------------------------------------

# The visited check removed, which is the whole cycle-safety rule. This does not
# fail an assertion, it fails to terminate.
P_NO_VISITED = (GRA,
                "\t\tif visited[id] {\n\t\t\tout = append(out, graphLine{kind: graphBackLine, name: graphNodeName(n), depth: depth, last: last})\n\t\t\treturn\n\t\t}\n\t\tvisited[id] = true",
                "\t\tvisited[id] = true")
# The back reference drawn as an ordinary node line: the node is still drawn
# once, and nothing says the second appearance is a reference.
P_BACKREF_AS_NODE = (GRA,
                     "\t\t\tout = append(out, graphLine{kind: graphBackLine, name: graphNodeName(n), depth: depth, last: last})",
                     "\t\t\tout = append(out, graphLine{kind: graphNodeLine, name: graphNodeName(n), depth: depth, last: last})")
# The marker changed. The name still renders, so the counting assertion stays
# green and only the naming assertion moves.
P_BACKREF_MARKER = (VIE,
                    "\t\tseg := paint(s.Div, rail) + paint(s.Dim, \"↩ \"+l.name)",
                    "\t\tseg := paint(s.Div, rail) + paint(s.Dim, \"* \"+l.name)")
# The orphan sweep dropped. Every member of a cycle is somebody's target, so
# none of them is a root and the whole cycle vanishes from the pane.
P_NO_ORPHAN_SWEEP = (GRA,
                     "\tfor _, id := range orphans {\n\t\tif !visited[id] {\n\t\t\twalk(id, 0, true)\n\t\t}\n\t}",
                     "\tfor _, id := range orphans {\n\t\t_ = id\n\t}")

# --- ordering ---------------------------------------------------------------

P_ORDER_NO_RUNTIME = (GRA,
                      "\t\t\tif ra, rb := runtimeGroupRank(a.Runtime), runtimeGroupRank(b.Runtime); ra != rb {\n\t\t\t\treturn ra < rb\n\t\t\t}\n",
                      "")
P_ORDER_NO_NAME = (GRA,
                   "\t\t\tif na, nb := graphNodeName(a), graphNodeName(b); na != nb {\n\t\t\t\treturn na < nb\n\t\t\t}\n",
                   "")
# grok and codex transposed in the family order.
P_ORDER_SWAP_RANK = (GRA,
                     "\tcase types.RuntimeGrok:\n\t\treturn 1\n\tcase types.RuntimeCodex:\n\t\treturn 2",
                     "\tcase types.RuntimeGrok:\n\t\treturn 2\n\tcase types.RuntimeCodex:\n\t\treturn 1")

# --- message and service edges ----------------------------------------------

# Off by one on the cap, which is the way a cap is usually got wrong.
P_MSG_CAP_OFF = (GRA, "\t\t\tif i >= maxGraphEdgeLines {", "\t\t\tif i > maxGraphEdgeLines {")
# A service edge announced as a message. The pane's whole job is provenance.
P_SVC_AS_MSG = (GRA,
                "\tif e.Type == graph.EdgeService {\n\t\treturn \"svc→\"\n\t}",
                "\tif false {\n\t\treturn \"svc→\"\n\t}")
# Message edges admitted as spawn edges, so talking to a node adopts it.
P_MSG_PARENTS_A = (GRA,
                   "\t\tcase graph.EdgeSpawn, graph.EdgeLaunch:\n\t\t\tspawned[e.Target] = true",
                   "\t\tcase graph.EdgeSpawn, graph.EdgeLaunch, graph.EdgeMessage:\n\t\t\tspawned[e.Target] = true")
P_MSG_PARENTS_B = (GRA,
                   "\t\tcase graph.EdgeMessage, graph.EdgeService:\n\t\t\tside[e.Source] = append(side[e.Source], e)",
                   "\t\tcase graph.EdgeService:\n\t\t\tside[e.Source] = append(side[e.Source], e)")

# --- keys and the read-only rule --------------------------------------------

P_NO_FOOTER_PRESETS = (VIE,
                       "\t\t\ts.key(\"1\", \"table\"),\n\t\t\ts.key(\"2\", \"graph\"),\n\t\t}",
                       "\t\t}")
P_KEY_TWO_DEAD = (MOD,
                  "\tcase \"2\":\n\t\tm.setGraphView(true)\n\tcase \"tab\":\n\t\tm.setGraphView(!m.graphView)\n\t}\n\treturn m, nil\n}\n\n// graphKey",
                  "\tcase \"2\":\n\tcase \"tab\":\n\t\tm.setGraphView(!m.graphView)\n\t}\n\treturn m, nil\n}\n\n// graphKey")
P_KEY_TAB_DEAD = (MOD,
                  "\tcase \"2\":\n\t\tm.setGraphView(true)\n\tcase \"tab\":\n\t\tm.setGraphView(!m.graphView)\n\t}\n\treturn m, nil\n}\n\n// graphKey",
                  "\tcase \"2\":\n\t\tm.setGraphView(true)\n\tcase \"tab\":\n\t}\n\treturn m, nil\n}\n\n// graphKey")
# `1` inert inside the pane: the operator can get in and not out.
P_KEY_ONE_DEAD_IN_PANE = (MOD,
                          "\tcase \"1\":\n\t\tm.setGraphView(false)\n\tcase \"2\":\n\t\tm.setGraphView(true)\n\tcase \"tab\":\n\t\tm.setGraphView(!m.graphView)\n\tcase \"j\", \"down\":",
                          "\tcase \"1\":\n\tcase \"2\":\n\t\tm.setGraphView(true)\n\tcase \"tab\":\n\t\tm.setGraphView(!m.graphView)\n\tcase \"j\", \"down\":")
# The graph as the default view.
P_DEFAULT_GRAPH = (MOD,
                   "\treturn Model{\n\t\tsrc:     src,\n\t\tstyles:  NewStyles(t),",
                   "\treturn Model{\n\t\tgraphView: true,\n\t\tsrc:     src,\n\t\tstyles:  NewStyles(t),")
# The preset ignored at paint time: `2` sets the flag and the table still draws.
P_GRAPH_NOT_PAINTED = (VIE,
                       "\tif f.graphView {\n\t\tvar b strings.Builder\n\t\ts.renderHeader(&b, f)",
                       "\tif false {\n\t\tvar b strings.Builder\n\t\ts.renderHeader(&b, f)")
# The pane's key dispatch removed, so the table's keymap runs underneath the
# graph and every action key acts on whatever table row the cursor indexes.
# This is the plant the read-only rule exists for.
P_NO_GRAPH_KEYMAP = (MOD,
                     "\t\tif m.graphView {\n\t\t\treturn m.graphKey(msg)\n\t\t}\n",
                     "")

# --- the colour pin ---------------------------------------------------------

# The pin removed from TestMain. Under `go test` lipgloss then detects Ascii and
# renders every style as the bare word, so Dim and Hi are the same bytes.
P_NO_COLOR_PIN = (TST,
                  "\tlipgloss.SetColorProfile(termenv.TrueColor)\n",
                  "\t_ = termenv.TrueColor\n\t_ = lipgloss.NewStyle\n")

# --- weakenings -------------------------------------------------------------

W_EVERY_NODE = (TST,
                "\t\tif lineIndex(out, want) < 0 {",
                "\t\tif false && lineIndex(out, want) < 0 {")
W_FOREST = (TST,
            "\t\tif child != parent+1 {",
            "\t\tif false && child != parent+1 {")
W_INDENT = (TST,
            "\t\tif !strings.Contains(lineAt(lines, child), \"└─\") {",
            "\t\tif false && !strings.Contains(lineAt(lines, child), \"└─\") {")
W_ROOT_RAIL = (TST,
               "\t\tif strings.Contains(lineAt(lines, parent), \"└─\") || strings.Contains(lineAt(lines, parent), \"├─\") {",
               "\t\tif false && (strings.Contains(lineAt(lines, parent), \"└─\") || strings.Contains(lineAt(lines, parent), \"├─\")) {")
W_ID_TAIL = (TST,
             "\tif ghost := rowLine(out, \"01a022e3\"); strings.Contains(ghost, \"grok:session:\") {",
             "\tif ghost := rowLine(out, \"01a022e3\"); false && strings.Contains(ghost, \"grok:session:\") {")
W_TAB_NAME = (TST,
              "\tif !strings.Contains(top, \"┤ graph ├\") {",
              "\tif false && !strings.Contains(top, \"┤ graph ├\") {")
W_COUNTS = (TST,
            "\tif !strings.Contains(top, \"4 nodes · 2 edges · 1 gaps · topo r7\") {",
            "\tif false && !strings.Contains(top, \"4 nodes · 2 edges · 1 gaps · topo r7\") {")

W_DAGGER = (TST,
            "\tif dagger < 0 || dagger > name {",
            "\tif false && (dagger < 0 || dagger > name) {")
W_GHOST_DIM = (TST,
               "\tif !strings.Contains(raw, st.Dim.Render(\"01a022e3\")) {",
               "\tif false && !strings.Contains(raw, st.Dim.Render(\"01a022e3\")) {")
W_PARTIAL = (TST,
             "\tif partial := rowLine(out, \"impl-t6\"); !strings.Contains(partial, \"impl-t6?\") {",
             "\tif partial := rowLine(out, \"impl-t6\"); false && !strings.Contains(partial, \"impl-t6?\") {")
W_ONLY_MARKED = (TST,
                 "\tif live := rowLine(out, \"aegis-79\"); strings.Contains(live, \"✝\") || strings.Contains(live, \"aegis-79?\") {",
                 "\tif live := rowLine(out, \"aegis-79\"); false && (strings.Contains(live, \"✝\") || strings.Contains(live, \"aegis-79?\")) {")
W_STALE_DIM = (TST,
               "\tif !strings.Contains(raw, st.Dim.Render(\"thinking\")) {",
               "\tif false && !strings.Contains(raw, st.Dim.Render(\"thinking\")) {")
W_FRESH_NOT_DIM = (TST,
                   "\tif strings.Contains(raw, st.Dim.Render(\"active\")) {",
                   "\tif false && strings.Contains(raw, st.Dim.Render(\"active\")) {")

W_EMPTY_HEIGHT = (TST,
                  "\t\tif len(lines) != 30 {",
                  "\t\tif false && len(lines) != 30 {")
W_EMPTY_CANARY = (TST,
                  "\t\tif body != 1 {",
                  "\t\tif false && body != 1 {")

W_DEFAULT_TABLE = (TST,
                   "\tif tm.(Model).graphView {\n\t\tt.Fatalf(\"graph-pane-keys-toggle rule violated: the table is not the default view\")",
                   "\tif false {\n\t\tt.Fatalf(\"graph-pane-keys-toggle rule violated: the table is not the default view\")")
W_FOOTER_PRESETS = (TST,
                    "\tif !strings.Contains(table, \"1 table\") || !strings.Contains(table, \"2 graph\") {",
                    "\tif false && (!strings.Contains(table, \"1 table\") || !strings.Contains(table, \"2 graph\")) {")
W_TWO_OPENS = (TST,
               "\tif !tm.(Model).graphView {\n\t\tt.Fatalf(\"graph-pane-keys-toggle rule violated: 2 did not open the graph\")",
               "\tif false {\n\t\tt.Fatalf(\"graph-pane-keys-toggle rule violated: 2 did not open the graph\")")
W_TWO_SHOWS = (TST,
               "\tif !strings.Contains(view, \"aegis-79\") {",
               "\tif false && !strings.Contains(view, \"aegis-79\") {")
W_TWO_REPLACES = (TST,
                  "\tif strings.Contains(view, \"NAME\") {",
                  "\tif false && strings.Contains(view, \"NAME\") {")
W_ONE_RETURNS = (TST,
                 "\tif tm.(Model).graphView {\n\t\tt.Fatalf(\"graph-pane-keys-toggle rule violated: 1 did not return to the table\")",
                 "\tif false {\n\t\tt.Fatalf(\"graph-pane-keys-toggle rule violated: 1 did not return to the table\")")
W_ONE_SHOWS_TABLE = (TST,
                     "\tif !strings.Contains(ansi.Strip(tm.View()), \"NAME\") {",
                     "\tif false && !strings.Contains(ansi.Strip(tm.View()), \"NAME\") {")
W_TAB_OPENS = (TST,
               "\tif !tm.(Model).graphView {\n\t\tt.Fatalf(\"graph-pane-tab-flips-the-view rule violated: tab from the table did not open the graph\")",
               "\tif false {\n\t\tt.Fatalf(\"graph-pane-tab-flips-the-view rule violated: tab from the table did not open the graph\")")
W_TAB_CLOSES = (TST,
                "\tif tm.(Model).graphView {\n\t\tt.Fatalf(\"graph-pane-tab-flips-the-view rule violated: tab from the graph did not return to the table\")",
                "\tif false {\n\t\tt.Fatalf(\"graph-pane-tab-flips-the-view rule violated: tab from the graph did not return to the table\")")

W_CYCLE_ALPHA = (TST,
                 "\tif n := strings.Count(out, \"alpha\"); n != 2 {",
                 "\tif n := strings.Count(out, \"alpha\"); false && n != 2 {")
W_CYCLE_BETA = (TST,
                "\tif n := strings.Count(out, \"beta\"); n != 1 {",
                "\tif n := strings.Count(out, \"beta\"); false && n != 1 {")
W_CYCLE_GAMMA = (TST,
                 "\tif n := strings.Count(out, \"gamma\"); n != 2 {",
                 "\tif n := strings.Count(out, \"gamma\"); false && n != 2 {")
W_BACKREF_ALPHA = (TST,
                   "\tif !strings.Contains(out, \"↩ alpha\") {",
                   "\tif false && !strings.Contains(out, \"↩ alpha\") {")
W_BACKREF_GAMMA = (TST,
                   "\tif !strings.Contains(out, \"↩ gamma\") {",
                   "\tif false && !strings.Contains(out, \"↩ gamma\") {")

W_NOACT_ENQUEUE = (TST,
                   "\tif len(got) != 0 {",
                   "\tif false && len(got) != 0 {")
W_NOACT_STATE = (TST,
                 "\tif mm.confirmOp != \"\" || mm.promptMode != \"\" || mm.markKey != \"\" {",
                 "\tif false && (mm.confirmOp != \"\" || mm.promptMode != \"\" || mm.markKey != \"\") {")
W_NOACT_STAYS = (TST,
                 "\tif !mm.graphView {\n\t\tt.Fatalf(\"graph-pane-selection-carries-no-actions rule violated: an action key left the graph view\")",
                 "\tif false {\n\t\tt.Fatalf(\"graph-pane-selection-carries-no-actions rule violated: an action key left the graph view\")")
W_NOACT_CONFIRM = (TST,
                   "\tif v := ansi.Strip(tm.View()); strings.Contains(v, \"y/N\") {",
                   "\tif v := ansi.Strip(tm.View()); false && strings.Contains(v, \"y/N\") {")

# Both of these strings also occur in the table suite's own geometry test, so
# each carries its own t.Fatalf phrase as the discriminator.
W_GEOM_HEIGHT = (TST,
                 "\t\tif len(lines) != sz[1] {\n\t\t\tt.Fatalf(\"graph-pane-fills-terminal-height",
                 "\t\tif false {\n\t\t\tt.Fatalf(\"graph-pane-fills-terminal-height")
W_GEOM_WIDTH = (TST,
                "\t\t\tif w := ansi.StringWidth(l); w != sz[0] {\n\t\t\t\tt.Fatalf(\"graph-pane-line-is-terminal-width",
                "\t\t\tif w := ansi.StringWidth(l); false {\n\t\t\t\tt.Fatalf(\"graph-pane-line-is-terminal-width")

W_ORDER_PRESENT = (TST,
                   "\t\tif at[i] = lineIndex(out, w); at[i] < 0 {",
                   "\t\tif at[i] = lineIndex(out, w); false && at[i] < 0 {")
W_ORDER_SEQ = (TST,
               "\t\tif at[i] <= at[i-1] {",
               "\t\tif false && at[i] <= at[i-1] {")

W_MSG_CAP = (TST,
             "\tif n := strings.Count(out, \"msg→\"); n != 3 {",
             "\tif n := strings.Count(out, \"msg→\"); false && n != 3 {")
W_SVC_NAME = (TST,
              "\tif !strings.Contains(out, \"svc→ codex-worker\") {",
              "\tif false && !strings.Contains(out, \"svc→ codex-worker\") {")
W_MSG_NO_PARENT = (TST,
                   "\tif strings.Contains(extraLine, \"└─\") || strings.Contains(extraLine, \"├─\") {",
                   "\tif false && (strings.Contains(extraLine, \"└─\") || strings.Contains(extraLine, \"├─\")) {")

R_FOREST = "graph-pane-renders-the-spawn-forest rule violated"
R_EVERY = "graph-pane-renders-every-node rule violated"
R_INDENT = "graph-pane-indents-children-under-parents rule violated"
R_ROOT_RAIL = "graph-pane-roots-carry-no-rail rule violated"
R_ID_TAIL = "graph-pane-falls-back-to-the-id-tail rule violated"
R_TAB = "graph-pane-header-names-itself rule violated"
R_COUNTS = "graph-pane-header-counts-what-it-drew rule violated"
R_DAGGER = "graph-pane-marks-ghosts-with-a-dagger rule violated"
R_GHOST_DIM = "graph-pane-dims-a-ghost-row rule violated"
R_PARTIAL = "graph-pane-marks-partial-nodes rule violated"
R_ONLY_MARKED = "graph-pane-marks-only-what-is-marked rule violated"
R_STALE = "graph-pane-dims-a-stale-state rule violated"
R_EMPTY_H = "graph-pane-empty-keeps-frame-height rule violated"
R_QUIET = "graph-pane-empty-is-not-quiet rule violated"
R_TOGGLE = "graph-pane-keys-toggle rule violated"
R_PRESETS = "graph-pane-footer-offers-both-presets rule violated"
R_TWO_SHOWS = "graph-pane-two-shows-the-graph rule violated"
R_TWO_REPLACES = "graph-pane-two-replaces-the-table rule violated"
R_ONE_TABLE = "graph-pane-one-shows-the-table rule violated"
R_TAB_FLIP = "graph-pane-tab-flips-the-view rule violated"
R_ONCE = "graph-pane-draws-a-repeated-node-once rule violated"
R_BACKREF = "graph-pane-names-the-back-reference rule violated"
R_NOACT = "graph-pane-selection-carries-no-actions rule violated"
R_HEIGHT = "graph-pane-fills-terminal-height rule violated"
R_WIDTH = "graph-pane-line-is-terminal-width rule violated"
R_ORDER = "graph-pane-orders-runtime-groups-then-name rule violated"
R_MSG_CAP = "graph-pane-caps-message-lines-at-three rule violated"
R_SVC = "graph-pane-names-a-service-edge-as-service rule violated"
R_MSG_PARENT = "graph-pane-message-edges-do-not-parent rule violated"

CYCLES = [
    # --- the forest's shape -------------------------------------------------
    ("S-T6-01-prod", [P_NO_RAIL], T_FOREST, "RED", R_INDENT),
    ("S-T6-01-weak", [P_NO_RAIL, W_INDENT], T_FOREST, "GREEN", None),
    ("S-T6-02-prod", [P_POSTORDER_A, P_POSTORDER_B], T_FOREST, "RED", R_FOREST),
    ("S-T6-02-weak", [P_POSTORDER_A, P_POSTORDER_B, W_FOREST, W_INDENT, W_ROOT_RAIL], T_FOREST, "GREEN", None),
    ("S-T6-03-prod", [P_ROOT_DEPTH], T_FOREST, "RED", R_ROOT_RAIL),
    ("S-T6-03-weak", [P_ROOT_DEPTH, W_ROOT_RAIL], T_FOREST, "GREEN", None),
    ("S-T6-04-prod", [P_NAME_NO_FALLBACK], T_FOREST, "RED", R_EVERY),
    ("S-T6-04-weak", [P_NAME_NO_FALLBACK, W_EVERY_NODE, W_FOREST, W_INDENT], T_FOREST, "GREEN", None),
    # The whole id is a superstring of the tail, so every substring assertion
    # in the test stays green and only the tail witness moves.
    ("S-T6-05-prod", [P_NAME_WHOLE_ID], T_FOREST, "RED", R_ID_TAIL),
    ("S-T6-05-weak", [P_NAME_WHOLE_ID, W_ID_TAIL], T_FOREST, "GREEN", None),

    # --- the header border --------------------------------------------------
    ("S-T6-06-prod", [P_TAB_RENAMED], T_FOREST, "RED", R_TAB),
    ("S-T6-06-weak", [P_TAB_RENAMED, W_TAB_NAME], T_FOREST, "GREEN", None),
    ("S-T6-07-prod", [P_COUNTS_SLIP], T_FOREST, "RED", R_COUNTS),
    ("S-T6-07-weak", [P_COUNTS_SLIP, W_COUNTS], T_FOREST, "GREEN", None),

    # --- ghost, partial, stale ----------------------------------------------
    ("S-T6-08-prod", [P_NO_GHOST_DIM], T_MARKS, "RED", R_GHOST_DIM),
    ("S-T6-08-weak", [P_NO_GHOST_DIM, W_GHOST_DIM], T_MARKS, "GREEN", None),
    ("S-T6-09-prod", [P_NO_DAGGER], T_MARKS, "RED", R_DAGGER),
    ("S-T6-09-weak", [P_NO_DAGGER, W_DAGGER], T_MARKS, "GREEN", None),
    ("S-T6-10-prod", [P_GHOST_ALWAYS], T_MARKS, "RED", R_ONLY_MARKED),
    ("S-T6-10-weak", [P_GHOST_ALWAYS, W_ONLY_MARKED, W_FRESH_NOT_DIM], T_MARKS, "GREEN", None),
    ("S-T6-11-prod", [P_NO_PARTIAL], T_MARKS, "RED", R_PARTIAL),
    ("S-T6-11-weak", [P_NO_PARTIAL, W_PARTIAL], T_MARKS, "GREEN", None),
    ("S-T6-12-prod", [P_PARTIAL_ALWAYS], T_MARKS, "RED", R_ONLY_MARKED),
    ("S-T6-12-weak", [P_PARTIAL_ALWAYS, W_ONLY_MARKED], T_MARKS, "GREEN", None),
    ("S-T6-13-prod", [P_NO_STALE_DIM], T_MARKS, "RED", R_STALE),
    ("S-T6-13-weak", [P_NO_STALE_DIM, W_STALE_DIM], T_MARKS, "GREEN", None),
    # The other direction of the same rule: dimming everything is as wrong as
    # dimming nothing, and only the second witness can see it.
    ("S-T6-14-prod", [P_STALE_ALWAYS], T_MARKS, "RED", R_STALE),
    ("S-T6-14-weak", [P_STALE_ALWAYS, W_FRESH_NOT_DIM], T_MARKS, "GREEN", None),

    # --- the empty pane -----------------------------------------------------
    ("S-T6-15-prod", [P_EMPTY_BLANK], T_EMPTY, "RED", R_QUIET),
    ("S-T6-15-weak", [P_EMPTY_BLANK, W_EMPTY_CANARY], T_EMPTY, "GREEN", None),
    ("S-T6-16-prod", [P_CANARY_EVERYWHERE], T_EMPTY, "RED", R_QUIET),
    ("S-T6-16-weak", [P_CANARY_EVERYWHERE, W_EMPTY_CANARY], T_EMPTY, "GREEN", None),

    # --- geometry -----------------------------------------------------------
    ("S-T6-17-prod", [P_BODY_TOO_TALL], T_GEOM, "RED", R_HEIGHT),
    ("S-T6-17b-prod", [P_BODY_TOO_TALL], T_EMPTY, "RED", R_EMPTY_H),
    ("S-T6-17-weak", [P_BODY_TOO_TALL, W_GEOM_HEIGHT], T_GEOM, "GREEN", None),
    ("S-T6-18-prod", [P_NO_BODY_FILL], T_GEOM, "RED", R_HEIGHT),
    ("S-T6-18-weak", [P_NO_BODY_FILL, W_GEOM_HEIGHT], T_GEOM, "GREEN", None),
    ("S-T6-18b-prod", [P_BORDER_NARROW], T_GEOM, "RED", R_WIDTH),
    ("S-T6-18b-weak", [P_BORDER_NARROW, W_GEOM_WIDTH], T_GEOM, "GREEN", None),

    # --- cycles -------------------------------------------------------------
    # Not an assertion failure: a cycle-safety plant fails to terminate, and
    # Go's own stack limit is the thing that catches it.
    ("S-T6-19-prod", [P_NO_VISITED], T_CYCLE, "RED", "stack overflow"),
    ("S-T6-20-prod", [P_BACKREF_AS_NODE], T_CYCLE, "RED", R_BACKREF),
    ("S-T6-20-weak", [P_BACKREF_AS_NODE, W_BACKREF_ALPHA, W_BACKREF_GAMMA], T_CYCLE, "GREEN", None),
    ("S-T6-21-prod", [P_BACKREF_MARKER], T_CYCLE, "RED", R_BACKREF),
    ("S-T6-21-weak", [P_BACKREF_MARKER, W_BACKREF_ALPHA, W_BACKREF_GAMMA], T_CYCLE, "GREEN", None),
    ("S-T6-22-prod", [P_NO_ORPHAN_SWEEP], T_CYCLE, "RED", R_ONCE),
    ("S-T6-22-weak", [P_NO_ORPHAN_SWEEP, W_CYCLE_ALPHA, W_CYCLE_BETA, W_CYCLE_GAMMA, W_BACKREF_ALPHA, W_BACKREF_GAMMA], T_CYCLE, "GREEN", None),
    # The acyclic fixture is untouched by the sweep, which is the point: the
    # sweep is a cycle rule and only the cycle test should see it move.
    ("S-T6-22b-prod", [P_NO_ORPHAN_SWEEP], T_FOREST, "GREEN", None),

    # --- ordering -----------------------------------------------------------
    ("S-T6-23-prod", [P_ORDER_NO_RUNTIME], T_ORDER, "RED", R_ORDER),
    ("S-T6-23-weak", [P_ORDER_NO_RUNTIME, W_ORDER_SEQ], T_ORDER, "GREEN", None),
    ("S-T6-24-prod", [P_ORDER_NO_NAME], T_ORDER, "RED", R_ORDER),
    ("S-T6-24-weak", [P_ORDER_NO_NAME, W_ORDER_SEQ], T_ORDER, "GREEN", None),
    ("S-T6-25-prod", [P_ORDER_SWAP_RANK], T_ORDER, "RED", R_ORDER),
    ("S-T6-25-weak", [P_ORDER_SWAP_RANK, W_ORDER_SEQ], T_ORDER, "GREEN", None),

    # --- a recorded GREEN with a reason -------------------------------------
    # boxLine fits the whole line, so dropping the name column's padding leaves
    # every line exactly terminal-width and moves only the columns. The
    # geometry test does not pin column alignment and this says so out loud.
    ("S-T6-26-prod", [P_NO_NAME_PAD], T_GEOM, "GREEN", None),
    ("S-T6-26b-prod", [P_NO_NAME_PAD], T_FOREST, "GREEN", None),

    # --- message and service edges ------------------------------------------
    ("S-T6-27-prod", [P_MSG_CAP_OFF], T_MSG, "RED", R_MSG_CAP),
    ("S-T6-27-weak", [P_MSG_CAP_OFF, W_MSG_CAP], T_MSG, "GREEN", None),
    # Renaming svc to msg puts a fourth msg line on the pane, so the CAP
    # assertion fires first. Relaxing it exposes the naming assertion
    # underneath, which is the one this plant is aimed at.
    ("S-T6-28-prod", [P_SVC_AS_MSG], T_MSG, "RED", R_MSG_CAP),
    ("S-T6-28-mid", [P_SVC_AS_MSG, W_MSG_CAP], T_MSG, "RED", R_SVC),
    ("S-T6-28-weak", [P_SVC_AS_MSG, W_MSG_CAP, W_SVC_NAME], T_MSG, "GREEN", None),
    # Same shape: adopting message targets empties the annotation list first.
    ("S-T6-29-prod", [P_MSG_PARENTS_A, P_MSG_PARENTS_B], T_MSG, "RED", R_MSG_CAP),
    ("S-T6-29-mid", [P_MSG_PARENTS_A, P_MSG_PARENTS_B, W_MSG_CAP], T_MSG, "RED", R_MSG_PARENT),
    ("S-T6-29-weak", [P_MSG_PARENTS_A, P_MSG_PARENTS_B, W_MSG_CAP, W_MSG_NO_PARENT], T_MSG, "GREEN", None),

    # --- the colour pin -----------------------------------------------------
    # The cycle this whole file exists for. The ghost-dimming plant is RED with
    # the pin (S-T6-08) and GREEN without it, because an Ascii renderer makes
    # Dim.Render(x) and the bare word the same bytes. Every dim assertion in
    # this package certified nothing before the pin went in.
    ("S-T6-30-prod", [P_NO_COLOR_PIN], T_MARKS, "RED", R_STALE),
    ("S-T6-30-weak", [P_NO_GHOST_DIM, P_NO_COLOR_PIN, W_FRESH_NOT_DIM], T_MARKS, "GREEN", None),

    # --- keys and the read-only rule ----------------------------------------
    ("S-T6-31-prod", [P_NO_FOOTER_PRESETS], T_KEYS, "RED", R_PRESETS),
    ("S-T6-31-weak", [P_NO_FOOTER_PRESETS, W_FOOTER_PRESETS], T_KEYS, "GREEN", None),
    ("S-T6-32-prod", [P_KEY_TWO_DEAD], T_KEYS, "RED", R_TOGGLE),
    ("S-T6-32-weak", [P_KEY_TWO_DEAD, W_TWO_OPENS, W_TWO_SHOWS, W_TWO_REPLACES, W_ONE_SHOWS_TABLE], T_KEYS, "GREEN", None),
    ("S-T6-33-prod", [P_KEY_TAB_DEAD], T_KEYS, "RED", R_TAB_FLIP),
    ("S-T6-33-weak", [P_KEY_TAB_DEAD, W_TAB_OPENS], T_KEYS, "GREEN", None),
    ("S-T6-34-prod", [P_KEY_ONE_DEAD_IN_PANE], T_KEYS, "RED", R_TOGGLE),
    ("S-T6-34-weak", [P_KEY_ONE_DEAD_IN_PANE, W_ONE_RETURNS, W_ONE_SHOWS_TABLE], T_KEYS, "GREEN", None),
    ("S-T6-35-prod", [P_DEFAULT_GRAPH], T_KEYS, "RED", R_TOGGLE),
    ("S-T6-35-weak", [P_DEFAULT_GRAPH, W_DEFAULT_TABLE, W_FOOTER_PRESETS, W_TWO_SHOWS, W_TWO_REPLACES, W_ONE_SHOWS_TABLE], T_KEYS, "GREEN", None),
    ("S-T6-36-prod", [P_GRAPH_NOT_PAINTED], T_KEYS, "RED", R_TWO_SHOWS),
    ("S-T6-36-weak", [P_GRAPH_NOT_PAINTED, W_TWO_SHOWS, W_TWO_REPLACES], T_KEYS, "GREEN", None),
    ("S-T6-36b-prod", [P_GRAPH_NOT_PAINTED], T_FOREST, "RED", R_EVERY),
    # The read-only rule. Without the pane's own keymap the table's runs
    # underneath it and k arms a kill on whatever row the cursor indexes.
    ("S-T6-37-prod", [P_NO_GRAPH_KEYMAP], T_NOACT, "RED", R_NOACT),
    ("S-T6-37-weak", [P_NO_GRAPH_KEYMAP, W_NOACT_ENQUEUE, W_NOACT_STATE, W_NOACT_STAYS, W_NOACT_CONFIRM], T_NOACT, "GREEN", None),
]


def preflight():
    """Apply every distinct patch alone and vet the package.

    A cycle whose plant does not COMPILE measures the compiler, not the rule,
    and each one costs a whole epoch run to find. This finds them all in one
    pass.

    It refuses a dirty tree for the same reason the epoch does, and the reason
    is not tidiness: the restore between patches is `git checkout --`, which
    discards uncommitted work in the files it touches. Running this over
    unstaged edits destroys them silently, and the run afterwards looks
    completely normal.
    """
    import subprocess as sp

    def sh(args):
        return sp.run(args, cwd=REPO, capture_output=True, text=True)

    dirty = sh(["git", "status", "--porcelain"]).stdout.strip().splitlines()
    dirty = [l for l in dirty if "tests/sabotage-native-task6/" not in l]
    if dirty:
        print("ABORT: tree not clean; the restore between patches would discard:")
        for line in dirty:
            print("   ", line)
        return 1

    seen, bad = set(), []
    for cid, patches, _rx, _expect, _phrase in CYCLES:
        key = tuple((p[0], p[1], p[2]) for p in patches)
        if key in seen:
            continue
        seen.add(key)
        applied = True
        for patch in patches:
            full = os.path.join(REPO, patch[0])
            src = open(full).read()
            if src.count(patch[1]) != 1:
                bad.append((cid, patch[0], "old-string count=%d" % src.count(patch[1])))
                applied = False
                break
            open(full, "w").write(src.replace(patch[1], patch[2], 1))
        if applied:
            r = sh(["go", "vet", P_UI])
            if r.returncode != 0:
                bad.append((cid, "+".join(sorted({p[0] for p in patches})),
                            r.stderr.strip().splitlines()[-1][:160]))
        sh(["git", "checkout", "--"] + SOURCES)
    print("patch sets checked:", len(seen))
    for entry in bad:
        print("  BUILD-BREAK", entry)
    print("clean" if not bad else "FIX THESE")
    return 0 if not bad else 1


def main():
    if "--preflight" in sys.argv:
        sys.exit(preflight())
    if "--count" in sys.argv:
        print(len(CYCLES))
        sys.exit(0)
    # --range A B runs CYCLES[A:B] and APPENDS to the log, so a long epoch can be
    # run as consecutive bounded chunks whose combined log is one record. The
    # chunk boundaries are recorded in the log; a chunk still refuses a dirty
    # tree and still stops the whole run on the first mismatch.
    lo, hi = 0, len(CYCLES)
    mode = "w"
    if "--range" in sys.argv:
        at = sys.argv.index("--range")
        lo, hi = int(sys.argv[at + 1]), int(sys.argv[at + 2])
        mode = "w" if lo == 0 else "a"
    logf = open(LOG, mode)

    def emit(msg):
        print(msg)
        logf.write(msg + "\n")
        logf.flush()

    def sh(args, timeout=600):
        # A plant CAN hang or blow the stack: removing the visited check is
        # exactly the mutation cycle safety needs tested. Every go test call
        # carries its own -timeout so Go kills the run well inside the
        # subprocess bound below.
        try:
            return subprocess.run(args, cwd=REPO, capture_output=True, text=True, timeout=timeout)
        except subprocess.TimeoutExpired as exc:
            # Never propagate: an exception here skips the restore and leaves a
            # plant in the tree, which is a far worse outcome than a mismatch.
            return subprocess.CompletedProcess(args, 124, exc.stdout or "", (exc.stderr or "") + "\nDRIVER: subprocess timed out")

    def gotest(rx):
        return sh(["go", "test", P_UI, "-run", rx, "-count=1", "-timeout", GO_TEST_TIMEOUT])

    def porcelain():
        # This driver's own log lives in the repo on purpose, so the evidence is
        # re-runnable from a clean checkout. It is therefore the one path
        # expected to be dirty while the run is in progress, and the only one
        # excluded here: the check still has to see any plant that failed to
        # restore.
        lines = sh(["git", "status", "--porcelain"]).stdout.strip().splitlines()
        return "\n".join(l for l in lines if "tests/sabotage-native-task6/" not in l).strip()

    def apply_patch(path, old, new):
        full = os.path.join(REPO, path)
        with open(full) as fh:
            src = fh.read()
        n = src.count(old)
        if n != 1:
            emit(f"PATCH-ABORT {path}: old-string count={n} (need exactly 1)")
            return False
        with open(full, "w") as fh:
            fh.write(src.replace(old, new, 1))
        return True

    emit(f"head: {sh(['git', 'rev-parse', 'HEAD']).stdout.strip()}")
    emit(f"go: {subprocess.run(['go', 'version'], capture_output=True, text=True).stdout.strip()}")
    emit(f"tz_offset: {time.strftime('%z')}")
    if porcelain():
        emit("ABORT: tree not clean before start")
        sys.exit(1)

    emit(f"chunk: cycles [{lo}:{hi}) of {len(CYCLES)}")
    results = []
    for cid, patches, rx, expect, phrase in CYCLES[lo:hi]:
        emit(f"\n===== {cid} expect={expect} run={rx}")
        if not all(apply_patch(*p) for p in patches):
            sh(["git", "checkout", "--"] + SOURCES)
            sys.exit(1)
        touched = sorted({p[0] for p in patches})
        diff = sh(["git", "diff", "--stat", "--"] + touched).stdout.strip()
        if not diff:
            emit("ABORT: plant left no diff in the source under test")
            sh(["git", "checkout", "--"] + SOURCES)
            sys.exit(1)
        emit("plant diffstat:\n" + diff)
        started = time.time()
        r = gotest(rx)
        out = r.stdout + r.stderr
        emit(f"exit={r.returncode} wall={time.time() - started:.1f}s")
        emit(out.strip()[-1800:])
        if expect == "RED":
            good = r.returncode != 0 and phrase in out and "no tests to run" not in out
        else:
            good = r.returncode == 0 and "ok" in out and "no tests to run" not in out
        verdict = "AS-PREDICTED" if good else "MISMATCH"
        emit(f"{cid}: {verdict}")
        sh(["git", "checkout", "--"] + SOURCES)
        dirt = porcelain()
        if dirt:
            emit(f"ABORT: restore left dirt: {dirt}")
            sys.exit(1)
        rg = gotest(rx)
        if rg.returncode != 0:
            emit(f"ABORT: post-restore green check failed:\n{rg.stdout}{rg.stderr}")
            sys.exit(1)
        emit(f"{cid}: restored, porcelain clean, focused test green again")
        results.append((cid, verdict))
        if not good:
            emit("STOP on mismatch for inspection")
            sys.exit(2)

    emit(f"\n===== SUMMARY for cycles [{lo}:{hi})")
    for cid, verdict in results:
        emit(f"{cid}: {verdict}")
    reds = sum(1 for c in CYCLES if c[3] == "RED")
    emit(f"total={len(results)} as_predicted={sum(1 for _, v in results if v == 'AS-PREDICTED')}")
    emit(f"production_RED={reds} predicted_GREEN={len(CYCLES) - reds}")


if __name__ == "__main__":
    main()
