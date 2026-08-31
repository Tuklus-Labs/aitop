#!/usr/bin/env python3
"""Sabotage driver for Task 2 fix round 1 (synthesized-parent path).

Covers the assertions added or changed by review findings 1 and 2, plus a
re-run of the three earlier S-C4 cycles, whose weakening sets moved when the
test around them grew: a gate's discriminating power is a property of its
environment, so the motivating mutation is re-run wherever the gate moved.

Same protocol as the first epoch: code committed first, exact-match plants,
focused test, restore via git checkout --, porcelain proof, green re-check.
"""
import subprocess, sys, os, time

REPO = "/home/aegis/Projects/aitop/.worktrees/native-provenance"
LOG = "/tmp/claude-1000/-home-aegis/beec7d46-5496-4bd5-ab9e-e4635720fb4c/scratchpad/sabotage_runs_native_task2_fix1.log"

CLA = "internal/native/claude.go"
CLAT = "internal/native/claude_test.go"
NAT = "internal/native/native.go"
NATT = "internal/native/native_test.go"
ALL_FILES = [CLA, CLAT, NAT, NATT]
PKG = "./internal/native"

# --- production mutations -------------------------------------------------

# F1a: anchor any accepted child, admissible or not, so a store whose children
# the core will all drop still synthesizes a session for them.
P_ANCHOR_ANY = (CLA,
                '\t\tif anchor.location == "" && !terminalOutsideWindow(sighting, now) {',
                '\t\tif anchor.location == "" {')

# F1b: same defect reached from the other side: drop the store-level
# short-circuit that refuses to synthesize a parent for an unpublishable store.
P_NO_SHORTCIRCUIT = (CLA,
                     '\t\t\tif anchor.location == "" {\n\t\t\t\t// Every child here carries a terminal the core will drop, so a\n\t\t\t\t// session synthesized for them would outlive every child it\n\t\t\t\t// exists for, with no edge, no state and no terminal to end it.\n\t\t\t\tcontinue\n\t\t\t}\n',
                     '')

# F2a: synthesize an orphan's parent with no terminal at all, the immortal
# nameless primary the review found.
P_NO_PARENT_TERMINAL = (CLA,
                        '\t\tvanishedAt := anchor.newestActivity\n\t\tsighting.Exit = graph.OutcomeVanished\n\t\tsighting.ExitAt = &vanishedAt\n',
                        '')

# F2b: date the store from the meta mtime rather than the newest activity, so a
# session that was working seconds ago is recorded as having died minutes ago
# and falls out of the exit window with its children still inside it.
P_ANCHOR_META_DATE = (CLA,
                      '\t\tif activity.After(anchor.newestActivity) {\n\t\t\tanchor.newestActivity = activity\n\t\t}',
                      '\t\tif info.ModTime().After(anchor.newestActivity) {\n\t\t\tanchor.newestActivity = info.ModTime()\n\t\t}')

# F2c: invent a terminal for every synthesized parent, including sessions whose
# roster entry is merely stale and whose death nothing witnessed.
P_ALWAYS_VANISH = (CLA,
                   "\tif orphaned {",
                   "\tif true {")

# S-C4b re-run: date a CHILD's terminal at the meta mtime instead of its last
# activity (unchanged code; re-run because the gate around it moved).
P_CHILD_VANISH_AT = (CLA,
                     "\t\t\tvanishedAt := activity",
                     "\t\t\tvanishedAt := info.ModTime()")

# S-C4a re-run: widen the activity window.
P_ACTIVEWINDOW = (CLA,
                  "\tclaudeChildActiveWindow = 30 * time.Second",
                  "\tclaudeChildActiveWindow = 5 * time.Minute")

# S-C4c re-run: never vanish a child whose parent is gone.
P_NEVERVANISH = (CLA,
                 "\t\t\tvanishedAt := activity\n\t\t\tsighting.Exit = graph.OutcomeVanished\n\t\t\tsighting.ExitAt = &vanishedAt\n",
                 "")

# --- weakened assertions --------------------------------------------------

W_BURIED_PARENT = (CLAT,
                   '\tif _, ok := nodeByID(nodes, buriedParent); ok {\n\t\tt.Fatalf("claude-buried-store-synthesizes-no-parent rule violated: session=%s present with every child past window=%s ids=%v", claudeFixtureOrphanOld, nativeExitWindow, nodeIDs(nodes))\n\t}\n',
                   '')
W_NODECOUNT = (CLAT,
               "\tif len(nodes) != 8 {",
               "\tif len(nodes) < 8 {")
# Both plants that touch this count make it FALL (a node drops out of the exit
# window), so the weakening that admits the plant is the > form. The < form was
# tried first and still fired, which is the direction check this comment exists
# to keep: a relaxed comparison only helps on the side the value actually moved.
W_PUBCOUNT = (CLAT,
              "\tif published := countKind(events, graph.EventNodeObserved); published != 7 {",
              "\tif published := countKind(events, graph.EventNodeObserved); published > 7 {")
W_STALECOUNT = (CLAT,
                "\tif skipped := collector.staleTerminals.Load(); skipped != 1 {",
                "\tif skipped := collector.staleTerminals.Load(); skipped < 1 {")
W_PARENT_TERMINAL = (CLAT,
                     '\tif orphanParent.Exit != graph.OutcomeVanished || orphanParent.ExitAt == nil || !orphanParent.ExitAt.Equal(orphanAt) {',
                     '\tif false {')
W_PARENT_EXIT_EVENT = (CLAT,
                       "\tif _, ok := firstOfKind(events, graph.EventExitObserved, orphanParentID); !ok {",
                       "\tif _, ok := firstOfKind(events, graph.EventExitObserved, orphanParentID); false && !ok {")
W_HOST_TERMINAL = (CLAT,
                   '\tif hostParent.Exit != "" || hostParent.ExitAt != nil {',
                   "\tif false {")
W_CHILD_EXITAT = (CLAT,
                  "\tif orphan.ExitAt == nil || !orphan.ExitAt.Equal(orphanAt) {",
                  "\tif orphan.ExitAt == nil {")
W_FRESHORPHAN = (CLAT,
                 "\tif _, ok := firstOfKind(events, graph.EventExitObserved, orphanID); !ok {",
                 "\tif _, ok := firstOfKind(events, graph.EventExitObserved, orphanID); false && !ok {")
W_QUIET = (CLAT,
           '\tif quiet.State != "" || quiet.Exit != "" {',
           '\tif quiet.Exit != "" {')

T_C4 = "^TestClaudeScannerVanishesOrphanedChildren$"

CYCLES = [
    ("S-F1a-prod", [P_ANCHOR_ANY], T_C4, "RED", "claude-buried-store-synthesizes-no-parent rule violated"),
    ("S-F1a-weak", [P_ANCHOR_ANY, W_BURIED_PARENT, W_NODECOUNT, W_STALECOUNT], T_C4, "GREEN", None),
    ("S-F1b-prod", [P_NO_SHORTCIRCUIT], T_C4, "RED", "claude-buried-store-synthesizes-no-parent rule violated"),

    ("S-F2a-prod", [P_NO_PARENT_TERMINAL], T_C4, "RED", "claude-orphan-parent-vanishes-with-its-children rule violated"),
    ("S-F2a-weak", [P_NO_PARENT_TERMINAL, W_PARENT_TERMINAL, W_PARENT_EXIT_EVENT], T_C4, "GREEN", None),
    ("S-F2b-prod", [P_ANCHOR_META_DATE], T_C4, "RED", "claude-orphan-parent-vanishes-with-its-children rule violated"),
    # Four sites, two more than predicted: a mis-dated parent falls out of the
    # exit window, which moves the published-node count, the stale-terminal
    # counter and the parent exit event together.
    ("S-F2b-weak", [P_ANCHOR_META_DATE, W_PARENT_TERMINAL, W_PUBCOUNT, W_STALECOUNT, W_PARENT_EXIT_EVENT], T_C4, "GREEN", None),
    ("S-F2c-prod", [P_ALWAYS_VANISH], T_C4, "RED", "claude-present-sidecar-invents-no-terminal rule violated"),
    ("S-F2c-weak", [P_ALWAYS_VANISH, W_HOST_TERMINAL], T_C4, "GREEN", None),

    # Re-runs of the first epoch's S-C4 cycles against the grown test.
    ("S-C4a-rerun-prod", [P_ACTIVEWINDOW], T_C4, "RED", "claude-quiet-child-makes-no-claim rule violated"),
    ("S-C4a-rerun-weak", [P_ACTIVEWINDOW, W_QUIET], T_C4, "GREEN", None),
    ("S-C4b-rerun-prod", [P_CHILD_VANISH_AT], T_C4, "RED", "claude-orphan-exitat-is-last-activity rule violated"),
    # No weak pair: the first epoch greened this plant with three weakened
    # sites, and it cannot any more. A child mis-dated out of the exit window
    # now makes its whole store inadmissible, so Finding 1's gate withholds
    # the parent too and SEVEN assertions fire (child date, parent presence,
    # parent terminal, node count, published count, stale counter, both exit
    # events). Weakening all seven would only prove that deleting the test
    # passes it; the finding is that the gate got strictly stronger.
    ("S-C4c-rerun-prod", [P_NEVERVANISH], T_C4, "RED", "claude-orphan-child-vanishes rule violated"),
]

logf = open(LOG, "w")


def emit(msg):
    print(msg)
    logf.write(msg + "\n")
    logf.flush()


def sh(args, timeout=300):
    return subprocess.run(args, cwd=REPO, capture_output=True, text=True, timeout=timeout)


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


emit(f"head: {sh(['git', 'rev-parse', 'HEAD']).stdout.strip()}")
emit(f"go: {subprocess.run(['go', 'version'], capture_output=True, text=True).stdout.strip()}")
if porcelain():
    emit("ABORT: tree not clean before start")
    sys.exit(1)

results = []
for cid, patches, rx, expect, phrase in CYCLES:
    emit(f"\n===== {cid} expect={expect} run={rx}")
    if not all(apply_patch(*p) for p in patches):
        sh(["git", "checkout", "--"] + ALL_FILES)
        sys.exit(1)
    diff = sh(["git", "diff", "--stat"]).stdout.strip()
    if not diff:
        emit("ABORT: plant left no diff")
        sys.exit(1)
    emit("plant diffstat:\n" + diff)
    started = time.time()
    r = sh(["go", "test", PKG, "-run", rx, "-count=1"])
    out = r.stdout + r.stderr
    emit(f"exit={r.returncode} wall={time.time() - started:.1f}s")
    emit(out.strip()[-1800:])
    if expect == "RED":
        good = r.returncode != 0 and phrase in out and "no tests to run" not in out
    else:
        good = r.returncode == 0 and "ok" in out and "no tests to run" not in out
    verdict = "AS-PREDICTED" if good else "MISMATCH"
    emit(f"{cid}: {verdict}")
    sh(["git", "checkout", "--"] + ALL_FILES)
    if porcelain():
        emit(f"ABORT: restore left dirt: {porcelain()}")
        sys.exit(1)
    rg = sh(["go", "test", PKG, "-run", rx, "-count=1"])
    if rg.returncode != 0:
        emit(f"ABORT: post-restore green check failed:\n{rg.stdout}{rg.stderr}")
        sys.exit(1)
    emit(f"{cid}: restored, porcelain clean, focused test green again")
    results.append((cid, verdict))
    if not good:
        emit("STOP on mismatch for inspection")
        sys.exit(2)

emit("\n===== SUMMARY")
for cid, verdict in results:
    emit(f"{cid}: {verdict}")
emit(f"total={len(results)} as_predicted={sum(1 for _, v in results if v == 'AS-PREDICTED')}")
