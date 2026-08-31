#!/usr/bin/env python3
"""Sabotage addendum driver for the aitop occupancy-wiring tail tests.

For each of the 8 new tests: one physical production mutation (predicted RED
with a named rule phrase) and one weakened-assertion mutation paired with the
same production plant (predicted false GREEN). Every cycle proves the plant is
present (replace count == 1 per patch, non-empty git diff), runs the focused
test, restores via git checkout --, and proves the restore (porcelain empty,
focused test green). Stops on the first expectation mismatch.
"""
import subprocess, sys, os, json

REPO = "/home/aegis/Projects/aitop"
LOG = "/tmp/claude-1000/-home-aegis/beec7d46-5496-4bd5-ab9e-e4635720fb4c/scratchpad/sabotage_runs_pass2.log"

OCC = "internal/graph/occupancy.go"
OCCT = "internal/graph/occupancy_test.go"
MAIN = "cmd/aitop/main.go"
MAINT = "cmd/aitop/main_test.go"
ALL_FILES = [OCC, OCCT, MAIN, MAINT]

# --- production mutations -------------------------------------------------
M1 = (OCC, "\tdefault:\n\t\treturn StateUnknown\n\t}", "\tdefault:\n\t\treturn StateActive\n\t}")
M2 = (OCC,
      "\tid, err := PassiveProcessID(row.Process.PID, row.Process.StartTime)\n\tif err != nil {\n\t\treturn \"\", \"\", nil, false\n\t}\n\tproc := ProcessIdentity{PID: row.Process.PID, StartTicks: row.Process.StartTime}",
      "\tid, err := PassiveProcessID(row.Process.PID+1, row.Process.StartTime)\n\tif err != nil {\n\t\treturn \"\", \"\", nil, false\n\t}\n\tproc := ProcessIdentity{PID: row.Process.PID, StartTicks: row.Process.StartTime}")
M3 = (OCC,
      "\tif runtime == types.RuntimeUnknown || !validRole(role) {\n\t\treturn nil\n\t}",
      "\tif runtime == types.RuntimeUnknown {\n\t\treturn nil\n\t}")
M4 = (OCC,
      "\t\t\tevents = append(events, occupancyEventsForRow(row, now)...)\n\t\t\tif len(row.Children) > 0 {\n\t\t\t\twalk(row.Children)\n\t\t\t}",
      "\t\t\tevents = append(events, occupancyEventsForRow(row, now)...)")
M5 = (OCC,
      "\tif row.OverlayOK && row.Overlay.SessionID != \"\" {",
      "\tif row.OverlayOK && row.Overlay.SessionID == \"\" {")
M6 = (OCC,
      "\tfor _, event := range OccupancyEventsFromRows(c.latest(), now) {\n\t\t_, _ = sink.Publish(event)\n\t}",
      "\tfor _, event := range OccupancyEventsFromRows(c.latest(), now) {\n\t\t_ = event\n\t}")
M7 = (MAIN,
      "\tif snap != nil {\n\t\tsnap.Graph = shadow.Snapshot()\n\t}",
      "\tif snap != nil {\n\t\t_ = shadow.Snapshot()\n\t}")
M8 = (MAIN,
      "\tif reg != nil {\n\t\tif err := reg.Go(\"graph\", shadow.Run); err != nil {\n\t\t\treturn err\n\t\t}\n\t} else {\n\t\tgo func() { _ = shadow.Run(ctx) }()\n\t}",
      "\tgo func() { _ = shadow.Run(ctx) }()")

# --- weakened assertions --------------------------------------------------
W1 = (OCCT, "\t\tif !ok || data.State != StateUnknown {", "\t\tif !ok && data.State != StateUnknown {")
W2 = (OCCT,
      "\tif node.Actor != want {\n\t\tt.Fatalf(\"occupancy-passive-node-id rule violated: actor=%s want=%s\", node.Actor, want)\n\t}",
      "\t_ = want")
W3 = (OCCT, "\tif len(events) != 0 {", "\tif len(events) > 3 {")
W4 = (OCCT, "\tif !actors[parent] || !actors[child] {", "\tif !actors[parent] {")
W5 = (OCCT, "\t\tif ev.Kind == EventNodeObserved && ev.Actor == want {", "\t\tif ev.Kind == EventNodeObserved {")
W6 = (OCCT,
      "\t\tcase <-deadline:\n\t\t\tt.Fatalf(\"occupancy-shadow-publish rule violated: nodes still empty after wait snapshot=%v\", snap)",
      "\t\tcase <-deadline:\n\t\t\treturn")
W7 = (MAINT,
      "\tif occupancyMappableRows(snap.Rows) > 0 && len(snap.Graph.Nodes) == 0 {\n\t\tt.Fatalf(\"production-capture-once-publishes-graph rule violated: mappableRows=%d graphNodes=0\", occupancyMappableRows(snap.Rows))\n\t}\n",
      "")
W8 = (MAINT,
      "\t\tif reg.n.Load() != 1 || reg.name != \"graph\" || reg.run == nil {\n\t\t\tt.Fatalf(\"production-run-interactive-registers-graph rule violated: n=%d name=%s runNil=%t err=%v\", reg.n.Load(), reg.name, reg.run == nil, err)\n\t\t}",
      "\t\t_ = err")

GRAPH_PKG = "./internal/graph"
MAIN_PKG = "./cmd/aitop"

CYCLES = [
    ("S-T7-weak", [M7, W7], MAIN_PKG, "^TestProductionCaptureOncePublishesGraph$", "GREEN", None),
    ("S-T8-prod", [M8], MAIN_PKG, "^TestProductionRunInteractiveRegistersGraph$", "RED", "production-run-interactive-registers-graph rule violated"),
    ("S-T8-weak", [M8, W8], MAIN_PKG, "^TestProductionRunInteractiveRegistersGraph$", "GREEN", None),
]
_DISABLED = [
    ("S-T1-prod", [M1], GRAPH_PKG, "^TestOccupancyEventsFromRowsIdleIsUnknownPassive$", "RED", "occupancy-passive-idle-is-unknown rule violated"),
    ("S-T1-weak", [M1, W1], GRAPH_PKG, "^TestOccupancyEventsFromRowsIdleIsUnknownPassive$", "GREEN", None),
    ("S-T2-prod", [M2], GRAPH_PKG, "^TestOccupancyEventsFromRowsEmitsPassiveProcessNode$", "RED", "occupancy-passive-node-id rule violated"),
    ("S-T2-weak", [M2, W2], GRAPH_PKG, "^TestOccupancyEventsFromRowsEmitsPassiveProcessNode$", "GREEN", None),
    ("S-T3-prod", [M3], GRAPH_PKG, "^TestOccupancyEventsFromRowsSkipsIgnoreAndUnknownRuntime$", "RED", "occupancy-skip-ignore-unknown-runtime rule violated"),
    ("S-T3-weak", [M3, W3], GRAPH_PKG, "^TestOccupancyEventsFromRowsSkipsIgnoreAndUnknownRuntime$", "GREEN", None),
    ("S-T4-prod", [M4], GRAPH_PKG, "^TestOccupancyEventsFromRowsWalksChildren$", "RED", "occupancy-walks-children rule violated"),
    ("S-T4-weak", [M4, W4], GRAPH_PKG, "^TestOccupancyEventsFromRowsWalksChildren$", "GREEN", None),
    ("S-T5-prod", [M5], GRAPH_PKG, "^TestOccupancyEventsFromRowsUsesSessionIDWhenValid$", "RED", "occupancy-session-id-node rule violated"),
    ("S-T5-weak", [M5, W5], GRAPH_PKG, "^TestOccupancyEventsFromRowsUsesSessionIDWhenValid$", "GREEN", None),
    ("S-T6-prod", [M6], GRAPH_PKG, "^TestOccupancyCollectorPublishesIntoShadow$", "RED", "occupancy-shadow-publish rule violated"),
    ("S-T6-weak", [M6, W6], GRAPH_PKG, "^TestOccupancyCollectorPublishesIntoShadow$", "GREEN", None),
    ("S-T7-prod", [M7], MAIN_PKG, "^TestProductionCaptureOncePublishesGraph$", "RED", "production-capture-once-publishes-graph rule violated"),
]

log = open(LOG, "w")
def emit(s):
    log.write(s + "\n"); log.flush(); print(s)

def sh(args, **kw):
    return subprocess.run(args, cwd=REPO, capture_output=True, text=True, **kw)

def porcelain():
    return sh(["git", "status", "--porcelain"]).stdout.strip()

def apply_patch(path, old, new):
    full = os.path.join(REPO, path)
    src = open(full).read()
    n = src.count(old)
    if n != 1:
        emit(f"PATCH-ABORT {path}: old-string count={n} (need exactly 1)")
        return False
    open(full, "w").write(src.replace(old, new, 1))
    return True

if porcelain():
    emit("ABORT: tree not clean before start"); sys.exit(1)

results = []
for cid, patches, pkg, rx, expect, phrase in CYCLES:
    emit(f"\n===== {cid} expect={expect} pkg={pkg} run={rx}")
    ok = all(apply_patch(*p) for p in patches)
    if not ok:
        sh(["git", "checkout", "--"] + ALL_FILES); sys.exit(1)
    diff = sh(["git", "diff", "--stat"]).stdout.strip()
    if not diff:
        emit("ABORT: plant left no diff"); sys.exit(1)
    emit("plant diffstat:\n" + diff)
    r = sh(["go", "test", pkg, "-run", rx, "-count=1"], timeout=120)
    out = r.stdout + r.stderr
    emit(f"exit={r.returncode}")
    emit(out.strip()[-1500:])
    if expect == "RED":
        good = r.returncode != 0 and phrase in out and "no tests to run" not in out
    else:
        good = r.returncode == 0 and "ok" in out and "no tests to run" not in out
    verdict = "AS-PREDICTED" if good else "MISMATCH"
    emit(f"{cid}: {verdict}")
    sh(["git", "checkout", "--"] + ALL_FILES)
    if porcelain():
        emit(f"ABORT: restore left dirt: {porcelain()}"); sys.exit(1)
    rg = sh(["go", "test", pkg, "-run", rx, "-count=1"], timeout=120)
    if rg.returncode != 0:
        emit(f"ABORT: post-restore green check failed:\n{rg.stdout}{rg.stderr}"); sys.exit(1)
    emit(f"{cid}: restored, focused test green again")
    results.append((cid, verdict))
    if not good:
        emit("STOP on mismatch for inspection"); sys.exit(2)

emit("\n===== SUMMARY")
for cid, verdict in results:
    emit(f"{cid}: {verdict}")
emit(f"total={len(results)} as_predicted={sum(1 for _, v in results if v == 'AS-PREDICTED')}")
