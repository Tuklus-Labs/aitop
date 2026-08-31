#!/usr/bin/env python3
"""Sabotage driver for aitop native-provenance Task 1 (internal/native core).

For each new test: one physical production mutation (predicted RED with a named
rule phrase) and one weakened-assertion mutation carrying the SAME production
plant (predicted false GREEN). Every cycle proves the plant is present (exact
replace count == 1 per patch, non-empty git diff), runs the focused test,
restores via git checkout --, and proves the restore (porcelain empty, focused
test green again). Stops on the first expectation mismatch.

Run from the native-provenance worktree, on a clean tree, AFTER the production
and test code is committed: git checkout -- discards uncommitted work.
"""
import subprocess, sys, os, time

REPO = "/home/aegis/Projects/aitop/.worktrees/native-provenance"
LOG = "/tmp/claude-1000/-home-aegis/beec7d46-5496-4bd5-ab9e-e4635720fb4c/scratchpad/sabotage_runs_native_task1.log"

NAT = "internal/native/native.go"
NATT = "internal/native/native_test.go"
ALL_FILES = [NAT, NATT]
PKG = "./internal/native"

# --- production mutations -------------------------------------------------

# M1: node events publish at function return instead of before the edges. The
# edge still names correct endpoints and incarnations; only the ORDER moves.
M1 = (NAT,
      "\t\tc.publish(sink, c.nodeEvent(sighting, incarnation, now))\n",
      "\t\tdefer c.publish(sink, c.nodeEvent(sighting, incarnation, now))\n")

# M2: fold the tick time into the replay identity, so identical content stops
# replaying to one event id.
M2 = (NAT,
      "\t\tID:         graph.ImmutableEventID(c.runtime, record, location),",
      "\t\tID:         graph.ImmutableEventID(c.runtime, record+strconv.FormatInt(now.UnixNano(), 10), location),")

# M3: stop walking child rows, so a subagent's own session never binds to its
# live process.
M3 = (NAT,
      "\t\t\tif len(row.Children) > 0 {\n\t\t\t\twalk(row.Children)\n\t\t\t}\n",
      "")

# M4: claim state without opening a heartbeat lane, so the claim decays.
M4 = (NAT,
      "\t\tlanes = append(lanes, graph.NativeHealthLane{\n\t\t\tSource:           c.sourceRef(),\n\t\t\tActor:            sighting.ID,\n\t\t\tActorIncarnation: incarnation,\n\t\t})\n",
      "")

# M5: admit terminals of any age.
M5 = (NAT,
      "\treturn now.Sub(*sighting.ExitAt) > nativeExitWindow",
      "\treturn false")

# M6a: return from publish on a sink error, so saturation and rejection stop
# being counted.
M6A = (NAT,
       "\tdisposition, _ := sink.Publish(event)\n\tswitch disposition {",
       "\tdisposition, err := sink.Publish(event)\n\tif err != nil {\n\t\treturn\n\t}\n\tswitch disposition {")

# M6b: bail out of Run once the sink has rejected anything.
M6B = (NAT,
       "\t\tcase <-ticker.C:\n\t\t\tc.tick(sink)\n\t\t}",
       "\t\tcase <-ticker.C:\n\t\t\tif c.disp.Rejected.Load() > 0 {\n\t\t\t\treturn fmt.Errorf(\"native collector sink rejection rule violated: source=%s\", c.id)\n\t\t\t}\n\t\t\tc.tick(sink)\n\t\t}")

# M7: move the display bound. Events stay valid, only the truncation point
# moves, so the rune-boundary assertion is the only one that can see it.
M7 = (NAT,
      "\tmaxDisplayBytes = 128",
      "\tmaxDisplayBytes = 120")

# M7b: strip spaces instead of control runes.
M7B = (NAT,
       "\t\tif unicode.IsControl(r) {",
       "\t\tif unicode.IsSpace(r) {")

# M8: unsort the declared capabilities.
M8 = (NAT,
      "\t\t\tgraph.CapabilityIdentity,\n\t\t\tgraph.CapabilitySpawn,\n\t\t\tgraph.CapabilityState,",
      "\t\t\tgraph.CapabilityIdentity,\n\t\t\tgraph.CapabilityState,\n\t\t\tgraph.CapabilitySpawn,")

# --- weakened assertions --------------------------------------------------

W1 = (NATT,
      "\tif !parentSeen || !childSeen || parentIndex >= edgeIndex || childIndex >= edgeIndex {",
      "\tif !parentSeen || !childSeen {")

W2 = (NATT,
      "\t\tif _, ok := idsOne[ev.ID]; !ok {",
      "\t\tif len(idsOne) == 0 {")

W3 = (NATT,
      "\t\t{parent, mustProcessIncarnation(t, parentProc)},\n\t\t{second, mustProcessIncarnation(t, childProc)},",
      "\t\t{parent, mustProcessIncarnation(t, parentProc)},")

W4 = (NATT,
      "\tif beats := countKind(claimEvents, graph.EventHeartbeatObserved); beats != 1 {\n\t\tt.Fatalf(\"native-state-claim-gets-heartbeat-lane rule violated: heartbeats=%d want=1 kinds=%v\", beats, kindsOf(claimEvents))\n\t}\n\tbeat, found := firstOfKind(claimEvents, graph.EventHeartbeatObserved, parent)\n\tif !found || beat.ActorIncarnation != stateEvent.ActorIncarnation {\n\t\tt.Fatalf(\"native-heartbeat-lane-identity rule violated: found=%t heartbeatActor=%s heartbeatInc=%s stateInc=%s\",\n\t\t\tfound, beat.Actor, beat.ActorIncarnation, stateEvent.ActorIncarnation)\n\t}\n\tif beat.Source.Ref.Authority != graph.AuthorityNative {\n\t\tt.Fatalf(\"native-heartbeat-lane-authority rule violated: authority=%d want=%d\", beat.Source.Ref.Authority, graph.AuthorityNative)\n\t}\n\n",
      "")

W5A = (NATT,
       "\tfor _, ev := range events {\n\t\tif ev.Actor == staleID || ev.Target == staleID {\n\t\t\tt.Fatalf(\"native-exit-window-skips-stale-terminals rule violated: kind=%s actor=%s target=%s exitAt=%s window=%s\",\n\t\t\t\tev.Kind, ev.Actor, ev.Target, staleAt, nativeExitWindow)\n\t\t}\n\t}\n",
       "")

W5B = (NATT,
       "\tif skipped := c.staleTerminals.Load(); skipped != 1 {",
       "\tif skipped := c.staleTerminals.Load(); skipped > 1 {")

W6A = (NATT,
       "\tif rejected != uint64(len(events)) || rejected == 0 {",
       "\tif rejected > uint64(len(events)) {")

W7 = (NATT,
      "\tif len(wideData.ProvenName) != 126 || !utf8.ValidString(wideData.ProvenName) {",
      "\tif len(wideData.ProvenName) > 128 || !utf8.ValidString(wideData.ProvenName) {")

W8 = (NATT,
      "\tif _, err := graph.NewShadow(graph.DefaultReconcileConfig(), graph.DefaultStoreConfig(), c); err != nil {\n\t\tt.Fatalf(\"native-descriptor-accepted-by-shadow rule violated: err=%v descriptor=%+v\", err, descriptor)\n\t}",
      "\tshadow, _ := graph.NewShadow(graph.DefaultReconcileConfig(), graph.DefaultStoreConfig(), c)\n\t_ = shadow")

W9 = (NATT,
      "\t\t\tt.Fatalf(\"native-shadow-spawn-edge rule violated: no edge after 5s nodes=%d edges=%d published=%d rejected=%d scanCalls=%d\",\n\t\t\t\tnodes, edges, c.Disp().Published.Load(), c.Disp().Rejected.Load(), sc.callCount())",
      "\t\t\t_ = nodes\n\t\t\t_ = edges\n\t\t\treturn")

T1 = "^TestNativeEmitOrderNodesBeforeEdges$"
T2 = "^TestNativeEventIDsDeterministicAcrossTicks$"
T3 = "^TestNativeIncarnationPrefersLiveProcess$"
T4 = "^TestNativeStateClaimGetsHeartbeatLane$"
T5 = "^TestNativeExitWindowSkipsStaleTerminals$"
T6 = "^TestNativeRejectionDoesNotStopRun$"
T7 = "^TestNativeDisplayBoundsAndUTF8$"
T8 = "^TestNativeDescriptorValid$"
T9 = "^TestNativeSpawnEdgeLandsInRealShadow$"

CYCLES = [
    ("S-N1-prod", [M1], T1, "RED", "native-emit-order-nodes-before-edges rule violated: parentIdx="),
    ("S-N1-weak", [M1, W1], T1, "GREEN", None),
    ("S-N2-prod", [M2], T2, "RED", "native-replay-identity-stable rule violated: tick2 event"),
    ("S-N2-weak", [M2, W2], T2, "GREEN", None),
    ("S-N3-prod", [M3], T3, "RED", "native-incarnation-prefers-live-process rule violated"),
    ("S-N3-weak", [M3, W3], T3, "GREEN", None),
    ("S-N4-prod", [M4], T4, "RED", "native-state-claim-gets-heartbeat-lane rule violated: heartbeats=0"),
    ("S-N4-weak", [M4, W4], T4, "GREEN", None),
    ("S-N5-prod", [M5], T5, "RED", "native-exit-window-skips-stale-terminals rule violated"),
    ("S-N5-weak", [M5, W5A, W5B], T5, "GREEN", None),
    ("S-N6-prod", [M6A], T6, "RED", "native-rejection-counted rule violated: rejected=0"),
    ("S-N6-weak", [M6A, W6A], T6, "GREEN", None),
    ("S-N6b-prod", [M6B], T6, "RED", "native-rejection-does-not-stop-run rule violated: scanCalls=1"),
    ("S-N7-prod", [M7], T7, "RED", "native-display-rune-boundary rule violated: bytes=120"),
    ("S-N7-weak", [M7, W7], T7, "GREEN", None),
    ("S-N7b-prod", [M7B], T7, "RED", "native-display-events-validate rule violated"),
    ("S-N8-prod", [M8], T8, "RED", "native-descriptor-accepted-by-shadow rule violated"),
    ("S-N8-weak", [M8, W8], T8, "GREEN", None),
    ("S-N9-prod", [M1], T9, "RED", "native-shadow-spawn-edge rule violated: no edge after 5s"),
    ("S-N9-weak", [M1, W9], T9, "GREEN", None),
]

logf = open(LOG, "w")


def emit(msg):
    print(msg)
    logf.write(msg + "\n")
    logf.flush()


def sh(args, timeout=180):
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
    emit(out.strip()[-1500:])
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
