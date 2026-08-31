#!/usr/bin/env python3
"""Sabotage driver for Task 7 (the --screenshot-graph flag).

Protocol, identical to Tasks 1 through 6: the code is committed FIRST, each
cycle plants an exact-match mutation, proves the plant is present (replace
count == 1 per patch and a non-empty git diff restricted to the sources under
test), runs the focused test with -count=1, restores with `git checkout --`,
proves the restore (porcelain empty), and re-runs the focused test green. A
cycle that does not match its prediction stops the run for inspection rather
than being recorded and moved past.

Mutations are chosen by CLAIM. The test under measurement makes five separate
claims about one frame -- its size, its pane identity, the nodes it drew, the
empty state it did NOT draw, and the table flag it did not disturb -- plus one
about the flag combinations run() refuses. Every one of them has a plant aimed
at IT, and each weakened cycle names the exact assertion sites relaxed.

Three cycles are worth reading before the rest.

S-T7-02 and S-T7-03 are the same defect at two different lines: the graph flag
wired to the TABLE renderer, once in productionRunDeps and once at the dispatch
in run(). Both are edits a real author makes (copy-paste in the deps literal,
copy-paste in the dispatch). Both need THREE assertions relaxed to green, one
more than predicted: the table frame is missing the pane's border tab and the
graph's node name, and its empty state carries no second canary line. The pair
is recorded rather than collapsed because the two lines fail independently:
fixing one leaves the other.

S-T7-04 was re-aimed after its first form went GREEN. See P_NAME_BY_ID.

S-T7-08 is the cycle that earns the eleven rows added to
TestRunRejectsScreenshotWithJSONOrOnce. Its weakened form deletes exactly those
rows and nothing else, so the GREEN says the rows are what caught the dropped
guard, not the rows that were already there.

Two corrections were made against a running epoch and both are recorded here
rather than quietly fixed: the S-T7-02 relaxation count, and the S-T7-04 aim.
Neither was visible by reading the plant; both took running it.
"""
import os
import subprocess
import sys
import time

REPO = "/home/aegis/Projects/aitop/.worktrees/native-provenance"
LOG = os.path.join(REPO, "tests/sabotage-native-task7/sabotage_runs_native_task7.log")

GO_TEST_TIMEOUT = "90s"

MAIN = "cmd/aitop/main.go"
TST = "cmd/aitop/main_test.go"
VIE = "internal/ui/view.go"
GRA = "internal/ui/graph.go"

SOURCES = [MAIN, TST, VIE, GRA]

P_CMD = "./cmd/aitop"

T_FRAME = "^TestScreenshotGraphRendersTheGraphPane$"
T_REJECT = "^TestRunRejectsScreenshotWithJSONOrOnce$"

# --- production plants ------------------------------------------------------

# The pane's border tab renamed. Aimed at the identity assertion alone: the
# pane still draws its nodes, still fills 42 lines, and the word "graph" is
# still in the frame (the pane's own key row offers `1 table`, the table's
# offers `2 graph`), so only an assertion reading the BORDER can see this.
P_TAB_RENAMED = (VIE,
                 '\tleft := []string{s.tab(s.Title.Render(" graph "))}',
                 '\tleft := []string{s.tab(s.Title.Render(" forest "))}')

# The graph flag wired to the table renderer in productionRunDeps.
P_DEPS_TABLE_RENDER = (MAIN,
                       "\t\t\treturn ui.RenderGraph(s, th, w, h, now)",
                       "\t\t\treturn ui.Render(s, th, w, h, now)")

# The same defect at the dispatch: the graph flag reaching for the table's
# renderer out of the deps struct.
P_DISPATCH_TABLE_RENDER = (MAIN,
                           "\t\treturn runScreenshot(ctx, stdout, stderr, opt, deps, opt.ScreenshotGraph, deps.RenderGraphScreenshot)",
                           "\t\treturn runScreenshot(ctx, stdout, stderr, opt, deps, opt.ScreenshotGraph, deps.RenderScreenshot)")

# The ProvenName preference dropped, so every node is named by its ID tail.
#
# The first aim of this plant was the ID-TAIL fallback, and it went GREEN: the
# fixture node carries a ProvenName, so the fallback is a branch this test never
# reaches, and emptying it demonstrated nothing about the claim. Being right
# that "the pane draws the name it was given" is one claim; knowing which line
# carries it is another. The fallback branch is covered where it is exercised,
# by internal/ui's own TestGraphPaneRendersSpawnForest, whose grok node is
# deliberately unnamed.
P_NAME_BY_ID = (GRA,
                '\tif n.ProvenName != "" {\n\t\treturn n.ProvenName\n\t}\n\tid := string(n.ID)',
                "\tid := string(n.ID)")

# The empty pane painting a blank centred line instead of the canary. The frame
# still has its 42 lines and its border tab; a zero-row pane and a dead
# collector become byte-identical, which is the exact thing the canary exists
# to prevent.
P_EMPTY_BLANK = (VIE,
                 "\t\t\t\tline = centre(s.Hi.Render(snapshot.Canary), w-2)",
                 '\t\t\t\tline = centre(s.Hi.Render(""), w-2)')

# The requested height ignored in favour of a fixed one. Aimed at the size
# assertion: the pane, its tab, and its rows are all still correct.
P_SIZE_PINNED = (MAIN,
                 "\tframe := render(snap, th, width, height, now)",
                 "\tframe := render(snap, th, width, 24, now)")

# The TABLE flag wired to the graph renderer: the mirror image of S-T7-02, and
# the plant the negative half of the test exists for.
P_TABLE_FLAG_GRAPH = (MAIN,
                      "\t\t\treturn ui.Render(s, th, w, h, now)",
                      "\t\t\treturn ui.RenderGraph(s, th, w, h, now)")

# The whole graph exclusivity guard dropped, so --screenshot-graph combines
# freely with --json, --once, and --screenshot.
P_NO_GRAPH_GUARD = (MAIN,
                    "\t// Both screenshot flags write one frame to stdout and exit. Asking for both\n"
                    "\t// asks for two frames down one pipe with no ordering anyone could rely on.\n"
                    '\tif opt.ScreenshotGraphSet && (opt.ScreenshotGraph == "" || opt.JSON || opt.Once || opt.ScreenshotSet) {\n'
                    "\t\twriteDiag(stderr, diagnosticScopeUsage, diagnosticTaskFlags, errUsage)\n"
                    "\t\treturn 2\n"
                    "\t}\n",
                    "")

# --- weakenings (assertion sites, one per patch) ----------------------------

# The border tab assertion relaxed to "the border has a tab on it", which is
# true of every pane aitop draws.
W_TAB_ANY = (TST,
             '\t\t\tif !strings.Contains(frame, "┤ graph ├") {',
             '\t\t\tif !strings.Contains(frame, "┤ ") {')

# The node-count comparison disabled at its CONDITION, leaving the statement
# and its message in place, so the diff is one plausible-looking line.
W_NAME = (TST,
          "\t\t\tif got := graphFrameLinesWith(frame, graphFrameName); got != tc.wantName {",
          "\t\t\tif got := graphFrameLinesWith(frame, graphFrameName); got < 0 {")

W_CANARY = (TST,
            "\t\t\tif got := graphFrameLinesWith(frame, snapshot.Canary); got != tc.wantCanary {",
            "\t\t\tif got := graphFrameLinesWith(frame, snapshot.Canary); got < 0 {")

W_SIZE = (TST,
          "\t\t\tif len(lines) != 42 {",
          "\t\t\tif len(lines) < 0 {")

# The negative half's needle changed to one the pane cannot draw. `false` would
# orphan the frame binding and the cycle would measure the compiler.
W_TABLE_ALONE = (TST,
                 '\t\tif frame := ansi.Strip(stdout.String()); strings.Contains(frame, "┤ graph ├") {',
                 '\t\tif frame := ansi.Strip(stdout.String()); strings.Contains(frame, "┤ graph ├ never") {')

# Exactly the eleven rows this task added, and nothing else.
W_DROP_GRAPH_ROWS = (TST,
                     '\t\t{"--screenshot-graph="},\n'
                     '\t\t{"--json", "--screenshot-graph="},\n'
                     '\t\t{"--once", "--screenshot-graph="},\n'
                     '\t\t{"--screenshot-graph=", "--json"},\n'
                     '\t\t{"--screenshot-graph=", "--once"},\n'
                     '\t\t{"--screenshot-graph=120x40", "--json"},\n'
                     '\t\t{"--screenshot-graph=120x40", "--once"},\n'
                     '\t\t{"--json", "--screenshot-graph=120x40"},\n'
                     '\t\t{"--once", "--screenshot-graph=120x40"},\n'
                     "\t\t// Two frames, one stdout, no ordering worth defining.\n"
                     '\t\t{"--screenshot=120x40", "--screenshot-graph=120x40"},\n'
                     '\t\t{"--screenshot-graph=120x40", "--screenshot=120x40"},\n',
                     "")

# --- rule phrases -----------------------------------------------------------

R_PANE = "screenshot-graph-renders-the-graph-pane rule violated"
R_SIZE = "screenshot-graph-renders-the-requested-size rule violated"
R_NODES = "screenshot-graph-draws-the-nodes-it-was-given rule violated"
R_QUIET = "screenshot-graph-empty-is-not-quiet rule violated"
R_TABLE = "screenshot-graph-leaves-the-table-flag-alone rule violated"
R_REJECT = "run-rejects-screenshot-with-json-or-once rule violated"

CYCLES = [
    # --- pane identity ------------------------------------------------------
    ("S-T7-01-prod", [P_TAB_RENAMED], T_FRAME, "RED", R_PANE),
    ("S-T7-01-weak", [P_TAB_RENAMED, W_TAB_ANY], T_FRAME, "GREEN", None),

    # The wrong renderer, twice. Three assertions have to be relaxed, not the
    # two predicted on the first run: the table frame has neither the pane's
    # border tab nor the graph's node name, AND its empty state carries no
    # second canary line, because only the graph pane paints one. Measured, not
    # reasoned: the first epoch stopped here with the canary assertion still
    # firing on the `empty` subtest while `nodes` had gone fully green.
    ("S-T7-02-prod", [P_DEPS_TABLE_RENDER], T_FRAME, "RED", R_PANE),
    ("S-T7-02-weak", [P_DEPS_TABLE_RENDER, W_TAB_ANY, W_NAME, W_CANARY], T_FRAME, "GREEN", None),
    ("S-T7-03-prod", [P_DISPATCH_TABLE_RENDER], T_FRAME, "RED", R_PANE),
    ("S-T7-03-weak", [P_DISPATCH_TABLE_RENDER, W_TAB_ANY, W_NAME, W_CANARY], T_FRAME, "GREEN", None),

    # --- what the pane drew -------------------------------------------------
    ("S-T7-04-prod", [P_NAME_BY_ID], T_FRAME, "RED", R_NODES),
    ("S-T7-04-weak", [P_NAME_BY_ID, W_NAME], T_FRAME, "GREEN", None),
    ("S-T7-05-prod", [P_EMPTY_BLANK], T_FRAME, "RED", R_QUIET),
    ("S-T7-05-weak", [P_EMPTY_BLANK, W_CANARY], T_FRAME, "GREEN", None),

    # --- the frame's geometry ----------------------------------------------
    ("S-T7-06-prod", [P_SIZE_PINNED], T_FRAME, "RED", R_SIZE),
    ("S-T7-06-weak", [P_SIZE_PINNED, W_SIZE], T_FRAME, "GREEN", None),

    # --- the negative half --------------------------------------------------
    ("S-T7-07-prod", [P_TABLE_FLAG_GRAPH], T_FRAME, "RED", R_TABLE),
    ("S-T7-07-weak", [P_TABLE_FLAG_GRAPH, W_TABLE_ALONE], T_FRAME, "GREEN", None),

    # --- the flag combinations run() refuses --------------------------------
    ("S-T7-08-prod", [P_NO_GRAPH_GUARD], T_REJECT, "RED", R_REJECT),
    ("S-T7-08-weak", [P_NO_GRAPH_GUARD, W_DROP_GRAPH_ROWS], T_REJECT, "GREEN", None),
]


def preflight():
    """Apply every distinct patch set alone and vet the package.

    A cycle whose plant does not COMPILE measures the compiler, not the rule,
    and each one otherwise costs a whole epoch run to find.

    It refuses a dirty tree, and the reason is not tidiness: the restore
    between patches is `git checkout --`, which discards uncommitted work in
    the files it touches. Running this over unstaged edits destroys them
    silently, and the run afterwards looks completely normal.
    """
    import subprocess as sp

    def sh(args):
        return sp.run(args, cwd=REPO, capture_output=True, text=True)

    dirty = sh(["git", "status", "--porcelain"]).stdout.strip().splitlines()
    dirty = [l for l in dirty if "tests/sabotage-native-task7/" not in l]
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
            r = sh(["go", "vet", P_CMD])
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
    # --range A B runs CYCLES[A:B] and APPENDS to the log, so a long epoch can
    # be run as consecutive bounded chunks whose combined log is one record.
    # A chunk still refuses a dirty tree and still stops on the first mismatch.
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
        try:
            return subprocess.run(args, cwd=REPO, capture_output=True, text=True, timeout=timeout)
        except subprocess.TimeoutExpired as exc:
            # Never propagate: an exception here skips the restore and leaves a
            # plant in the tree, which is far worse than a mismatch.
            return subprocess.CompletedProcess(args, 124, exc.stdout or "", (exc.stderr or "") + "\nDRIVER: subprocess timed out")

    def gotest(rx):
        return sh(["go", "test", P_CMD, "-run", rx, "-count=1", "-timeout", GO_TEST_TIMEOUT])

    def porcelain():
        # This driver's own log lives in the repo on purpose, so the evidence is
        # re-runnable from a clean checkout. It is therefore the one path
        # expected to be dirty while the run is in progress, and the only one
        # excluded here: the check still has to see any plant that failed to
        # restore.
        lines = sh(["git", "status", "--porcelain"]).stdout.strip().splitlines()
        return "\n".join(l for l in lines if "tests/sabotage-native-task7/" not in l).strip()

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
        # A t.Fatalf prints its rule phrase FIRST and the offending state after,
        # and the offending state here is a whole 150x42 frame. A tail-only
        # excerpt therefore logs several kilobytes of box drawing and elides the
        # one line naming which rule moved, which is exactly what the record
        # exists for. Keep the head too. The verdict below reads the UNTRUNCATED
        # output, so this only ever cost the log, never a wrong call.
        text = out.strip()
        if len(text) > 2000:
            emit(text[:1200] + "\n...[middle elided]...\n" + text[-800:])
        else:
            emit(text)
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
