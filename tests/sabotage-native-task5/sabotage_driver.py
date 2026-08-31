#!/usr/bin/env python3
"""Sabotage driver for Task 5 (production wiring and the fork-runtime fix).

Protocol, identical to Tasks 1 through 4: the code is committed FIRST, each
cycle plants an exact-match mutation, proves the plant is present (replace count
== 1 per patch and a non-empty git diff restricted to the sources under test),
runs the focused test with -count=1, restores with `git checkout --`, proves the
restore (porcelain empty), and re-runs the focused test green. A cycle that does
not match its prediction stops the run for inspection rather than being recorded
and moved past.

This task cuts across six packages, so a cycle names its own package as well as
its own test. Mutations are chosen by CLAIM: each is aimed at a named assertion,
and the weakening column names every site that has to be relaxed before the
plant passes.

Two cycles are worth reading before the rest. S-T5-14 removes the joiner's role
while LEAVING the sidecar runtime in place: that is the state a fix following
the brief's letter would have shipped, and the fork child is still invisible.
S-T5-25 restores the pre-fix collector ordering, which is what put three
collision gaps and a Partial occupancy source into every one-shot on this box.
"""
import os
import subprocess
import sys
import time

REPO = "/home/aegis/Projects/aitop/.worktrees/native-provenance"
LOG = os.path.join(REPO, "tests/sabotage-native-task5/sabotage_runs_native_task5.log")

# Go's own per-run bound. It has to be shorter than the subprocess timeout, so a
# plant that removes a wait's bound is killed by Go (which panics with a stack
# naming the blocked goroutine) rather than by the harness (which would have to
# abandon the restore).
GO_TEST_TIMEOUT = "90s"

NAT = "internal/native/native.go"
NATT = "internal/native/native_test.go"
CLA = "internal/native/claude.go"
CLAT = "internal/native/claude_test.go"
COD = "internal/native/codex.go"
CODT = "internal/native/codex_test.go"
GRK = "internal/native/grok.go"
GRKT = "internal/native/grok_test.go"
SID = "internal/act/sidecar.go"
SIDT = "internal/act/sidecar_test.go"
FRK = "internal/overlay/forks/forks.go"
FRKT = "internal/overlay/forks/forks_test.go"
JOI = "internal/join/join.go"
JOIT = "internal/join/join_test.go"
ATT = "internal/snapshot/graph_attach.go"
ATTT = "internal/snapshot/graph_attach_test.go"
JSN = "internal/snapshot/json_v2.go"
MAI = "cmd/aitop/main.go"
MAIT = "cmd/aitop/main_test.go"

SOURCES = [NAT, NATT, CLA, CLAT, COD, CODT, GRK, GRKT, SID, SIDT, FRK, FRKT,
           JOI, JOIT, ATT, ATTT, JSN, MAI, MAIT]

P_NATIVE = "./internal/native"
P_ACT = "./internal/act"
P_FORKS = "./internal/overlay/forks"
P_JOIN = "./internal/join"
P_SNAP = "./internal/snapshot"
P_MAIN = "./cmd/aitop"

T_CLA_LIVE = "^TestClaudeScannerLiveSessionIsInsideHorizon$"
T_COD_LIVE = "^TestCodexScannerLiveThreadIsInsideHorizon$"
T_GRK_LIVE = "^TestGrokScannerLiveSessionIsInsideHorizon$"
T_SID_RT = "^TestForkSidecarCarriesRuntime$"
T_SID_FOLLOW = "^TestForkSidecarRuntimeFollowsTheTarget$"
T_FRK = "^TestForksOverlayReadsRuntime$"
T_JOIN_ROLE = "^TestForkChildRowCarriesSubagentRole$"
T_ATT_REG = "^TestAttachGraphRegistersNativeCollectors$"
T_ATT_EMPTY = "^TestAttachGraphEmptyHomesIsOccupancyOnly$"
T_ATT_BIND = "^TestAttachGraphBindsNativeNodesToLiveProcesses$"
T_SCHEMA = "^TestSchema2WritesUnclaimedStateAsUnknown$"
T_CAP_HOMES = "^TestProductionCaptureOncePassesEngineHomes$"
T_INT_HOMES = "^TestProductionRunInteractivePassesEngineHomes$"
T_ORDER = "^TestProductionCaptureOnceFillsRowsBeforeRunningCollectors$"
T_ATT_ABS = "^TestAttachGraphResolvesRelativeHomes$"

# Fix round 1: the one-shot's readiness wait, and the portable ordering test.
T_TICK_AFTER = "^TestCollectorSignalsItsFirstTickAfterItFinishes$"
T_TICK_ERR = "^TestCollectorSignalsItsFirstTickWhenTheScanFails$"
T_TICK_STAYS = "^TestCollectorFirstTickStaysClosedAcrossTicks$"
T_TICK_NODES = "^TestCollectorFirstTickNodesAreWhatTheStoreTook$"
T_TICK_NODES_ONE = "^TestCollectorFirstTickNodesStopAtTheFirstTick$"
T_EV_IN = "^TestWaitNativeEvidenceReturnsWhenEveryLaneIsIn$"
T_EV_BOUND = "^TestWaitNativeEvidenceIsBoundedByItsBudget$"
T_EV_SHARED = "^TestWaitNativeEvidenceSharesOneBudgetAcrossLanes$"
T_LANES = "^TestAttachGraphReturnsOneLanePerHome$"
T_CAP_UNDER = "^TestProductionCaptureOnceWaitsForANativeLaneUnderTheCap$"
T_CAP_OVER = "^TestProductionCaptureOnceReturnsAtTheCapForASlowerLane$"
T_BUDGET = "^TestNativeFirstTickBudgetIsTwoSeconds$"
T_DARK = "^TestDarkLocalUnitNeedsNoSpine$"

# Fix wave 2: the newborn binding rule, and the screenshot size hole.
T_NEWBORN = "^TestNewbornSightingWaitsOneTickForItsProcessBinding$"
T_NEWBORN_ONCE = "^TestNewbornSightingIsWaitedForOnlyOnce$"
T_STALE_NOW = "^TestStaleUnboundSightingPublishesImmediately$"
T_BOUND_NOW = "^TestBoundSightingPublishesImmediately$"
T_SHOT = "^TestScreenshotGraphRendersTheGraphPane$"

# --- fix wave 2: the newborn binding rule ---------------------------------

# The rule removed. This is the code as fix round 1 shipped it, and it
# establishes a newborn under an invocation incarnation that nothing can rotate.
P_NB_NO_WAIT = (NAT,
                "\t\tif c.awaitBinding(processes, sighting, now) {",
                "\t\tif false && c.awaitBinding(processes, sighting, now) {")
# The grace given every tick instead of once, which makes a session aitop can
# never bind permanently invisible rather than one poll late.
P_NB_ALWAYS = (NAT,
               "\tif c.awaitingBinding[sighting.ID] {\n\t\treturn false\n\t}\n",
               "")
# Bound sightings held back too, so every session on the box pays a poll and the
# rule fires precisely where it can do nothing.
P_NB_IGNORES_BINDING = (NAT,
                        "\tif _, bound := processes[sighting.SessionID]; bound {\n\t\treturn false\n\t}",
                        "\tif _, bound := processes[sighting.SessionID]; false && bound {\n\t\treturn false\n\t}")
# Stale sightings held back as well: the window widened past anything that could
# be called newborn.
P_NB_WIDE_WINDOW = (NAT,
                    "\tnativeBindWindow  = 6 * time.Second",
                    "\tnativeBindWindow  = 6 * time.Hour")
# A terminal sighting held back, delaying the one piece of evidence that ends a
# node by a poll for no possible gain: nothing will ever bind a dead session.
P_NB_HOLDS_TERMINALS = (NAT,
                        "\tif sighting.Exit != \"\" {\n\t\treturn false\n\t}",
                        "\tif sighting.Exit != \"\" && false {\n\t\treturn false\n\t}")
# The prune dropped, so a node keeps its mark after it goes away and never gets
# the grace again when it comes back under a new incarnation.
P_NB_NO_PRUNE = (NAT,
                 "\tfor id := range c.awaitingBinding {\n\t\tif !seen[id] {\n\t\t\tdelete(c.awaitingBinding, id)\n\t\t}\n\t}",
                 "\t_ = seen")

# The claude scanner dating a sighting from something that is not its activity,
# so a two-hour-old session reads as a newborn and is held back forever.
P_NB_CLAUDE_UNDATED = (CLA,
                       "\t\t\tActivityAt: &sidecar.modTime,\n",
                       "")

# --- fix wave 2: the screenshot size hole ---------------------------------

# The width hardcoded. A frame of the right HEIGHT is the right height at any
# width, which is how this passed the whole suite before the assertion existed.
P_SHOT_FIXED_WIDTH = (MAI,
                      "\treturn ui.Render(s, th, w, h, now)",
                      "\treturn ui.Render(s, th, 120, h, now)")

# --- fix round 1: the first-tick signal -----------------------------------

# The signal closed on the way INTO the tick. A waiter then learns only that the
# collector is running, which it already knew, and samples the graph while the
# disk walk is still going.
P_TICK_ON_ENTRY = (NAT,
                   "\tdefer func() {\n\t\tc.firstTickOnce.Do(func() {",
                   "\tfunc() {\n\t\tc.firstTickOnce.Do(func() {")
# The signal withheld when the scan fails. A one-shot then pays its whole budget
# every time a runtime's home is unreadable.
P_TICK_NOT_ON_ERROR = (NAT,
                       "\tnodes, spawns, err := c.scan.scan(now, liveSessions(processes))\n\tif err != nil {\n\t\tc.scanErrors.Add(1)\n\t\treturn\n\t}",
                       "\tnodes, spawns, err := c.scan.scan(now, liveSessions(processes))\n\tif err != nil {\n\t\tc.scanErrors.Add(1)\n\t\tselect {}\n\t}")
# The signal re-closed every tick, which panics on the second one. The Once is
# what makes a waiter's answer independent of which tick it asked during.
P_TICK_NO_ONCE = (NAT,
                  "\t\tc.firstTickOnce.Do(func() {\n\t\t\t// Cleared before the signal, so nothing that reads the ids after\n\t\t\t// the close can see a later tick appending to them.\n\t\t\tc.firstTickPending = false\n\t\t\tclose(c.firstTick)\n\t\t})",
                  "\t\tfunc() {\n\t\t\tc.firstTickPending = false\n\t\t\tclose(c.firstTick)\n\t\t}()")
# Every attempted node recorded, not only the ones the store took. A waiter then
# looks for evidence that is never coming and burns its budget every run.
# Only an outright rejection excluded, so a saturation DROP is recorded as
# taken. Written this way rather than deleting the check, which would orphan the
# disposition binding and test the compiler instead of the rule.
P_TICK_NODES_ALL = (NAT,
                    "\t\tif c.firstTickPending && admittedToStore(disposition) {",
                    "\t\tif c.firstTickPending && disposition != graph.PublishRejected {")
# The list grows on every tick, so it names nodes from ticks nobody waits on.
P_TICK_NODES_FOREVER = (NAT,
                        "\t\t\tc.firstTickPending = false\n",
                        "")
# A dropped event counted as taken.
P_TICK_ADMIT_DROPS = (NAT,
                      "\tcase graph.PublishAcceptedNormal, graph.PublishAcceptedCritical,\n\t\tgraph.PublishDuplicate, graph.PublishCoalesced:\n\t\treturn true",
                      "\tcase graph.PublishAcceptedNormal, graph.PublishAcceptedCritical,\n\t\tgraph.PublishDuplicate, graph.PublishCoalesced,\n\t\tgraph.PublishDroppedNormal, graph.PublishDroppedCritical:\n\t\treturn true")

# --- fix round 1: the bounded wait ----------------------------------------

# The visibility half dropped: the one-shot waits for the lane to finish and
# samples immediately, which is the flake this replaced.
P_EV_NO_VISIBILITY = (ATT,
                      "\twaitNodesVisible(shadow, lanes[:ready], deadline)",
                      "\t_ = ready")
# A per-lane budget rather than a shared one, so three slow homes cost three
# bounds and the wait is not bounded by what the caller asked for.
P_EV_PER_LANE = (ATT,
                 "\t\tremaining := time.Until(deadline)",
                 "\t\tremaining := budget")
# The bound removed entirely: a pathological store hangs the command.
P_EV_UNBOUNDED = (ATT,
                  "\t\texpired := time.NewTimer(remaining)\n\t\tselect {\n\t\tcase <-lane.FirstTick():\n\t\t\tready++\n\t\tcase <-expired.C:\n\t\t\texpired.Stop()\n\t\t\treturn ready\n\t\t}\n\t\texpired.Stop()",
                  "\t\t<-lane.FirstTick()\n\t\tready++")
# Lanes that never came in consulted anyway, which is a read of ids a still
# running tick is writing.
P_EV_ALL_LANES = (ATT,
                  "\twaitNodesVisible(shadow, lanes[:ready], deadline)",
                  "\twaitNodesVisible(shadow, lanes, deadline)")
# The lane list dropped on the floor: a home registers a collector nobody waits
# on, which reads exactly like a fast lane.
P_ATT_NO_LANES = (ATT,
                  "\t\tlanes = append(lanes, lane)\n\t}\n\tif codexHome != \"\" {",
                  "\t}\n\tif codexHome != \"\" {")

# --- fix round 1: the production one-shot ---------------------------------

P_MAI_NO_WAIT = (MAI,
                 "\tlanes.WaitNativeEvidence(shadow, nativeFirstTickBudget)",
                 "\t_ = lanes")
# The budget widened to a minute. Both timing tests still pass: the fast lane
# lands and the slow one is still dropped by the enclosing context. Only the
# pinned constant sees it, and a one-shot that can take a minute is a hung
# command.
P_MAI_HUGE_BUDGET = (MAI,
                     "const nativeFirstTickBudget = 2 * time.Second",
                     "const nativeFirstTickBudget = 60 * time.Second")

# --- fix round 1: the portable ordering test ------------------------------

# The overlay injection removed, which is the pre-fix state: the assertion then
# depends on the box having an agent-shaped process running.
P_ORDER_NO_INJECT = (MAIT,
                     "\t\teng.Overlay = func() ([]types.Overlay, error) {\n\t\t\treturn []types.Overlay{{\n\t\t\t\tRuntime:     types.RuntimeLocal,\n\t\t\t\tSessionName: \"aitop-test-dark-unit\",\n\t\t\t\tStatus:      \"off\",\n\t\t\t}}, nil\n\t\t}\n",
                     "\t\teng.Overlay = func() ([]types.Overlay, error) { return nil, nil }\n")

# The dark-roster path removed, so a local unit with no pid never becomes a row
# and the ordering test's fixture has nothing to stand on.
P_JOIN_NO_DARK = (JOI,
                  '\t\tif o.Runtime == types.RuntimeLocal && o.SessionName != "" {',
                  "\t\tif false {")
# Every local overlay admitted to the dark roster, named or not, which puts an
# unnameable row in the graph's local-unit lane.
P_JOIN_DARK_ANY_LOCAL = (JOI,
                         '\t\tif o.Runtime == types.RuntimeLocal && o.SessionName != "" {',
                         "\t\tif o.Runtime == types.RuntimeLocal {")

# --- the horizon rule's live-process clause -------------------------------

# The clause removed. This is the code as Tasks 2-4 shipped it, and it loses a
# session whose process is up and whose sidecar has gone quiet.
P_CLA_NO_LIFT = (CLA,
                 "\t\tif now.Sub(sidecar.modTime) > nativeHorizon && !liveProcs[sidecar.SessionID] {",
                 "\t\tif now.Sub(sidecar.modTime) > nativeHorizon {")
# The bound removed instead of lifted: every cold sidecar on the box is admitted.
P_CLA_NO_HORIZON = (CLA,
                    "\t\tif now.Sub(sidecar.modTime) > nativeHorizon && !liveProcs[sidecar.SessionID] {",
                    "\t\tif false {")
# The lift keyed on the roster's own map, which answers a different question:
# it says which sidecar FILE owns a session id, never whether a process holds it,
# and it is non-empty for every session on disk.
P_CLA_WRONG_MAP = (CLA,
                   "\t\tif now.Sub(sidecar.modTime) > nativeHorizon && !liveProcs[sidecar.SessionID] {",
                   "\t\tif now.Sub(sidecar.modTime) > nativeHorizon && live[sidecar.SessionID] == nil {")

P_COD_NO_LIFT = (COD,
                 "\t\t\tif now.Sub(info.ModTime()) > nativeHorizon && !codexLiveRollout(name, live) {",
                 "\t\t\tif now.Sub(info.ModTime()) > nativeHorizon {")
# The bound widened 100x rather than removed: `if false` would orphan the info
# binding and fail to build, which tests the compiler rather than the rule.
P_COD_NO_HORIZON = (COD,
                    "\t\t\tif now.Sub(info.ModTime()) > nativeHorizon && !codexLiveRollout(name, live) {",
                    "\t\t\tif now.Sub(info.ModTime()) > 100*nativeHorizon {")
# The suffix match dropped: any live thread admits every stale rollout in the
# walked days, which is the whole corpus for those dates.
# The suffix test replaced by a check that the name exists at all: any live
# thread then admits every stale rollout in the walked days, which is the whole
# corpus for those dates. The base binding is kept so the plant compiles and the
# cycle measures the rule rather than the compiler.
P_COD_NO_MATCH = (COD,
                  '\t\tif thread != "" && strings.HasSuffix(base, "-"+thread) {',
                  '\t\tif thread != "" && len(base) > 0 {')
# Match the id anywhere in the name rather than at the end. A rollout's name
# carries a timestamp too, so this is a looser rule that still looks right.
P_COD_CONTAINS = (COD,
                  '\t\tif thread != "" && strings.HasSuffix(base, "-"+thread) {',
                  '\t\tif thread != "" && strings.Contains(base, thread) {')

# The lift on the recorded horizon only. The stat prefilter then rejects the
# live session before its own claim is ever read.
P_GRK_NO_STAT_LIFT = (GRK,
                      "\t\tif now.Sub(info.ModTime()) > nativeHorizon && !live[visit.session] {",
                      "\t\tif now.Sub(info.ModTime()) > nativeHorizon {")
# The lift on the prefilter only. The runtime's own last_active_at then rejects it.
P_GRK_NO_RECORDED_LIFT = (GRK,
                          "\tif now.Sub(*activity) > nativeHorizon && !live[visit.session] {",
                          "\tif now.Sub(*activity) > nativeHorizon {")
# Both grok bounds widened. ONE of them is not enough to admit a cold session:
# the stat prefilter and the runtime's own last_active_at each independently
# reject it, so a plant that removes either alone leaves the absence assertion
# correctly green. Measured, not predicted -- the first attempt aimed at the
# recorded horizon alone and went green.
P_GRK_NO_HORIZON = (GRK,
                    "\tif now.Sub(*activity) > nativeHorizon && !live[visit.session] {",
                    "\tif now.Sub(*activity) > 100*nativeHorizon && !live[visit.session] {")
P_GRK_WIDE_STAT = (GRK,
                   "\t\tif now.Sub(info.ModTime()) > nativeHorizon && !live[visit.session] {",
                   "\t\tif now.Sub(info.ModTime()) > 100*nativeHorizon && !live[visit.session] {")

# --- the fork sidecar's runtime -------------------------------------------

P_SID_NO_RUNTIME = (SID, "\t\tRuntime:      in.Target.Runtime,\n", "")
# The runtime written as a constant. Every fork this box has ever taken was of a
# claude row, so this passes the presence test and lies on every other runtime.
P_SID_CONST = (SID,
               "\t\tRuntime:      in.Target.Runtime,",
               "\t\tRuntime:      types.RuntimeClaude,")
# The child's runtime read off the capsule rather than the forked row.
P_SID_FROM_CAPSULE = (SID,
                      "\t\tRuntime:      in.Target.Runtime,",
                      "\t\tRuntime:      types.Runtime(cap.Kind),")

P_FRK_NO_RUNTIME = (FRK, "\t\t\tRuntime:       f.Runtime,\n", "")
# The runtime read from a neighbouring field that is a fork KIND, not a runtime.
P_FRK_FROM_KIND = (FRK,
                   "\t\t\tRuntime:       f.Runtime,",
                   "\t\t\tRuntime:       types.Runtime(f.Kind),")

# --- the second gate: the forked child's role ------------------------------

# The fix as the brief described it: the runtime written and read, and nothing
# else. The child still never reaches the graph, because occupancyRole drops a
# row whose Process.Role is the zero value on the same line that drops an
# unknown runtime. This cycle is the reason the regression proof drives the
# whole chain instead of a hand-built row.
P_JOIN_NO_ROLE = (JOI,
                  "\t\t\t\tif c.ForkOf != \"\" {",
                  "\t\t\t\tif false {")
# Role every Overlay-only child. Inference slots and any future orphan shape are
# then enrolled into the graph by a rule written for forks.
P_JOIN_ROLE_ALL = (JOI,
                   "\t\t\t\tif c.ForkOf != \"\" {",
                   "\t\t\t\tif true {")
# A forked child filed as a primary. It is somebody's child by construction.
P_JOIN_ROLE_PRIMARY = (JOI,
                       "\t\t\t\t\tcr.Process.Role = types.RoleSubagent",
                       "\t\t\t\t\tcr.Process.Role = types.RolePrimary")
# A synthetic process invented alongside the role, which is the natural next
# step for anyone filling in a Process struct and puts a pid of 0 into every
# consumer that reads the spine.
# The child given its parent's process, which is the natural way to "attach"
# a child that has none and would hand it the parent's occupancy identity.
# The first attempt copied c.PID instead, and went green: a fork overlay's PID
# is always 0, so the plant wrote the value the assertion already expected.
P_JOIN_FAKE_PROC = (JOI,
                    "\t\t\t\t\tcr.Process.Role = types.RoleSubagent",
                    "\t\t\t\t\tcr.Process = r.Process\n\t\t\t\t\tcr.Process.Role = types.RoleSubagent")

# --- the attach ------------------------------------------------------------

P_ATT_NO_CLAUDE = (ATT, '\tif claudeHome != "" {', "\tif false {")
P_ATT_NO_APPEND = (ATT,
                   "\t\tcollectors = append(collectors, lane)\n\t\tlanes = append(lanes, lane)\n\t}\n\tif codexHome != \"\" {",
                   "\t\tlanes = append(lanes, lane)\n\t}\n\tif codexHome != \"\" {")
# The wrapper delegating with the engine's own homes, which is the refactor that
# looks like tidying up and gives every existing caller three collectors.
P_ATT_WRAPPER_HOMES = (ATT,
                       "\tshadow, occ, _, err := AttachGraph(eng, GraphHomes{})",
                       "\tshadow, occ, _, err := AttachGraph(eng, GraphHomes{Claude: eng.ClaudeHome, Codex: eng.CodexHome, Grok: eng.GrokHome})")
# The native collectors handed no rows. Every scanner test passes latest=nil, so
# this is exactly the state Task 3's review flagged: it looks harmless and it
# costs every native node its process incarnation.
P_ATT_NIL_LATEST = (ATT,
                    "\t\tlane := native.NewClaude(claudeHome, latest)",
                    "\t\tlane := native.NewClaude(claudeHome, nil)")
# Homes used as given. A relative home makes a node's identity a function of the
# process's working directory, because Location joins the record digest.
# The disable switch dropped: filepath.Abs("") returns the working directory, so
# an unconfigured runtime registers a collector on whatever repo the operator
# happens to be standing in.
P_ATT_ABS_EMPTY = (ATT,
                   '\tif home == "" {\n\t\treturn "", nil\n\t}\n',
                   "")

# Cleaned rather than resolved. Clean tidies a path and leaves a relative one
# relative, which is the edit that looks like normalisation and keeps the
# filepath import in use, so the cycle measures the rule rather than the build.
P_ATT_NO_ABS = (ATT,
                "\tabs, err := filepath.Abs(home)\n\tif err != nil {\n\t\treturn \"\", fmt.Errorf(\"snapshot graph attach home rule violated: home=%s err=%w\", home, err)\n\t}\n\treturn abs, nil",
                "\treturn filepath.Clean(home), nil")

# --- schema 2 and the unclaimed state --------------------------------------

P_JSN_NO_MAP = (JSN, "\t\tvalue = graph.StateUnknown\n", "")
# The pair completed along with the value: a since stamped alongside it, which
# asserts that somebody determined this state at a moment when nobody did.
P_JSN_INVENTS_SINCE = (JSN,
                       "\t\tvalue = graph.StateUnknown\n\t}",
                       "\t\tvalue = graph.StateUnknown\n\t\tst.Since = time.Now().UTC()\n\t}")
# Every state rewritten, not only the unclaimed one.
P_JSN_ALWAYS_UNKNOWN = (JSN, '\tif value == "" {', "\tif true {")

# --- the production run paths ----------------------------------------------

P_MAI_CAP_EMPTY = (MAI,
                   "\tshadow, occ, lanes, err := attachGraph(eng, productionGraphHomes(eng))",
                   "\tshadow, occ, lanes, err := attachGraph(eng, snapshot.GraphHomes{})")
P_MAI_INT_EMPTY = (MAI,
                   "\tshadow, _, _, err := attachGraph(eng, productionGraphHomes(eng))",
                   "\tshadow, _, _, err := attachGraph(eng, snapshot.GraphHomes{})")
# One home missed in the helper both paths share. This is the lapse the helper
# exists to make impossible to make twice, so it must red BOTH paths.
P_MAI_DROP_CODEX = (MAI,
                    "\treturn snapshot.GraphHomes{Claude: eng.ClaudeHome, Codex: eng.CodexHome, Grok: eng.GrokHome}",
                    "\treturn snapshot.GraphHomes{Claude: eng.ClaudeHome, Grok: eng.GrokHome}")
# The engine's grok home wired to its claude home: both non-empty, both wrong.
P_MAI_SWAP = (MAI,
              "\treturn snapshot.GraphHomes{Claude: eng.ClaudeHome, Codex: eng.CodexHome, Grok: eng.GrokHome}",
              "\treturn snapshot.GraphHomes{Claude: eng.ClaudeHome, Codex: eng.CodexHome, Grok: eng.ClaudeHome}")

# The ordering as it was before this task: collectors first, rows second.
P_MAI_OLD_ORDER = (MAI,
                   "\tsnap, err := eng.CaptureOnce(ctx)\n\tif err != nil {\n\t\treturn snap, err\n\t}\n\tdone := make(chan error, 1)\n\tgo func() { done <- shadow.Run(runCtx) }()\n\tocc.Notify()",
                   "\tdone := make(chan error, 1)\n\tgo func() { done <- shadow.Run(runCtx) }()\n\tsnap, err := eng.CaptureOnce(ctx)\n\tif err != nil {\n\t\treturn snap, err\n\t}\n\tocc.Notify()")

# --- weakenings ------------------------------------------------------------

W_CLA_PRESENT = (CLAT,
                 "\tnode, ok := nodeByID(nodes, id)\n\tif !ok {\n\t\tt.Fatalf(\"claude-live-session-is-inside-horizon rule violated:",
                 "\tnode, ok := nodeByID(nodes, id)\n\tif false && !ok {\n\t\tt.Fatalf(\"claude-live-session-is-inside-horizon rule violated:")
W_CLA_CONTENT = (CLAT,
                 "\tif node.Name != \"aegis-48\" || node.Project != \"aitop\" || node.Location != path || node.Role != types.RolePrimary {",
                 "\tif false && (node.Name != \"aegis-48\" || node.Project != \"aitop\" || node.Location != path || node.Role != types.RolePrimary) {")
W_CLA_NOCLAIM = (CLAT,
                 "\tif node.State != \"\" || node.Exit != \"\" {\n\t\tt.Fatalf(\"claude-live-session-makes-no-state-or-terminal-claim",
                 "\tif false {\n\t\tt.Fatalf(\"claude-live-session-makes-no-state-or-terminal-claim")
W_CLA_ABSENT = (CLAT,
                "\tif _, ok := nodeByID(nodes, id); ok {\n\t\tt.Fatalf(\"claude-cold-session-with-no-process-is-not-observed",
                "\tif false {\n\t\tt.Fatalf(\"claude-cold-session-with-no-process-is-not-observed")

W_COD_PRESENT = (CODT,
                 "\tnode, ok := nodeByID(nodes, liveID)\n\tif !ok {",
                 "\tnode, ok := nodeByID(nodes, liveID)\n\tif false && !ok {")
W_COD_CONTENT = (CODT,
                 "\tif node.Location != livePath || node.Project != \"aitop\" || node.Role != types.RolePrimary {",
                 "\tif false && (node.Location != livePath || node.Project != \"aitop\" || node.Role != types.RolePrimary) {")
W_COD_ABSENT = (CODT,
                "\tif _, ok := nodeByID(nodes, liveID); ok {\n\t\tt.Fatalf(\"codex-cold-thread-with-no-process-is-not-observed",
                "\tif false {\n\t\tt.Fatalf(\"codex-cold-thread-with-no-process-is-not-observed")
W_COD_SELECT = (CODT,
                "\tif _, ok := nodeByID(nodes, deadID); ok {",
                "\tif _, ok := nodeByID(nodes, deadID); false && ok {")
W_COD_SKIP = (CODT,
              "\tif n := scanner.skippedRollouts.Load(); n != 0 {",
              "\tif n := scanner.skippedRollouts.Load(); false {")

W_GRK_PRESENT = (GRKT,
                 "\tnode, ok := nodeByID(nodes, id)\n\tif !ok {\n\t\tt.Fatalf(\"grok-live-session-is-inside-horizon",
                 "\tnode, ok := nodeByID(nodes, id)\n\tif false && !ok {\n\t\tt.Fatalf(\"grok-live-session-is-inside-horizon")
W_GRK_CONTENT = (GRKT,
                 "\tif node.Name != grokHarnessName || node.Model != grokModel || node.Project != \"aitop\" || node.TaskName != \"wire the collectors\" || node.Location != path {",
                 "\tif false && (node.Name != grokHarnessName || node.Model != grokModel || node.Project != \"aitop\" || node.TaskName != \"wire the collectors\" || node.Location != path) {")
W_GRK_STARTED = (GRKT,
                 "\tif node.StartedAt == nil || !node.StartedAt.Equal(created) {",
                 "\tif false && (node.StartedAt == nil || !node.StartedAt.Equal(created)) {")
W_GRK_NOCLAIM = (GRKT,
                 "\tif node.State != \"\" || node.Exit != \"\" {\n\t\tt.Fatalf(\"grok-live-session-makes-no-state-or-terminal-claim",
                 "\tif false {\n\t\tt.Fatalf(\"grok-live-session-makes-no-state-or-terminal-claim")
W_GRK_SKIP = (GRKT,
              "\tif n := scanner.skippedSummaries.Load(); n != 0 {",
              "\tif n := scanner.skippedSummaries.Load(); false {")
W_GRK_ABSENT = (GRKT,
                "\tif _, ok := nodeByID(nodes, id); ok {\n\t\tt.Fatalf(\"grok-cold-session-with-no-process-is-not-observed",
                "\tif false {\n\t\tt.Fatalf(\"grok-cold-session-with-no-process-is-not-observed")

W_SID_RT = (SIDT,
            '\tif got := decoded["runtime"]; got != string(types.RuntimeClaude) {',
            '\tif got := decoded["runtime"]; false {')
W_SID_FOLLOW = (SIDT,
                "\t\tif got := decoded[\"runtime\"]; got != string(runtime) {",
                "\t\tif got := decoded[\"runtime\"]; false {")

W_FRK_OVERLAY = (FRKT,
                 "\tif ovs[0].Runtime != types.RuntimeClaude {",
                 "\tif false && (ovs[0].Runtime != types.RuntimeClaude) {")
W_FRK_ROW = (FRKT,
             "\tif child.Runtime != types.RuntimeClaude {",
             "\tif false && (child.Runtime != types.RuntimeClaude) {")
W_FRK_GRAPH = (FRKT,
               "\tif !found {\n\t\tt.Fatalf(\"fork-child-reaches-the-graph",
               "\tif false && !found {\n\t\tt.Fatalf(\"fork-child-reaches-the-graph")

W_JOIN_ROLE = (JOIT,
               "\tif got := byID[\"C\"].Process.Role; got != types.RoleSubagent {",
               "\tif got := byID[\"C\"].Process.Role; false {")
W_JOIN_SLOT = (JOIT,
               "\tif got := byID[\"S\"].Process.Role; got != types.RoleDrop {",
               "\tif got := byID[\"S\"].Process.Role; false {")
W_JOIN_PROC = (JOIT,
               "\tif byID[\"C\"].Process.PID != 0 || byID[\"C\"].Process.StartTime != 0 || !byID[\"C\"].OverlayOnly {",
               "\tif false && (byID[\"C\"].Process.PID != 0 || byID[\"C\"].Process.StartTime != 0 || !byID[\"C\"].OverlayOnly) {")

W_ATT_REG = (ATTT,
             "\tif !hasPrefix(snap, \"claude:session:\") {\n\t\tt.Fatalf(\"attach-graph-registers-native-collectors",
             "\tif false {\n\t\tt.Fatalf(\"attach-graph-registers-native-collectors")
W_ATT_EMPTY = (ATTT,
               "\t\tif snap := shadow.Snapshot(); hasPrefix(snap, \"claude:session:\") {",
               "\t\tif snap := shadow.Snapshot(); false && hasPrefix(snap, \"claude:session:\") {")
W_ATT_AGENT = (ATTT,
               "\tagent, ok := nodeByGraphID(snap, agentID)\n\tif !ok {",
               "\tagent, ok := nodeByGraphID(snap, agentID)\n\tif false && !ok {")
W_ATT_AGENT_CONTENT = (ATTT,
                       "\tif agent.Role != types.RoleSubagent || agent.TaskName != \"wire the collectors\" {",
                       "\tif false && (agent.Role != types.RoleSubagent || agent.TaskName != \"wire the collectors\") {")
W_ATT_INC = (ATTT,
             "\tif session.Incarnation != wantIncarnation {",
             "\tif false && (session.Incarnation != wantIncarnation) {")
W_ATT_PROC = (ATTT,
              "\tif session.Process == nil || session.Process.PID != attachPID || session.Process.StartTicks != attachStartTick {",
              "\tif false && (session.Process == nil || session.Process.PID != attachPID || session.Process.StartTicks != attachStartTick) {")
W_ATT_EDGE = (ATTT,
              "\tif edge == nil {\n\t\tt.Fatalf(\"attach-graph-native-spawn-edge-survives-admission",
              "\tif edge == nil {\n\t\treturn\n\t}\n\tif false {\n\t\tt.Fatalf(\"attach-graph-native-spawn-edge-survives-admission")
W_ATT_SESSION = (ATTT,
                 "\tsession, ok := nodeByGraphID(snap, sessionID)\n\tif !ok {",
                 "\tsession, ok := nodeByGraphID(snap, sessionID)\n\tif false && !ok {")

W_ATT_ABS = (ATTT,
             "\tif !filepath.IsAbs(got) {",
             "\tif false && (!filepath.IsAbs(got)) {")
W_ATT_ABS_EMPTY = (ATTT,
                   '\tif got, err := absHome(""); err != nil || got != "" {',
                   '\tif got, err := absHome(""); false {')

W_SCHEMA_WRITE = (ATTT,
                  "\tif err := WriteJSON(snap, &out, snap.At); err != nil {",
                  "\tif err := WriteJSON(snap, &out, snap.At); false && err != nil {")
# Removing the mapping makes WriteJSON fail, so nothing is written and the
# decode fails on empty input. The write assertion and the decode assertion are
# two witnesses of one rule, and a weakened cycle has to relax both.
W_NB_TICK1 = (NATT,
              "\tif n := sink.count(); n != 0 {\n\t\tevents, _ := sink.snapshot()",
              "\tif n := sink.count(); false && n != 0 {\n\t\tevents, _ := sink.snapshot()")
W_NB_INCARNATION = (NATT,
                    "\tif events[0].Actor != id || events[0].ActorIncarnation != want {",
                    "\tif false && (events[0].Actor != id || events[0].ActorIncarnation != want) {")
W_NB_NEVER_INV = (NATT,
                  "\t\tif e.ActorIncarnation == never {",
                  "\t\tif false && e.ActorIncarnation == never {")
# The every-tick plant trips both witnesses: the node is never published at all,
# so "waited for only once" reds AND "stays published" reds. Found by running
# the weakened cycle after relaxing the first.
W_NB_STAYS = (NATT,
              "\tif sink.count() <= before {",
              "\tif false && sink.count() <= before {")
W_NB_ONCE = (NATT,
             "\tif n := sink.count(); n == 0 {\n\t\tt.Fatalf(\"newborn-sighting-is-waited-for-only-once",
             "\tif false {\n\t\tt.Fatalf(\"newborn-sighting-is-waited-for-only-once")
W_NB_ONCE_TICK1 = (NATT,
                   "\tif n := sink.count(); n != 0 {\n\t\tt.Fatalf(\"newborn-sighting-waits-for-its-binding rule violated: tick 1 published %d events\", n)",
                   "\tif n := sink.count(); false && n != 0 {\n\t\tt.Fatalf(\"newborn-sighting-waits-for-its-binding rule violated: tick 1 published %d events\", n)")
W_NB_STALE = (NATT,
              "\tif n := sink.count(); n == 0 {\n\t\tt.Fatalf(\"stale-unbound-sighting-publishes-immediately",
              "\tif false {\n\t\tt.Fatalf(\"stale-unbound-sighting-publishes-immediately")
# Short-circuits rather than merely disabling: the assertions after this one
# index events[0], so a weakening that only silences the guard panics on an
# empty slice and reports a crash instead of a green.
W_NB_BOUND = (NATT,
              "\tif len(events) == 0 {\n\t\tt.Fatalf(\"bound-sighting-publishes-immediately",
              "\tif len(events) == 0 {\n\t\treturn\n\t}\n\tif false {\n\t\tt.Fatalf(\"bound-sighting-publishes-immediately")
# Four tabs, and anchored on the graph pane's own message. The table subtest's
# identical check sits one level shallower, so the shallower text is a SUBSTRING
# of this one and a three-tab patch would match both.
W_SHOT_WIDTH = (MAIT,
                "\t\t\t\tif w := ansi.StringWidth(l); w != graphFrameW {\n\t\t\t\t\tt.Fatalf(\"screenshot-graph-renders-the-requested-size",
                "\t\t\t\tif w := ansi.StringWidth(l); false && w != graphFrameW {\n\t\t\t\t\tt.Fatalf(\"screenshot-graph-renders-the-requested-size")
W_SHOT_WIDTH_TABLE = (MAIT,
                      "\t\t\tif w := ansi.StringWidth(l); w != graphFrameW {\n\t\t\t\tt.Fatalf(\"screenshot-renders-the-requested-size",
                      "\t\t\tif w := ansi.StringWidth(l); false && w != graphFrameW {\n\t\t\t\tt.Fatalf(\"screenshot-renders-the-requested-size")

W_TICK_AFTER_OPEN = (NATT,
                     "\t\tt.Fatalf(\"native-first-tick-is-open-before-the-collector-runs rule violated: signal closed with no tick taken\")",
                     "\t\t_ = t")
W_TICK_AFTER_MID = (NATT,
                    "\t\tt.Fatalf(\"native-first-tick-closes-only-when-the-tick-finishes rule violated: signal closed while scan was still running\")",
                    "\t\t_ = t")
# The mid-scan assertion is not the only witness: a signal that closes on entry
# also closes before anything is published, and the publishing assertion catches
# it independently. Two witnesses of one rule, both relaxed here.
# Anchored on its own message: three tests now assert on sink.count(), and a
# patch that could match any of them is a patch aimed at none.
W_TICK_AFTER_PUBLISHED = (NATT,
                          "\tif n := sink.count(); n == 0 {\n\t\tt.Fatalf(\"native-first-tick-closes-after-publishing",
                          "\tif n := sink.count(); false && n == 0 {\n\t\tt.Fatalf(\"native-first-tick-closes-after-publishing")

W_TICK_ERR = (NATT,
              "\t\tt.Fatalf(\"native-first-tick-closes-on-a-failed-scan rule violated: signal still open 3s after a scan error\")",
              "\t\t_ = t")
W_TICK_STAYS = (NATT,
                "\t\t\tt.Fatalf(\"native-first-tick-stays-closed rule violated: signal reopened after tick %d\", scanner.callCount())",
                "\t\t\t_ = scanner")
W_TICK_NODES = (NATT,
                "\tif len(got) != 2 || got[0] != parent || got[1] != second {",
                "\tif false && (len(got) != 2 || got[0] != parent || got[1] != second) {")
W_TICK_NODES_DROP = (NATT,
                     "\tif got := dropped.FirstTickNodes(); len(got) != 0 {",
                     "\tif got := dropped.FirstTickNodes(); false && len(got) != 0 {")
W_TICK_NODES_ONE = (NATT,
                    "\tif got := len(collector.FirstTickNodes()); got != first {",
                    "\tif got := len(collector.FirstTickNodes()); false && got != first {")

W_EV_COUNT = (ATTT,
              "\tif ready := lanes.WaitNativeEvidence(nil, 2*time.Second); ready != 3 {",
              "\tif ready := lanes.WaitNativeEvidence(nil, 2*time.Second); false && ready != 3 {")
W_EV_FAST = (ATTT,
             "\tif elapsed := time.Since(started); elapsed > time.Second {\n\t\tt.Fatalf(\"wait-native-evidence-returns-on-the-lanes-not-the-budget",
             "\tif false {\n\t\tt.Fatalf(\"wait-native-evidence-returns-on-the-lanes-not-the-budget")
W_EV_BOUND = (ATTT,
              "\tif elapsed > 2*time.Second {\n\t\tt.Fatalf(\"wait-native-evidence-is-bounded",
              "\tif false {\n\t\tt.Fatalf(\"wait-native-evidence-is-bounded")
# The per-lane plant trips BOTH witnesses of the sharing rule: the third lane is
# consulted, and two lanes come in where one fits. A weakened cycle has to relax
# both or it reds on the one it did not name.
W_EV_SHARED = (ATTT,
               "\tif third.consulted() {",
               "\tif false && third.consulted() {")
W_EV_SHARED_COUNT = (ATTT,
                     "\tif ready > 1 {",
                     "\tif false && ready > 1 {")
# The third witness, and it is named for a different rule on purpose: a budget
# per lane is also a wait not bounded by the number the caller supplied, so the
# boundedness assertion catches it too. Found by running the weakened cycle,
# which reddened on this one after the other two were relaxed.
W_EV_SHARED_ELAPSED = (ATTT,
                       "\tif elapsed > 2*budget {",
                       "\tif false && elapsed > 2*budget {")
W_DARK_ROWS = (JOIT,
               "\tif len(rows) != 1 {\n\t\tt.Fatalf(\"dark-local-unit-needs-no-spine",
               "\tif len(rows) != 1 {\n\t\treturn\n\t}\n\tif false {\n\t\tt.Fatalf(\"dark-local-unit-needs-no-spine")
W_DARK_UNNAMED = (JOIT,
                  "\tif rows := Join(nil, []types.Overlay{{Runtime: types.RuntimeLocal}}); len(rows) != 0 {",
                  "\tif rows := Join(nil, []types.Overlay{{Runtime: types.RuntimeLocal}}); false && len(rows) != 0 {")

W_LANES_COUNT = (ATTT,
                 "\t\tif len(lanes) != tc.want {",
                 "\t\tif false && len(lanes) != tc.want {")

W_CAP_UNDER = (MAIT,
               "\tif !graphHasNode(snap, id) {",
               "\tif false && !graphHasNode(snap, id) {")
W_CAP_OVER = (MAIT,
              "\tif elapsed > 15*time.Second {",
              "\tif false && elapsed > 15*time.Second {")
W_BUDGET = (MAIT,
            "\tif nativeFirstTickBudget != 2*time.Second {",
            "\tif false && nativeFirstTickBudget != 2*time.Second {")
W_ORDER_CANARY = (MAIT,
                  "\tif rowsAfter == 0 {",
                  "\tif false && rowsAfter == 0 {")

W_SCHEMA_JSON = (ATTT,
                 "\tif err := json.Unmarshal([]byte(out.String()), &decoded); err != nil {",
                 "\tif err := json.Unmarshal([]byte(out.String()), &decoded); false && err != nil {")

W_SCHEMA_VALUE = (ATTT,
                  "\tif state.Value != string(graph.StateUnknown) {",
                  "\tif false && (state.Value != string(graph.StateUnknown)) {")
W_SCHEMA_NODES = (ATTT,
                  "\tif len(decoded.Graph.Nodes) != 1 {",
                  "\tif len(decoded.Graph.Nodes) != 1 {\n\t\treturn\n\t}\n\tif false {")
W_SCHEMA_SOURCE = (ATTT,
                   '\tif len(state.Source) != 0 && string(state.Source) != "null" {',
                   '\tif false && (len(state.Source) != 0 && string(state.Source) != "null") {')
W_SCHEMA_SINCE = (ATTT,
                  '\tif state.Since != "" {',
                  '\tif false && (state.Since != "") {')

W_MAIN_HOMES = (MAIT,
                "\tif spy.homes != want {",
                "\tif false && (spy.homes != want) {")
W_MAIN_NONEMPTY = (MAIT,
                   '\tif eng.ClaudeHome == "" || eng.CodexHome == "" || eng.GrokHome == "" {',
                   "\tif false && (eng.ClaudeHome == "" || eng.CodexHome == "" || eng.GrokHome == "") {")
W_MAIN_ENVPREFIX = (MAIT,
                    "\tif !strings.HasPrefix(spy.homes.Claude, home) {",
                    "\tif false && (!strings.HasPrefix(spy.homes.Claude, home)) {")
W_MAIN_ORDER = (MAIT,
                "\tif !probe.sawRows.Load() {",
                "\tif false && (!probe.sawRows.Load()) {")

CYCLES = [
    # --- the horizon rule's live-process clause ---------------------------
    ("S-T5-01-prod", [P_CLA_NO_LIFT], P_NATIVE, T_CLA_LIVE, "RED", "claude-live-session-is-inside-horizon rule violated"),
    ("S-T5-01-weak", [P_CLA_NO_LIFT, W_CLA_PRESENT, W_CLA_CONTENT, W_CLA_NOCLAIM], P_NATIVE, T_CLA_LIVE, "GREEN", None),
    ("S-T5-02-prod", [P_CLA_NO_HORIZON], P_NATIVE, T_CLA_LIVE, "RED", "claude-cold-session-with-no-process-is-not-observed rule violated"),
    ("S-T5-02-weak", [P_CLA_NO_HORIZON, W_CLA_ABSENT], P_NATIVE, T_CLA_LIVE, "GREEN", None),
    ("S-T5-03-prod", [P_CLA_WRONG_MAP], P_NATIVE, T_CLA_LIVE, "RED", "claude-cold-session-with-no-process-is-not-observed rule violated"),
    ("S-T5-03-weak", [P_CLA_WRONG_MAP, W_CLA_ABSENT], P_NATIVE, T_CLA_LIVE, "GREEN", None),

    ("S-T5-04-prod", [P_COD_NO_LIFT], P_NATIVE, T_COD_LIVE, "RED", "codex-live-thread-is-inside-horizon rule violated"),
    ("S-T5-04-weak", [P_COD_NO_LIFT, W_COD_PRESENT, W_COD_CONTENT, W_COD_SKIP], P_NATIVE, T_COD_LIVE, "GREEN", None),
    ("S-T5-05-prod", [P_COD_NO_HORIZON], P_NATIVE, T_COD_LIVE, "RED", "codex-cold-thread-with-no-process-is-not-observed rule violated"),
    ("S-T5-05-weak", [P_COD_NO_HORIZON, W_COD_ABSENT, W_COD_SELECT], P_NATIVE, T_COD_LIVE, "GREEN", None),
    ("S-T5-06-prod", [P_COD_NO_MATCH], P_NATIVE, T_COD_LIVE, "RED", "codex-live-lift-admits-only-the-live-thread rule violated"),
    ("S-T5-06-weak", [P_COD_NO_MATCH, W_COD_SELECT], P_NATIVE, T_COD_LIVE, "GREEN", None),
    ("S-T5-07-prod", [P_COD_CONTAINS], P_NATIVE, T_COD_LIVE, "GREEN", None),

    ("S-T5-08-prod", [P_GRK_NO_STAT_LIFT], P_NATIVE, T_GRK_LIVE, "RED", "grok-live-session-is-inside-horizon rule violated"),
    ("S-T5-08-weak", [P_GRK_NO_STAT_LIFT, W_GRK_PRESENT, W_GRK_CONTENT, W_GRK_STARTED, W_GRK_NOCLAIM, W_GRK_SKIP], P_NATIVE, T_GRK_LIVE, "GREEN", None),
    ("S-T5-09-prod", [P_GRK_NO_RECORDED_LIFT], P_NATIVE, T_GRK_LIVE, "RED", "grok-live-session-is-inside-horizon rule violated"),
    ("S-T5-09-weak", [P_GRK_NO_RECORDED_LIFT, W_GRK_PRESENT, W_GRK_CONTENT, W_GRK_STARTED, W_GRK_NOCLAIM, W_GRK_SKIP], P_NATIVE, T_GRK_LIVE, "GREEN", None),
    ("S-T5-10a-prod", [P_GRK_NO_HORIZON], P_NATIVE, T_GRK_LIVE, "GREEN", None),
    ("S-T5-10b-prod", [P_GRK_WIDE_STAT], P_NATIVE, T_GRK_LIVE, "GREEN", None),
    ("S-T5-10-prod", [P_GRK_NO_HORIZON, P_GRK_WIDE_STAT], P_NATIVE, T_GRK_LIVE, "RED", "grok-cold-session-with-no-process-is-not-observed rule violated"),
    ("S-T5-10-weak", [P_GRK_NO_HORIZON, P_GRK_WIDE_STAT, W_GRK_ABSENT], P_NATIVE, T_GRK_LIVE, "GREEN", None),

    # --- the fork sidecar's runtime ---------------------------------------
    ("S-T5-11-prod", [P_SID_NO_RUNTIME], P_ACT, T_SID_RT, "RED", "fork-sidecar-carries-target-runtime rule violated"),
    ("S-T5-11-weak", [P_SID_NO_RUNTIME, W_SID_RT], P_ACT, T_SID_RT, "GREEN", None),
    ("S-T5-12-prod", [P_SID_CONST], P_ACT, T_SID_RT, "GREEN", None),
    ("S-T5-12b-prod", [P_SID_CONST], P_ACT, T_SID_FOLLOW, "RED", "fork-sidecar-runtime-follows-the-target rule violated"),
    ("S-T5-12b-weak", [P_SID_CONST, W_SID_FOLLOW], P_ACT, T_SID_FOLLOW, "GREEN", None),
    ("S-T5-13-prod", [P_SID_FROM_CAPSULE], P_ACT, T_SID_RT, "RED", "fork-sidecar-carries-target-runtime rule violated"),
    ("S-T5-13-weak", [P_SID_FROM_CAPSULE, W_SID_RT], P_ACT, T_SID_RT, "GREEN", None),

    ("S-T5-14-prod", [P_FRK_NO_RUNTIME], P_FORKS, T_FRK, "RED", "fork-overlay-carries-the-sidecar-runtime rule violated"),
    ("S-T5-14-weak", [P_FRK_NO_RUNTIME, W_FRK_OVERLAY, W_FRK_ROW, W_FRK_GRAPH], P_FORKS, T_FRK, "GREEN", None),
    ("S-T5-15-prod", [P_FRK_FROM_KIND], P_FORKS, T_FRK, "RED", "fork-overlay-carries-the-sidecar-runtime rule violated"),
    ("S-T5-15-weak", [P_FRK_FROM_KIND, W_FRK_OVERLAY, W_FRK_ROW, W_FRK_GRAPH], P_FORKS, T_FRK, "GREEN", None),

    # --- the second gate: the role. S-T5-16 is the brief's fix on its own ---
    ("S-T5-16-prod", [P_JOIN_NO_ROLE], P_FORKS, T_FRK, "RED", "fork-child-reaches-the-graph rule violated"),
    ("S-T5-16-weak", [P_JOIN_NO_ROLE, W_FRK_GRAPH], P_FORKS, T_FRK, "GREEN", None),
    ("S-T5-17-prod", [P_JOIN_NO_ROLE], P_JOIN, T_JOIN_ROLE, "RED", "fork-child-row-carries-subagent-role rule violated"),
    ("S-T5-17-weak", [P_JOIN_NO_ROLE, W_JOIN_ROLE], P_JOIN, T_JOIN_ROLE, "GREEN", None),
    ("S-T5-18-prod", [P_JOIN_ROLE_ALL], P_JOIN, T_JOIN_ROLE, "RED", "non-fork-orphan-keeps-its-empty-role rule violated"),
    ("S-T5-18-weak", [P_JOIN_ROLE_ALL, W_JOIN_SLOT], P_JOIN, T_JOIN_ROLE, "GREEN", None),
    ("S-T5-19-prod", [P_JOIN_ROLE_PRIMARY], P_JOIN, T_JOIN_ROLE, "RED", "fork-child-row-carries-subagent-role rule violated"),
    ("S-T5-19-weak", [P_JOIN_ROLE_PRIMARY, W_JOIN_ROLE], P_JOIN, T_JOIN_ROLE, "GREEN", None),
    ("S-T5-20-prod", [P_JOIN_FAKE_PROC], P_JOIN, T_JOIN_ROLE, "RED", "fork-child-row-invents-no-process rule violated"),
    ("S-T5-20-weak", [P_JOIN_FAKE_PROC, W_JOIN_PROC], P_JOIN, T_JOIN_ROLE, "GREEN", None),

    # --- the attach --------------------------------------------------------
    ("S-T5-21-prod", [P_ATT_NO_CLAUDE], P_SNAP, T_ATT_REG, "RED", "attach-graph-registers-native-collectors rule violated"),
    ("S-T5-21-weak", [P_ATT_NO_CLAUDE, W_ATT_REG], P_SNAP, T_ATT_REG, "GREEN", None),
    ("S-T5-22-prod", [P_ATT_NO_APPEND], P_SNAP, T_ATT_REG, "RED", "attach-graph-registers-native-collectors rule violated"),
    ("S-T5-22-weak", [P_ATT_NO_APPEND, W_ATT_REG], P_SNAP, T_ATT_REG, "GREEN", None),
    ("S-T5-23-prod", [P_ATT_WRAPPER_HOMES], P_SNAP, T_ATT_EMPTY, "RED", "attach-graph-empty-homes-is-occupancy-only rule violated"),
    ("S-T5-23-weak", [P_ATT_WRAPPER_HOMES, W_ATT_EMPTY], P_SNAP, T_ATT_EMPTY, "GREEN", None),
    ("S-T5-24-prod", [P_ATT_NIL_LATEST], P_SNAP, T_ATT_BIND, "RED", "attach-graph-native-spawn-edge-survives-admission rule violated"),
    ("S-T5-24-weak", [P_ATT_NIL_LATEST, W_ATT_EDGE, W_ATT_INC, W_ATT_PROC], P_SNAP, T_ATT_BIND, "GREEN", None),
    ("S-T5-25-prod", [P_ATT_NO_ABS], P_SNAP, T_ATT_ABS, "RED", "attach-home-is-absolute rule violated"),
    ("S-T5-25-weak", [P_ATT_NO_ABS, W_ATT_ABS], P_SNAP, T_ATT_ABS, "GREEN", None),
    ("S-T5-25b-prod", [P_ATT_NO_ABS], P_SNAP, T_ATT_REG, "GREEN", None),
    ("S-T5-25c-prod", [P_ATT_ABS_EMPTY], P_SNAP, T_ATT_ABS, "RED", "attach-empty-home-stays-empty rule violated"),

    # --- schema 2 ----------------------------------------------------------
    ("S-T5-26-prod", [P_JSN_NO_MAP], P_SNAP, T_SCHEMA, "RED", "schema-2-writes-an-unclaimed-state rule violated"),
    ("S-T5-26-weak", [P_JSN_NO_MAP, W_SCHEMA_WRITE, W_SCHEMA_JSON, W_SCHEMA_NODES], P_SNAP, T_SCHEMA, "GREEN", None),
    ("S-T5-27-prod", [P_JSN_ALWAYS_UNKNOWN], P_SNAP, T_SCHEMA, "GREEN", None),
    ("S-T5-27b-prod", [P_JSN_INVENTS_SINCE], P_SNAP, T_SCHEMA, "RED", "schema-2-writes-an-unclaimed-state rule violated"),
    ("S-T5-27b-weak", [P_JSN_INVENTS_SINCE, W_SCHEMA_WRITE, W_SCHEMA_JSON, W_SCHEMA_NODES, W_SCHEMA_SINCE], P_SNAP, T_SCHEMA, "GREEN", None),

    # --- the production run paths -------------------------------------------
    ("S-T5-28-prod", [P_MAI_CAP_EMPTY], P_MAIN, T_CAP_HOMES, "RED", "production-passes-engine-homes-to-the-graph rule violated"),
    ("S-T5-28-weak", [P_MAI_CAP_EMPTY, W_MAIN_HOMES, W_MAIN_ENVPREFIX], P_MAIN, T_CAP_HOMES, "GREEN", None),
    ("S-T5-28b-prod", [P_MAI_CAP_EMPTY], P_MAIN, T_INT_HOMES, "GREEN", None),
    ("S-T5-29-prod", [P_MAI_INT_EMPTY], P_MAIN, T_INT_HOMES, "RED", "production-passes-engine-homes-to-the-graph rule violated"),
    ("S-T5-29-weak", [P_MAI_INT_EMPTY, W_MAIN_HOMES], P_MAIN, T_INT_HOMES, "GREEN", None),
    ("S-T5-29b-prod", [P_MAI_INT_EMPTY], P_MAIN, T_CAP_HOMES, "GREEN", None),
    ("S-T5-30-prod", [P_MAI_DROP_CODEX], P_MAIN, T_CAP_HOMES, "RED", "production-passes-engine-homes-to-the-graph rule violated"),
    ("S-T5-30b-prod", [P_MAI_DROP_CODEX], P_MAIN, T_INT_HOMES, "RED", "production-passes-engine-homes-to-the-graph rule violated"),
    ("S-T5-30-weak", [P_MAI_DROP_CODEX, W_MAIN_HOMES], P_MAIN, T_CAP_HOMES, "GREEN", None),
    ("S-T5-31-prod", [P_MAI_SWAP], P_MAIN, T_CAP_HOMES, "RED", "production-passes-engine-homes-to-the-graph rule violated"),
    ("S-T5-31-weak", [P_MAI_SWAP, W_MAIN_HOMES], P_MAIN, T_CAP_HOMES, "GREEN", None),

    ("S-T5-32-prod", [P_MAI_OLD_ORDER], P_MAIN, T_ORDER, "RED", "capture-once-fills-rows-before-running-collectors rule violated"),
    ("S-T5-32-weak", [P_MAI_OLD_ORDER, W_MAIN_ORDER], P_MAIN, T_ORDER, "GREEN", None),

    # --- fix round 1: the first-tick signal ------------------------------
    ("S-T5-33-prod", [P_TICK_ON_ENTRY], P_NATIVE, T_TICK_AFTER, "RED", "native-first-tick-closes-only-when-the-tick-finishes rule violated"),
    ("S-T5-33-weak", [P_TICK_ON_ENTRY, W_TICK_AFTER_MID, W_TICK_AFTER_PUBLISHED], P_NATIVE, T_TICK_AFTER, "GREEN", None),
    ("S-T5-34-prod", [P_TICK_NOT_ON_ERROR], P_NATIVE, T_TICK_ERR, "RED", "native-first-tick-closes-on-a-failed-scan rule violated"),
    ("S-T5-34-weak", [P_TICK_NOT_ON_ERROR, W_TICK_ERR], P_NATIVE, T_TICK_ERR, "GREEN", None),
    ("S-T5-35-prod", [P_TICK_NO_ONCE], P_NATIVE, T_TICK_STAYS, "RED", "panic"),
    ("S-T5-36-prod", [P_TICK_NODES_ALL], P_NATIVE, T_TICK_NODES, "RED", "native-first-tick-nodes-exclude-what-the-store-refused rule violated"),
    ("S-T5-36-weak", [P_TICK_NODES_ALL, W_TICK_NODES_DROP], P_NATIVE, T_TICK_NODES, "GREEN", None),
    ("S-T5-37-prod", [P_TICK_ADMIT_DROPS], P_NATIVE, T_TICK_NODES, "RED", "native-first-tick-nodes-exclude-what-the-store-refused rule violated"),
    ("S-T5-37-weak", [P_TICK_ADMIT_DROPS, W_TICK_NODES_DROP], P_NATIVE, T_TICK_NODES, "GREEN", None),
    ("S-T5-38-prod", [P_TICK_NODES_FOREVER], P_NATIVE, T_TICK_NODES_ONE, "RED", "native-first-tick-nodes-stop-at-the-first-tick rule violated"),
    ("S-T5-38-weak", [P_TICK_NODES_FOREVER, W_TICK_NODES_ONE], P_NATIVE, T_TICK_NODES_ONE, "GREEN", None),

    # --- fix round 1: the bounded wait -----------------------------------
    ("S-T5-39-prod", [P_EV_UNBOUNDED], P_SNAP, T_EV_BOUND, "RED", "panic"),
    ("S-T5-40-prod", [P_EV_PER_LANE], P_SNAP, T_EV_SHARED, "RED", "wait-native-evidence-shares-one-budget-across-lanes rule violated"),
    ("S-T5-40-weak", [P_EV_PER_LANE, W_EV_SHARED, W_EV_SHARED_COUNT, W_EV_SHARED_ELAPSED], P_SNAP, T_EV_SHARED, "GREEN", None),
    ("S-T5-41-prod", [P_ATT_NO_LANES], P_SNAP, T_LANES, "RED", "attach-graph-returns-one-lane-per-home rule violated"),
    ("S-T5-41-weak", [P_ATT_NO_LANES, W_LANES_COUNT], P_SNAP, T_LANES, "GREEN", None),

    # --- fix round 1: the production one-shot ----------------------------
    ("S-T5-42-prod", [P_EV_NO_VISIBILITY], P_MAIN, T_CAP_UNDER, "RED", "capture-once-waits-for-a-native-lane-under-the-cap rule violated"),
    ("S-T5-42-weak", [P_EV_NO_VISIBILITY, W_CAP_UNDER], P_MAIN, T_CAP_UNDER, "GREEN", None),
    ("S-T5-43-prod", [P_MAI_NO_WAIT], P_MAIN, T_CAP_UNDER, "RED", "capture-once-waits-for-a-native-lane-under-the-cap rule violated"),
    ("S-T5-43-weak", [P_MAI_NO_WAIT, W_CAP_UNDER], P_MAIN, T_CAP_UNDER, "GREEN", None),
    ("S-T5-44-prod", [P_MAI_HUGE_BUDGET], P_MAIN, T_BUDGET, "RED", "native-first-tick-budget-is-two-seconds rule violated"),
    ("S-T5-44b-prod", [P_MAI_HUGE_BUDGET], P_MAIN, T_CAP_UNDER, "GREEN", None),
    ("S-T5-44-weak", [P_MAI_HUGE_BUDGET, W_BUDGET], P_MAIN, T_BUDGET, "GREEN", None),

    # --- fix wave 2: the newborn binding rule -----------------------------
    ("S-T5-47-prod", [P_NB_NO_WAIT], P_NATIVE, T_NEWBORN, "RED", "newborn-sighting-waits-for-its-binding rule violated"),
    ("S-T5-47-weak", [P_NB_NO_WAIT, W_NB_TICK1, W_NB_INCARNATION, W_NB_NEVER_INV], P_NATIVE, T_NEWBORN, "GREEN", None),
    ("S-T5-48-prod", [P_NB_ALWAYS], P_NATIVE, T_NEWBORN_ONCE, "RED", "newborn-sighting-is-waited-for-only-once rule violated"),
    ("S-T5-48-weak", [P_NB_ALWAYS, W_NB_ONCE, W_NB_STAYS], P_NATIVE, T_NEWBORN_ONCE, "GREEN", None),
    ("S-T5-49-prod", [P_NB_IGNORES_BINDING], P_NATIVE, T_BOUND_NOW, "RED", "bound-sighting-publishes-immediately rule violated"),
    ("S-T5-49-weak", [P_NB_IGNORES_BINDING, W_NB_BOUND], P_NATIVE, T_BOUND_NOW, "GREEN", None),
    ("S-T5-50-prod", [P_NB_WIDE_WINDOW], P_NATIVE, T_STALE_NOW, "RED", "stale-unbound-sighting-publishes-immediately rule violated"),
    ("S-T5-50-weak", [P_NB_WIDE_WINDOW, W_NB_STALE], P_NATIVE, T_STALE_NOW, "GREEN", None),
    ("S-T5-51-prod", [P_NB_CLAUDE_UNDATED], P_NATIVE, T_CLA_LIVE, "GREEN", None),
    ("S-T5-52-prod", [P_NB_HOLDS_TERMINALS], P_NATIVE, T_NEWBORN, "GREEN", None),
    ("S-T5-53-prod", [P_NB_NO_PRUNE], P_NATIVE, T_NEWBORN_ONCE, "GREEN", None),

    # --- fix wave 2: the screenshot size hole -----------------------------
    ("S-T5-54-prod", [P_SHOT_FIXED_WIDTH], P_MAIN, T_SHOT, "RED", "renders-the-requested-size rule violated"),
    ("S-T5-54-weak", [P_SHOT_FIXED_WIDTH, W_SHOT_WIDTH, W_SHOT_WIDTH_TABLE], P_MAIN, T_SHOT, "GREEN", None),

    # --- fix round 1: the portable ordering test --------------------------
    # Removing the injection CANNOT red the ordering test on this box, and that
    # is the finding rather than a gap: /proc here really does hold classified
    # agent processes, so the engine has rows either way. The recorded GREEN is
    # the honest statement of it.
    ("S-T5-45a-prod", [P_ORDER_NO_INJECT], P_MAIN, T_ORDER, "GREEN", None),
    # The portability property is asserted where it can be reproduced: against
    # an EMPTY spine, in the joiner.
    ("S-T5-45-prod", [P_JOIN_NO_DARK], P_JOIN, T_DARK, "RED", "dark-local-unit-needs-no-spine rule violated"),
    ("S-T5-45-weak", [P_JOIN_NO_DARK, W_DARK_ROWS], P_JOIN, T_DARK, "GREEN", None),
    ("S-T5-46-prod", [P_JOIN_DARK_ANY_LOCAL], P_JOIN, T_DARK, "RED", "unnamed-local-unit-is-not-a-dark-row rule violated"),
    ("S-T5-46-weak", [P_JOIN_DARK_ANY_LOCAL, W_DARK_UNNAMED], P_JOIN, T_DARK, "GREEN", None),
]


def preflight():
    """Apply every distinct patch alone and vet its package.

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
    dirty = [l for l in dirty if "tests/sabotage-native-task5/" not in l]
    if dirty:
        print("ABORT: tree not clean; the restore between patches would discard:")
        for line in dirty:
            print("   ", line)
        return 1

    seen, bad = set(), []
    for cid, patches, pkg, _rx, _expect, _phrase in CYCLES:
        for patch in patches:
            key = (patch[0], patch[1], patch[2], pkg)
            if key in seen:
                continue
            seen.add(key)
            full = os.path.join(REPO, patch[0])
            src = open(full).read()
            if src.count(patch[1]) != 1:
                bad.append((cid, patch[0], "old-string count=%d" % src.count(patch[1])))
                continue
            open(full, "w").write(src.replace(patch[1], patch[2], 1))
            r = sh(["go", "vet", pkg])
            sh(["git", "checkout", "--"] + SOURCES)
            if r.returncode != 0:
                bad.append((cid, patch[0], r.stderr.strip().splitlines()[-1][:140]))
    print("patches checked:", len(seen))
    for entry in bad:
        print("  BUILD-BREAK", entry)
    print("clean" if not bad else "FIX THESE")
    return 0 if not bad else 1


def main():
    if "--preflight" in sys.argv:
        sys.exit(preflight())
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
    if "--count" in sys.argv:
        print(len(CYCLES))
        sys.exit(0)
    logf = open(LOG, mode)

    def emit(msg):
        print(msg)
        logf.write(msg + "\n")
        logf.flush()

    def sh(args, timeout=600):
        # A plant CAN hang: removing a bound is exactly the mutation a bounded
        # wait needs tested. Every go test call therefore carries its own
        # -timeout, so Go kills the run and panics with a stack well inside the
        # subprocess timeout below.
        try:
            return subprocess.run(args, cwd=REPO, capture_output=True, text=True, timeout=timeout)
        except subprocess.TimeoutExpired as exc:
            # Never propagate: an exception here skips the restore and leaves a
            # plant in the tree, which is a far worse outcome than a mismatch.
            return subprocess.CompletedProcess(args, 124, exc.stdout or "", (exc.stderr or "") + "\nDRIVER: subprocess timed out")

    def gotest(pkg, rx):
        return sh(["go", "test", pkg, "-run", rx, "-count=1", "-timeout", GO_TEST_TIMEOUT])

    def porcelain():
        # This driver's own log lives in the repo on purpose, so the evidence is
        # re-runnable from a clean checkout. It is therefore the one path
        # expected to be dirty while the run is in progress, and the only one
        # excluded here: the check still has to see any plant that failed to
        # restore.
        lines = sh(["git", "status", "--porcelain"]).stdout.strip().splitlines()
        return "\n".join(l for l in lines if "tests/sabotage-native-task5/" not in l).strip()

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
    for cid, patches, pkg, rx, expect, phrase in CYCLES[lo:hi]:
        emit(f"\n===== {cid} expect={expect} pkg={pkg} run={rx}")
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
        r = gotest(pkg, rx)
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
        rg = gotest(pkg, rx)
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
    reds = sum(1 for c in CYCLES if c[4] == "RED")
    emit(f"total={len(results)} as_predicted={sum(1 for _, v in results if v == 'AS-PREDICTED')}")
    emit(f"production_RED={reds} predicted_GREEN={len(CYCLES) - reds}")


if __name__ == "__main__":
    main()
