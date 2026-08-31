#!/usr/bin/env python3
"""Sabotage driver for aitop native-provenance Task 2 (Claude scanner + seam guard).

Same protocol as Task 1: for each new test, one physical production mutation
(predicted RED with a named rule phrase) and, where a weakening can green it,
the SAME production plant carrying weakened assertions (predicted false GREEN).
Cycles marked production-only exist where no single-site weakening can green the
plant; that is itself the finding, and it is recorded rather than manufactured.

Every cycle proves the plant is present (exact replace count == 1 per patch,
non-empty git diff), runs the focused test, restores via git checkout --, and
proves the restore (porcelain empty, focused test green again). Stops on the
first expectation mismatch.

Run from the native-provenance worktree, on a clean tree, AFTER the production
and test code is committed: git checkout -- discards uncommitted work.
"""
import subprocess, sys, os, time

REPO = "/home/aegis/Projects/aitop/.worktrees/native-provenance"
LOG = "/tmp/claude-1000/-home-aegis/beec7d46-5496-4bd5-ab9e-e4635720fb4c/scratchpad/sabotage_runs_native_task2.log"

CLA = "internal/native/claude.go"
CLAT = "internal/native/claude_test.go"
NAT = "internal/native/native.go"
NATT = "internal/native/native_test.go"
ALL_FILES = [CLA, CLAT, NAT, NATT]
PKG = "./internal/native"

# --- production mutations -------------------------------------------------

# P1: decode procStart as a number. It is a JSON string on disk, so every
# roster record fails to decode and the live session list empties.
P_PROCSTART = (CLA,
               '\tProcStart string `json:"procStart"`',
               '\tProcStart uint64 `json:"procStart"`')

# P2: point a primary's location at the directory instead of the file.
P_LOCATION = (CLA,
              "\t\t\tLocation:  sidecar.path,",
              "\t\t\tLocation:  filepath.Dir(sidecar.path),")

# P3: read startedAt as epoch seconds instead of milliseconds.
P_EPOCH = (CLA,
           "\tat := time.UnixMilli(millis).UTC()",
           "\tat := time.Unix(millis, 0).UTC()")

# P4: stop counting the sidecar we skipped for being undecodable.
P_SKIPCOUNT = (CLA,
               "\t\tvar decoded claudeSidecarFile\n\t\tif err := json.Unmarshal(body, &decoded); err != nil || decoded.SessionID == \"\" {\n\t\t\ts.skippedSidecars.Add(1)\n\t\t\tcontinue\n\t\t}",
               "\t\tvar decoded claudeSidecarFile\n\t\tif err := json.Unmarshal(body, &decoded); err != nil || decoded.SessionID == \"\" {\n\t\t\tcontinue\n\t\t}")

# P5: ignore parentAgentId, so a nested child hangs off the session and the
# spawn chain flattens into siblings.
P_PARENTAGENT = (CLA,
                 "\tif parentAgent != \"\" {\n\t\treturn graph.ClaudeAgentID(session, parentAgent)\n\t}\n\treturn graph.ClaudeSessionID(session)",
                 "\treturn graph.ClaudeSessionID(session)")

# P6: drop the agentType fallback, so the many metas with no name render blank.
P_NAMEFALLBACK = (CLA,
                  "\t\t\tName:      firstNonEmpty(meta.Name, meta.AgentType),",
                  "\t\t\tName:      meta.Name,")

# P7: label the edge with the child's canonical NodeID instead of its short id.
# This is the survey's named trap and the reason the seam guard exists.
P_RELCANON = (CLA,
              "\t\t\tRelationship:    graph.RelationshipID(agent),",
              "\t\t\tRelationship:    graph.RelationshipID(childID),")

# P8: stop inheriting the project from the live parent's cwd.
P_PROJECT = (CLA,
             "\t\t\tsighting.Project = join.ProjectName(parent.CWD)",
             "\t\t\tsighting.Project = \"\"")

# P9: horizon-gate on the meta mtime alone, losing a long-running child whose
# spawn record was written hours ago.
P_MAXACTIVITY = (CLA,
                 "\t\t\ttranscriptAt = transcript.ModTime()\n\t\t\tif transcriptAt.After(activity) {\n\t\t\t\tactivity = transcriptAt\n\t\t\t}",
                 "\t\t\ttranscriptAt = transcript.ModTime()")

# P10: widen the sidecar horizon, admitting sessions with no recent activity.
P_SIDECARHORIZON = (CLA,
                    "\t\tif now.Sub(sidecar.modTime) > nativeHorizon {",
                    "\t\tif now.Sub(sidecar.modTime) > 100*nativeHorizon {")

# P11: widen the agent horizon the same way.
P_AGENTHORIZON = (CLA,
                  "\t\tif now.Sub(activity) > nativeHorizon {",
                  "\t\tif now.Sub(activity) > 100*nativeHorizon {")

# P12: widen the activity window, so a child that stopped writing minutes ago
# is still claimed as running.
P_ACTIVEWINDOW = (CLA,
                  "\tclaudeChildActiveWindow = 30 * time.Second",
                  "\tclaudeChildActiveWindow = 5 * time.Minute")

# P13: date the terminal at the meta mtime instead of the last activity.
P_VANISHAT = (CLA,
              "\t\t\tvanishedAt := activity",
              "\t\t\tvanishedAt := info.ModTime()")

# P14: never vanish an orphan, so a child whose parent is gone lives forever.
# First attempt at this plant forced the live branch (`if parent != nil` to
# `if true`), which dereferenced a nil sidecar and panicked in the Project line
# before the vanish assertion ran: too blunt to say anything about this claim.
# Emptying the vanish branch is the edit that fits it.
P_NEVERVANISH = (CLA,
                 "\t\t\tvanishedAt := activity\n\t\t\tsighting.Exit = graph.OutcomeVanished\n\t\t\tsighting.ExitAt = &vanishedAt\n",
                 "")

# P15: take the child's short id from the enclosing session instead of its own
# agent id, which is what reading it out of the transcript would produce.
P_CHILDSESSIONID = (CLA,
                    "\t\t\tSessionID: agent,\n\t\t\tRuntime:   types.RuntimeClaude,\n\t\t\tRole:      types.RoleSubagent,",
                    "\t\t\tSessionID: session,\n\t\t\tRuntime:   types.RuntimeClaude,\n\t\t\tRole:      types.RoleSubagent,")

# P16: forget to strip the agent- prefix off the filename.
P_TRIMPREFIX = (CLA,
                "\t\tagent := strings.TrimSuffix(strings.TrimPrefix(name, claudeAgentPrefix), claudeMetaSuffix)",
                "\t\tagent := strings.TrimSuffix(name, claudeMetaSuffix)")

# P17: give the child the session's node identity, the full transcript trap.
P_CHILDISSESSION = (CLA,
                    "\t\tchildID, err := graph.ClaudeAgentID(session, agent)",
                    "\t\tchildID, err := graph.ClaudeSessionID(session)")

# P18: stop claiming a running child, so nothing keeps the node active.
P_NOSTATE = (CLA,
             "\t\t\tif !transcriptAt.IsZero() && now.Sub(transcriptAt) < claudeChildActiveWindow {\n\t\t\t\tsighting.State = graph.StateActive\n\t\t\t}",
             "")

# P19: keep the length half of the seam guard, drop the id-shape half. A
# canonical session NodeID is 51 bytes, so the length bound cannot see it.
P_NOCOLONCHECK = (NAT,
                  "\treturn !strings.Contains(string(relationship), \":\")",
                  "\treturn true")

# P20: remove the seam guard entirely.
P_NOGUARD = (NAT,
             "\t\tif !usableRelationship(spawn.Relationship) {\n\t\t\tc.malformedRelationships.Add(1)\n\t\t\tcontinue\n\t\t}\n",
             "")

# --- weakened assertions --------------------------------------------------

W_LOCATION_A = (CLAT,
                "\tif first.Location != firstPath {",
                "\tif !strings.Contains(first.Location, \"sessions\") {")
W_LOCATION_B = (CLAT,
                "\tif second.Project != \"aitop/native-provenance\" || second.Location != secondPath {",
                "\tif second.Project != \"aitop/native-provenance\" {")

W_EPOCH = (CLAT,
           "\tif first.StartedAt == nil || first.StartedAt.UnixMilli() != claudeFixtureStartedAt {",
           "\tif first.StartedAt == nil {")

W_SKIPCOUNT = (CLAT,
               "\tif skipped := scanner.skippedSidecars.Load(); skipped != 2 {",
               "\tif skipped := scanner.skippedSidecars.Load(); skipped > 2 {")

W_PARENTAGENT = (CLAT,
                 "\tif nestedSpawn.ParentID != named || nestedSpawn.ParentSessionID != claudeNamedAgent {",
                 "\tif nestedSpawn.ParentID == \"\" {")

W_NAMEFALLBACK = (CLAT,
                  "\tif nestedNode.Name != \"Explore\" || nestedNode.Model != \"\" {",
                  "\tif nestedNode.Model != \"\" {")

W_REL_A = (CLAT,
           "\tif namedSpawn.Relationship != graph.RelationshipID(claudeNamedAgent) || namedSpawn.ChildSessionID != claudeNamedAgent {",
           "\tif namedSpawn.ChildSessionID != claudeNamedAgent {")
W_REL_B = (CLAT,
           "\tif nestedSpawn.Relationship != graph.RelationshipID(claudeNestedAgent) || nestedSpawn.Location != nestedMeta {",
           "\tif nestedSpawn.Location != nestedMeta {")
W_REL_C = (CLAT,
           "\t\tif strings.Contains(string(spawn.Relationship), \":\") {",
           "\t\tif strings.Contains(string(spawn.Relationship), \"\\x00\") {")
W_REL_D = (CLAT,
           "\t\tif len(spawn.Relationship) == 0 || len(spawn.Relationship) > 64 {",
           "\t\tif len(spawn.Relationship) == 0 {")

W_PROJECT = (CLAT,
             "\tif namedNode.Project != \"aitop\" {\n\t\tt.Fatalf(\"claude-agent-project-from-live-parent rule violated: project=%q want=%q\", namedNode.Project, \"aitop\")\n\t}\n",
             "")

W_MIXED = (CLAT,
           "\t\tmustAgentID(t, claudeFixtureSession, \"amixed00000000000\"),\n",
           "")
W_SPAWNCOUNT_C3 = (CLAT,
                   "\tif len(spawns) != 2 {\n\t\tt.Fatalf(\"claude-horizon-keeps-fresh-edges rule violated: spawns=%d want=2\", len(spawns))\n\t}",
                   "")

W_ABSENT = (CLAT,
            "\tabsent := []graph.NodeID{\n\t\tmustSessionID(t, claudeFixtureSecond),\n\t\tmustAgentID(t, claudeFixtureSession, \"astale00000000000\"),\n\t}",
            "\tabsent := []graph.NodeID{\n\t\tmustAgentID(t, claudeFixtureSession, \"astale00000000000\"),\n\t}")
W_NODECOUNT = (CLAT,
               "\tif len(nodes) != len(present) {",
               "\tif len(nodes) < len(present) {")

W_QUIET = (CLAT,
           "\tif quiet.State != \"\" || quiet.Exit != \"\" {",
           "\tif quiet.Exit != \"\" {")

W_EXITAT = (CLAT,
            "\tif orphan.ExitAt == nil || !orphan.ExitAt.Equal(orphanAt) {",
            "\tif orphan.ExitAt == nil {")
W_STALECOUNT = (CLAT,
                "\tif skipped := collector.staleTerminals.Load(); skipped != 1 {",
                "\tif skipped := collector.staleTerminals.Load(); skipped < 1 {")
W_FRESHORPHAN = (CLAT,
                 "\tif _, ok := firstOfKind(events, graph.EventExitObserved, orphanID); !ok {",
                 "\tif _, ok := firstOfKind(events, graph.EventExitObserved, orphanID); false && !ok {")

W_SESSIONID = (CLAT,
               "\t\tif node.SessionID != agent {\n\t\t\tt.Fatalf(\"claude-agent-id-from-filename rule violated: sessionID=%q want=%q id=%s\", node.SessionID, agent, node.ID)\n\t\t}\n",
               "")
W_NEVERTRANSCRIPT = (CLAT,
                     "\t\tif node.ID == session || node.SessionID == claudeFixtureThird {",
                     "\t\tif node.ID == session {")

W_SHADOWSOURCE = (CLAT,
                  "\t\t\t\tif edge.Source != want.parent || edge.Type != graph.EdgeSpawn || edge.Provenance != graph.ProvenanceNative {",
                  "\t\t\t\tif edge.Type != graph.EdgeSpawn || edge.Provenance != graph.ProvenanceNative {")
W_SHADOWSTATE = (CLAT,
                 "\t\t\tif node := nodes[named]; node.Role != types.RoleSubagent || node.ProvenName != \"lane1r-architect\" || node.State.Value != graph.StateActive {",
                 "\t\t\tif node := nodes[named]; node.Role != types.RoleSubagent || node.ProvenName != \"lane1r-architect\" {")

W_EDGECOUNT = (NATT,
               "\tif edges := countKind(events, graph.EventRelationshipObserved); edges != 1 {",
               "\tif edges := countKind(events, graph.EventRelationshipObserved); edges < 1 {")
# Keeps data in use: dropping the term entirely leaves the binding unused and
# the package stops compiling, which is a build failure rather than a green.
W_DATAREL = (NATT,
             "\tif !isEdge || data.Relationship != graph.RelationshipID(testChildAgent) {",
             "\tif !isEdge || data.Type != graph.EdgeSpawn {")
W_MALFORMEDCOUNT = (NATT,
                    "\tif dropped := c.malformedRelationships.Load(); dropped != 3 {",
                    "\tif dropped := c.malformedRelationships.Load(); dropped < 2 {")

T_C1 = "^TestClaudeScannerFindsSidecarPrimaries$"
T_C2 = "^TestClaudeScannerEmitsAgentSpawnChain$"
T_C3 = "^TestClaudeScannerHorizonSkipsStale$"
T_C4 = "^TestClaudeScannerVanishesOrphanedChildren$"
T_C5 = "^TestClaudeScannerAgentIDFromFilename$"
T_C6 = "^TestClaudeCollectorLandsChainInRealShadow$"
T_C7 = "^TestNativeSpawnRelationshipSeamGuard$"

CYCLES = [
    ("S-C1a-prod", [P_PROCSTART], T_C1, "RED", "claude-sidecar-primary-count rule violated: nodes=0 want=2"),
    ("S-C1b-prod", [P_LOCATION], T_C1, "RED", "claude-sidecar-location-is-source-file rule violated"),
    ("S-C1b-weak", [P_LOCATION, W_LOCATION_A, W_LOCATION_B], T_C1, "GREEN", None),
    ("S-C1c-prod", [P_EPOCH], T_C1, "RED", "claude-sidecar-startedat-from-epoch-ms rule violated"),
    ("S-C1c-weak", [P_EPOCH, W_EPOCH], T_C1, "GREEN", None),
    # Prediction corrected after the first run: both unusable roster files fail
    # in this one branch (undecodable JSON and an empty sessionId share it), so
    # removing the counter takes the tally to 0, not to 1.
    ("S-C1d-prod", [P_SKIPCOUNT], T_C1, "RED", "claude-sidecar-skips-counted rule violated: skippedSidecars=0"),
    ("S-C1d-weak", [P_SKIPCOUNT, W_SKIPCOUNT], T_C1, "GREEN", None),

    ("S-C2a-prod", [P_PARENTAGENT], T_C2, "RED", "claude-spawn-parent-from-parentagentid rule violated"),
    ("S-C2a-weak", [P_PARENTAGENT, W_PARENTAGENT], T_C2, "GREEN", None),
    ("S-C2b-prod", [P_NAMEFALLBACK], T_C2, "RED", "claude-agent-name-falls-back-to-type rule violated"),
    ("S-C2b-weak", [P_NAMEFALLBACK, W_NAMEFALLBACK], T_C2, "GREEN", None),
    ("S-C2c-prod", [P_RELCANON], T_C2, "RED", "claude-spawn-relationship-is-agent-id rule violated"),
    ("S-C2c-weak", [P_RELCANON, W_REL_A, W_REL_B, W_REL_C, W_REL_D], T_C2, "GREEN", None),
    ("S-C2c-shadow", [P_RELCANON], T_C6, "RED", "claude-shadow-spawn-chain rule violated: fewer than 2 edges"),
    ("S-C2d-prod", [P_PROJECT], T_C2, "RED", "claude-agent-project-from-live-parent rule violated"),
    ("S-C2d-weak", [P_PROJECT, W_PROJECT], T_C2, "GREEN", None),

    ("S-C3a-prod", [P_MAXACTIVITY], T_C3, "RED", "claude-horizon-admits-recent-activity rule violated"),
    ("S-C3a-weak", [P_MAXACTIVITY, W_MIXED, W_SPAWNCOUNT_C3], T_C3, "GREEN", None),
    ("S-C3b-prod", [P_SIDECARHORIZON], T_C3, "RED", "claude-horizon-skips-stale rule violated"),
    ("S-C3b-weak", [P_SIDECARHORIZON, W_ABSENT, W_NODECOUNT], T_C3, "GREEN", None),
    ("S-C3c-prod", [P_AGENTHORIZON], T_C3, "RED", "claude-horizon-skips-stale rule violated"),

    ("S-C4a-prod", [P_ACTIVEWINDOW], T_C4, "RED", "claude-quiet-child-makes-no-claim rule violated"),
    ("S-C4a-weak", [P_ACTIVEWINDOW, W_QUIET], T_C4, "GREEN", None),
    ("S-C4b-prod", [P_VANISHAT], T_C4, "RED", "claude-orphan-exitat-is-last-activity rule violated"),
    ("S-C4b-weak", [P_VANISHAT, W_EXITAT, W_STALECOUNT, W_FRESHORPHAN], T_C4, "GREEN", None),
    ("S-C4c-prod", [P_NEVERVANISH], T_C4, "RED", "claude-orphan-child-vanishes rule violated"),

    ("S-C5a-prod", [P_CHILDSESSIONID], T_C5, "RED", "claude-agent-id-from-filename rule violated: sessionID="),
    ("S-C5a-weak", [P_CHILDSESSIONID, W_SESSIONID, W_NEVERTRANSCRIPT], T_C5, "GREEN", None),
    ("S-C5b-prod", [P_TRIMPREFIX], T_C5, "RED", "claude-agent-id-from-filename rule violated: agent=aapi-scout"),
    ("S-C5c-prod", [P_CHILDISSESSION], T_C5, "RED", "claude-agent-id-from-filename rule violated"),

    ("S-C6a-prod", [P_PARENTAGENT], T_C6, "RED", "claude-shadow-spawn-chain rule violated: child="),
    ("S-C6a-weak", [P_PARENTAGENT, W_SHADOWSOURCE], T_C6, "GREEN", None),
    ("S-C6b-prod", [P_NOSTATE], T_C6, "RED", "claude-shadow-node-content rule violated"),
    ("S-C6b-weak", [P_NOSTATE, W_SHADOWSTATE], T_C6, "GREEN", None),

    ("S-C7a-prod", [P_NOCOLONCHECK], T_C7, "RED", "native-spawn-relationship-seam-guard rule violated: relationshipEvents=2"),
    ("S-C7a-weak", [P_NOCOLONCHECK, W_EDGECOUNT, W_DATAREL, W_MALFORMEDCOUNT], T_C7, "GREEN", None),
    ("S-C7b-prod", [P_NOGUARD], T_C7, "RED", "native-emitted-events-validate rule violated"),
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
