#!/usr/bin/env python3
"""Sabotage driver for Task 4 (native grok session and subagent scanner).

Protocol, identical to Tasks 1 through 3: the code is committed FIRST, each
cycle plants an exact-match mutation, proves the plant is present (replace count
== 1 per patch and a non-empty git diff restricted to the source under test),
runs the focused test with -count=1, restores with `git checkout --`, proves the
restore (porcelain empty), and re-runs the focused test green. A cycle that does
not match its prediction stops the run for inspection rather than being recorded
and moved past.

Mutations are chosen by CLAIM, not by convenience: each one is aimed at a named
assertion, and the weakening column names every site that has to be relaxed
before the plant passes. Both the driver and its log live beside the tests they
certify, so the evidence is re-runnable from a clean checkout.
"""
import os
import subprocess
import sys
import time

REPO = "/home/aegis/Projects/aitop/.worktrees/native-provenance"
LOG = os.path.join(REPO, "tests/sabotage-native-task4/sabotage_runs_native_task4.log")

GRK = "internal/native/grok.go"
GRKT = "internal/native/grok_test.go"
ALL_FILES = [GRK, GRKT, "internal/native/codex.go", "internal/native/codex_test.go",
             "internal/native/claude.go", "internal/native/claude_test.go",
             "internal/native/native.go", "internal/native/native_test.go"]
PKG = "./internal/native"

T_MAINS = "^TestGrokScannerRosterAndDarkMains$"
T_LIFE = "^TestGrokScannerChildLifecycle$"
T_SPAWN = "^TestGrokScannerSpawnEdges$"
T_ENC = "^TestGrokScannerCWDEncoding$"
T_ROSTER = "^TestGrokScannerToleratesRosterShapes$"
T_SHADOW = "^TestGrokCollectorLandsSubagentInRealShadow$"

# --- production mutations: mains ------------------------------------------

# The display name is the harness. Printing agent_name puts a profile name in
# the roster column, which looks entirely plausible in the UI.
P_NAME_FROM_AGENT = (GRK,
                     "\t\tname = grokHarnessName",
                     "\t\tname = summary.AgentName")

# Every agent_name on disk starts with "grok", so the shorter prefix is the
# edit that looks like a harmless simplification and names everything Grok.
P_SHORT_PREFIX = (GRK,
                  '\tgrokBuildAgentPrefix = "grok-build"',
                  '\tgrokBuildAgentPrefix = "grok"')

# Publish every summary as a main. 228 of the 434 summaries on disk are
# subagents with a session directory of their own.
P_NO_KIND_GATE = (GRK,
                  "\tif strings.HasPrefix(summary.SessionKind, grokSubagentKindPrefix) {",
                  "\tif false && strings.HasPrefix(summary.SessionKind, grokSubagentKindPrefix) {")

# The bound this amendment REMOVED, planted as a mutation: gate the walk on the
# bucket directory's mtime again. A bucket's mtime moves when a session dir is
# created inside it and never when a session writes, so this is the defect the
# report measured (0 of 28 buckets inside the horizon while a session was live).
P_BUCKET_MTIME_BOUND = (GRK,
                        "\t\tentries, err := os.ReadDir(filepath.Join(sessions, bucket.Name()))",
                        "\t\tinfo, err := bucket.Info()\n\t\tif err != nil {\n\t\t\tcontinue\n\t\t}\n\t\tif now.Sub(info.ModTime()) > nativeHorizon {\n\t\t\tcontinue\n\t\t}\n\t\tentries, err := os.ReadDir(filepath.Join(sessions, bucket.Name()))")

# Drop the walk's stat prefilter: every summary on the box is read every tick,
# and a file whose mtime is older than the timestamp inside it is admitted.
P_NO_STAT_GATE = (GRK,
                  "\tif visit.roster == nil {",
                  "\tif false && visit.roster == nil {")

# Gate the roster route on the file's mtime too, which loses a session whose
# process is live and whose file has gone quiet.
P_STAT_GATE_EVERYWHERE = (GRK,
                          "\tif visit.roster == nil {",
                          "\tif true {")

# Widen the recorded-activity horizon 100x.
P_WIDE_HORIZON = (GRK,
                  "\tif now.Sub(*activity) > nativeHorizon {",
                  "\tif now.Sub(*activity) > 100*nativeHorizon {")

# Locate a main at its directory rather than at the file the fact came from.
P_MAIN_LOCATION_DIR = (GRK,
                       "\t\tLocation:  path,\n\t}, true",
                       "\t\tLocation:  filepath.Dir(path),\n\t}, true")

# Date a live session from created_at, which can belong to an older incarnation.
P_STARTED_IGNORES_ROSTER = (GRK,
                            "\t\t\tstarted = opened\n",
                            "")

# Claim liveness for a primary, which occupancy already owns.
P_MAIN_CLAIMS_ACTIVE = (GRK,
                        "\t\tStartedAt: started,\n\t\tLocation:  path,",
                        "\t\tStartedAt: started,\n\t\tState:     graph.StateActive,\n\t\tLocation:  path,")

# Derive the project from where the file lives rather than from the cwd.
P_PROJECT_FROM_PATH = (GRK,
                       "\t\tProject:   join.ProjectName(cwd),",
                       "\t\tProject:   join.ProjectName(filepath.Dir(filepath.Dir(path))),")

# Stop counting summaries classified as somebody's child.
P_NO_SUBAGENT_COUNT = (GRK,
                       "\t\ts.subagentSummaries.Add(1)\n",
                       "")

# Never read created_at, so a session with no roster entry has no start.
P_NO_CREATED_AT = (GRK,
                   "\tstarted := grokTime(summary.CreatedAt)",
                   "\tvar started *time.Time")

# Read the title out of a neighbouring key.
P_TASKNAME_FROM_AGENT = (GRK,
                         "\t\tTaskName:  summary.GeneratedTitle,",
                         "\t\tTaskName:  summary.AgentName,")

# Reach every session through the bucket walk alone. On the live box both roster
# sessions sit in a bucket whose mtime is hours old.
P_NO_ROSTER_PATH = (GRK,
                    "\troster := s.readRoster()",
                    "\troster := []grokRosterEntry(nil)")

# --- production mutations: children ---------------------------------------

# Claim liveness from the child's own last word alone.
P_ACTIVE_WITHOUT_ROSTER = (GRK,
                           "\t\t\tif visit.roster != nil {\n\t\t\t\tsighting.State = graph.StateActive\n\t\t\t}",
                           "\t\t\tif true {\n\t\t\t\tsighting.State = graph.StateActive\n\t\t\t}")

# Never claim liveness at all.
P_NEVER_ACTIVE = (GRK,
                  "\t\t\tif visit.roster != nil {\n\t\t\t\tsighting.State = graph.StateActive\n\t\t\t}",
                  "\t\t\tif false {\n\t\t\t\tsighting.State = graph.StateActive\n\t\t\t}")

# Roster absence read as death. This is the mass-vanish shape: a torn read of a
# file that is rewritten live would mark every child on the box gone, and a
# terminal is sticky for the whole incarnation.
P_ABSENT_ROSTER_VANISHES = (GRK,
                            "\t\t\tif visit.roster != nil {\n\t\t\t\tsighting.State = graph.StateActive\n\t\t\t}",
                            "\t\t\tif visit.roster != nil {\n\t\t\t\tsighting.State = graph.StateActive\n\t\t\t} else {\n\t\t\t\tsighting.Exit = graph.OutcomeVanished\n\t\t\t\tsighting.ExitAt = grokTime(meta.StartedAt)\n\t\t\t}")

# Treat a cancelled subagent as a completed one. 13 of 228 files on disk.
P_CANCELLED_COMPLETES = (GRK,
                         "\t\tcase grokStatusCompleted:\n\t\t\tsighting.Exit = graph.OutcomeCompleted",
                         '\t\tcase grokStatusCompleted, "cancelled":\n\t\t\tsighting.Exit = graph.OutcomeCompleted')

# Date the terminal from started_at.
P_EXITAT_FROM_START = (GRK,
                       "\t\tcase grokStatusCompleted:\n\t\t\tsighting.Exit = graph.OutcomeCompleted\n\t\t\tsighting.ExitAt = grokTime(meta.CompletedAt)",
                       "\t\tcase grokStatusCompleted:\n\t\t\tsighting.Exit = graph.OutcomeCompleted\n\t\t\tsighting.ExitAt = grokTime(meta.StartedAt)")

# A failure reported as a success. The graph evicts a success ghost in 5 minutes
# and a failure in 15, so this also shortens how long the evidence is visible.
P_FAILED_COMPLETES = (GRK,
                      "\t\tcase grokStatusFailed:\n\t\t\tsighting.Exit = graph.OutcomeFailed",
                      "\t\tcase grokStatusFailed:\n\t\t\tsighting.Exit = graph.OutcomeCompleted")

# Widen the child horizon 100x.
P_WIDE_CHILD_HORIZON = (GRK,
                        "\t\tif now.Sub(s.childActivity(meta, info.ModTime(), now)) > nativeHorizon {",
                        "\t\tif now.Sub(s.childActivity(meta, info.ModTime(), now)) > 100*nativeHorizon {")

# Date a child from meta.json alone. Its mtime is the spawn time while the child
# is running, so a job outliving the horizon disappears mid-flight.
P_META_MTIME_ONLY = (GRK,
                     "\tif now.Sub(metaModTime) <= nativeHorizon || meta.ChildCWD == \"\" {",
                     "\tif true {")

# Child content read out of neighbouring keys.
P_CHILD_NAME_FROM_DESC = (GRK,
                          "\t\t\tName:      meta.SubagentType,",
                          "\t\t\tName:      meta.Description,")
P_CHILD_ROLE_PRIMARY = (GRK,
                        "\t\t\tRole:      types.RoleSubagent,",
                        "\t\t\tRole:      types.RolePrimary,")
P_CHILD_SHORTID_FROM_PARENT = (GRK,
                               "\t\t\tSessionID: child,",
                               "\t\t\tSessionID: visit.session,")
P_CHILD_LOCATION_DIR = (GRK,
                        "\t\t\tLocation:  metaPath,",
                        "\t\t\tLocation:  filepath.Dir(metaPath),")

# Publish the sessions you know about first, then their children: the ordering
# that looks natural and files a subagent as a primary of its own.
P_MAINS_FIRST_A = (GRK,
                   "\tfor _, visit := range visits {\n\t\tsighting, ok := s.readMain(now, visit)\n\t\tif !ok || emitted[sighting.ID] {\n\t\t\tcontinue\n\t\t}\n\t\temitted[sighting.ID] = true\n\t\tnodes = append(nodes, sighting)\n\t}\n\n\t// Synthesized parents last",
                   "\t// Synthesized parents last")
P_MAINS_FIRST_B = (GRK,
                   "\t// Children first. Every subagent",
                   "\tfor _, visit := range visits {\n\t\tsighting, ok := s.readMain(now, visit)\n\t\tif !ok || emitted[sighting.ID] {\n\t\t\tcontinue\n\t\t}\n\t\temitted[sighting.ID] = true\n\t\tnodes = append(nodes, sighting)\n\t}\n\n\t// Children first. Every subagent")

# --- production mutations: edges and synthesis ----------------------------

P_REL_FROM_CHILD = (GRK,
                    "\t\t\tRelationship:    graph.RelationshipID(meta.SubagentID),",
                    "\t\t\tRelationship:    graph.RelationshipID(child),")
P_REL_FROM_PARENT = (GRK,
                     "\t\t\tRelationship:    graph.RelationshipID(meta.SubagentID),",
                     "\t\t\tRelationship:    graph.RelationshipID(visit.session),")
P_CHILD_SESSION_FROM_SUBAGENT_ID = (GRK,
                                    "\t\t\tChildSessionID:  child,",
                                    "\t\t\tChildSessionID:  meta.SubagentID,")
P_NO_SELF_GUARD = (GRK,
                   '\t\tif child == "" || child == visit.session {',
                   '\t\tif child == "" {')
P_NO_PARENT_MATCH = (GRK,
                     '\t\tif meta.ParentSessionID != "" && meta.ParentSessionID != visit.session {',
                     '\t\tif false && meta.ParentSessionID != "" && meta.ParentSessionID != visit.session {')
# A mismatch treated as a reason to drop the child rather than only its edge.
P_MISMATCH_DROPS_CHILD_A = (GRK,
                            "\t\tchildren = append(children, sighting)\n\n\t\tif parentErr != nil {",
                            "\t\tif parentErr != nil {")
P_MISMATCH_DROPS_CHILD_B = (GRK,
                            "\t\tspawns = append(spawns, SpawnSighting{",
                            "\t\tchildren = append(children, sighting)\n\t\tspawns = append(spawns, SpawnSighting{")
P_ALWAYS_SYNTHESIZE = (GRK,
                       "\t\tif emitted[id] {\n\t\t\tcontinue\n\t\t}\n\t\temitted[id] = true",
                       "\t\tif false && emitted[id] {\n\t\t\tcontinue\n\t\t}\n\t\temitted[id] = true")
P_NEVER_SYNTHESIZE = (GRK,
                      '\t\tif anchor != "" {\n\t\t\tanchors[i] = anchor\n\t\t}',
                      '\t\tif false && anchor != "" {\n\t\t\tanchors[i] = anchor\n\t\t}')
# Anchor on a child whose terminal the core will drop, so the parent outlives
# the only child it exists for.
P_ANCHOR_IGNORES_WINDOW = (GRK,
                           '\t\tif anchor == "" && !terminalOutsideWindow(sighting, now) {',
                           '\t\tif anchor == "" {')
P_SYNTH_PARENT_NAMED = (GRK,
                        "\t\t\tRole:      types.RolePrimary,\n\t\t\tLocation:  anchor,",
                        "\t\t\tRole:      types.RolePrimary,\n\t\t\tName:      grokHarnessName,\n\t\t\tLocation:  anchor,")
P_SYNTH_PARENT_VANISHES = (GRK,
                           "\t\t\tRole:      types.RolePrimary,\n\t\t\tLocation:  anchor,",
                           "\t\t\tRole:      types.RolePrimary,\n\t\t\tExit:      graph.OutcomeVanished,\n\t\t\tExitAt:    &now,\n\t\t\tLocation:  anchor,")
P_SPAWN_LOCATION_DIR = (GRK,
                        "\t\t\tLocation:        metaPath,",
                        "\t\t\tLocation:        filepath.Dir(metaPath),")

# --- production mutations: encoding and roster ----------------------------

ENC = '\treturn strings.ReplaceAll(strings.ReplaceAll(cwd, "/", "%2F"), " ", "%20")'
P_CLAUDE_SLUG = (GRK, ENC,
                 '\treturn strings.ReplaceAll(strings.ReplaceAll(cwd, "/", "-"), " ", "%20")')
# The form this amendment REPLACED. Slash-only encoding resolves a spaced cwd to
# a path that does not exist and loses the session on the roster route, which is
# 1 of the 434 sessions on the live box.
P_SLASH_ONLY = (GRK, ENC,
                '\treturn strings.ReplaceAll(cwd, "/", "%2F")')
P_DROP_LEADING_SLASH = (GRK, ENC,
                        '\treturn strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(cwd, "/"), "/", "%2F"), " ", "%20")')
# Anchored on the ReadFile branch above it: the same two-tab `if visit.roster`
# text is a substring of the three-tab one in scanStore, and a patch that could
# match either is a patch aimed at neither.
P_NO_ROSTER_MISS_COUNT = (GRK,
                          "\tif err != nil {\n\t\tif visit.roster != nil {",
                          "\tif err != nil {\n\t\tif false && visit.roster != nil {")
P_NO_MAP_ROSTER = (GRK,
                   "\t\tvar byKey map[string]grokRosterEntry\n\t\tif err := json.Unmarshal(body, &byKey); err != nil {",
                   "\t\tvar byKey map[string]grokRosterEntry\n\t\tif true {")
P_MISSING_ROSTER_COUNTED = (GRK,
                            "\t\tif !os.IsNotExist(err) {",
                            "\t\tif true || !os.IsNotExist(err) {")
# No roster, no walk: the disk stops being observed the moment the file is
# unreadable, rather than only the claims stopping.
P_NO_ROSTER_NO_WALK = (GRK,
                       "\tbuckets, err := os.ReadDir(sessions)\n\tif err != nil {",
                       "\tif len(visits) == 0 {\n\t\treturn visits\n\t}\n\tbuckets, err := os.ReadDir(sessions)\n\tif err != nil {")
P_EMPTY_HOME_ERRORS = (GRK,
                       "\tvisits := s.visits(now)",
                       "\tvisits := s.visits(now)\n\tif len(visits) == 0 {\n\t\treturn nil, nil, os.ErrNotExist\n\t}")

# --- weakened assertions --------------------------------------------------
#
# Every weakening keeps its original condition and its init statement, guarded
# by `false &&`. A weakening that DELETES the condition also deletes the only
# use of some local, and the compile error that follows is a fact about Go's
# unused-variable rule rather than about the assertion.

W_NAME_LIVE = (GRKT,
               '\tif live.Name != "Grok" {',
               '\tif false && live.Name != "Grok" {')
W_MAIN_CONTENT = (GRKT,
                  '\tif live.Model != grokModel || live.TaskName != "native provenance task 4" || live.Project != "aitop" {',
                  '\tif false && (live.Model != grokModel || live.TaskName != "native provenance task 4" || live.Project != "aitop") {')
W_MAIN_LOCATION = (GRKT,
                   "\tif live.Location != livePath {",
                   "\tif !strings.HasPrefix(livePath, live.Location) {")
W_LIVE_STARTED = (GRKT,
                  "\tif live.StartedAt == nil || !live.StartedAt.Equal(now.Add(-20*time.Minute)) {",
                  "\tif false && (live.StartedAt == nil || !live.StartedAt.Equal(now.Add(-20*time.Minute))) {")
W_MAIN_NOCLAIM = (GRKT,
                  '\tif live.State != "" || live.Exit != "" || live.ExitAt != nil {',
                  '\tif false && (live.State != "" || live.Exit != "" || live.ExitAt != nil) {')
W_DARK_CONTENT = (GRKT,
                  '\tif dark.Name != "Grok" || dark.Model != grokModel || dark.Project != "pensive" || dark.Location != darkPath {',
                  '\tif false && (dark.Name != "Grok" || dark.Model != grokModel || dark.Project != "pensive" || dark.Location != darkPath) {')
W_DARK_STARTED = (GRKT,
                  "\tif dark.StartedAt == nil || !dark.StartedAt.Equal(now.Add(-25*time.Minute)) {",
                  "\tif false && (dark.StartedAt == nil || !dark.StartedAt.Equal(now.Add(-25*time.Minute))) {")
W_DARK_NOCLAIM = (GRKT,
                  '\tif dark.State != "" || dark.Exit != "" {',
                  '\tif false && (dark.State != "" || dark.Exit != "") {')
W_NONBUILD = (GRKT,
              '\tif second.Name != "" || second.Location != secondPath {',
              '\tif false && (second.Name != "" || second.Location != secondPath) {')
W_MAINS_ABSENT = (GRKT,
                  '\t\tif seen := countNodeID(nodes, mustGrokID(t, absent.session)); seen != 0 {\n\t\t\tt.Fatalf("grok-dark-walk-admits-only-live-mains',
                  '\t\tif seen := countNodeID(nodes, mustGrokID(t, absent.session)); false && seen != 0 {\n\t\t\tt.Fatalf("grok-dark-walk-admits-only-live-mains')
W_MAINS_NODECOUNT = (GRKT,
                     "\tif len(nodes) != 5 {",
                     "\tif false && len(nodes) != 5 {")
W_MAINS_SKIPCOUNT = (GRKT,
                     "\tif skipped := scanner.skippedSummaries.Load(); skipped != 0 {",
                     "\tif skipped := scanner.skippedSummaries.Load(); false && skipped != 0 {")
# The amendment's headline reach: a live session in a bucket nobody has added a
# directory to for hours.
W_HOST_REACH_PRESENT = (GRKT,
                        "\thost, ok := nodeByID(nodes, mustGrokID(t, grokHostSession))\n\tif !ok {",
                        "\thost, ok := nodeByID(nodes, mustGrokID(t, grokHostSession))\n\tif false && !ok {")
W_HOST_REACH_CONTENT = (GRKT,
                        '\tif host.Location != hostPath || host.State != "" {',
                        '\tif false && (host.Location != hostPath || host.State != "") {')
# The other route: a live process whose file has gone quiet.
W_QUIET_PRESENT = (GRKT,
                   "\tquiet, ok := nodeByID(nodes, mustGrokID(t, grokQuietFileSession))\n\tif !ok {",
                   "\tquiet, ok := nodeByID(nodes, mustGrokID(t, grokQuietFileSession))\n\tif false && !ok {")
W_QUIET_CONTENT = (GRKT,
                   "\tif quiet.Location != quietPath || quiet.StartedAt == nil || !quiet.StartedAt.Equal(now.Add(-20*time.Minute)) {",
                   "\tif false && (quiet.Location != quietPath || quiet.StartedAt == nil || !quiet.StartedAt.Equal(now.Add(-20*time.Minute))) {")
W_SUBAGENT_COUNT = (GRKT,
                    "\tif seen := scanner.subagentSummaries.Load(); seen != 1 {",
                    "\tif seen := scanner.subagentSummaries.Load(); false && seen != 1 {")

W_CHILD_IDENTITY = (GRKT,
                    "\tif running.SessionID != grokRunningChild || running.Role != types.RoleSubagent || running.Runtime != types.RuntimeGrok {",
                    "\tif false && (running.SessionID != grokRunningChild || running.Role != types.RoleSubagent || running.Runtime != types.RuntimeGrok) {")
W_CHILD_CONTENT = (GRKT,
                   '\tif running.Name != "general-purpose" || running.Model != grokChildModel || running.TaskName != "survey the disk formats" || running.Project != "aitop" {',
                   '\tif false && (running.Name != "general-purpose" || running.Model != grokChildModel || running.TaskName != "survey the disk formats" || running.Project != "aitop") {')
W_CHILD_LOCATION = (GRKT,
                    "\tif running.Location != runningPath {",
                    "\tif !strings.HasPrefix(runningPath, running.Location) {")
W_COMPLETED_LOCATION = (GRKT,
                        "\tif completed.Location != completedPath {",
                        "\tif !strings.HasPrefix(completedPath, completed.Location) {")
W_RUNNING_ACTIVE = (GRKT,
                    '\tif running.State != graph.StateActive || running.Exit != "" {',
                    '\tif false && (running.State != graph.StateActive || running.Exit != "") {')
W_COMPLETED_TERMINAL = (GRKT,
                        '\tif completed.Exit != graph.OutcomeCompleted || completed.State != "" {',
                        '\tif false && (completed.Exit != graph.OutcomeCompleted || completed.State != "") {')
W_EXITAT = (GRKT,
            "\tif completed.ExitAt == nil || !completed.ExitAt.Equal(completedAt) {",
            "\tif false && (completed.ExitAt == nil || !completed.ExitAt.Equal(completedAt)) {")
W_FAILED = (GRKT,
            "\tif failed.Exit != graph.OutcomeFailed || failed.ExitAt == nil || !failed.ExitAt.Equal(failedAt) {",
            "\tif false && (failed.Exit != graph.OutcomeFailed || failed.ExitAt == nil || !failed.ExitAt.Equal(failedAt)) {")
W_CANCELLED = (GRKT,
               '\tif cancelled.Exit != "" || cancelled.ExitAt != nil || cancelled.State != "" {',
               '\tif false && (cancelled.Exit != "" || cancelled.ExitAt != nil || cancelled.State != "") {')
W_LONGRUN_PRESENT = (GRKT,
                     "\tlongRun, ok := nodeByID(nodes, mustGrokID(t, grokLongRunChild))\n\tif !ok {",
                     "\tlongRun, ok := nodeByID(nodes, mustGrokID(t, grokLongRunChild))\n\tif false && !ok {")
W_LONGRUN_STATE = (GRKT,
                   "\tif longRun.State != graph.StateActive {",
                   "\tif false && longRun.State != graph.StateActive {")
W_DARKCHILD_CLAIM = (GRKT,
                     '\tif darkChild.State != "" || darkChild.Exit != "" {',
                     '\tif false && (darkChild.State != "" || darkChild.Exit != "") {')
W_CHILD_WINS = (GRKT,
                '\tif darkChild.Role != types.RoleSubagent || darkChild.Name != "explore" || darkChild.Location != darkChildPath {',
                '\tif false && (darkChild.Role != types.RoleSubagent || darkChild.Name != "explore" || darkChild.Location != darkChildPath) {')
W_HOSTPARENT_PRESENT = (GRKT,
                        "\thostParent, ok := nodeByID(nodes, mustGrokID(t, grokHostSession))\n\tif !ok {",
                        "\thostParent, ok := nodeByID(nodes, mustGrokID(t, grokHostSession))\n\tif false && !ok {")
W_SYNTH_MINIMAL = (GRKT,
                   '\tif hostParent.Role != types.RolePrimary || hostParent.Name != "" || hostParent.Location != darkChildPath {',
                   '\tif false && (hostParent.Role != types.RolePrimary || hostParent.Name != "" || hostParent.Location != darkChildPath) {')
W_NOT_DEATH = (GRKT,
               '\tif hostParent.Exit != "" || hostParent.ExitAt != nil {',
               '\tif false && (hostParent.Exit != "" || hostParent.ExitAt != nil) {')
W_BURIED_ABSENT = (GRKT,
                   "\tif seen := countNodeID(nodes, mustGrokID(t, grokBuriedChild)); seen != 0 {",
                   "\tif seen := countNodeID(nodes, mustGrokID(t, grokBuriedChild)); false && seen != 0 {")
W_LIFE_NODECOUNT = (GRKT,
                    "\tif len(nodes) != 10 {",
                    "\tif false && len(nodes) != 10 {")
W_LIFE_SPAWNCOUNT = (GRKT,
                     "\tif len(spawns) != 8 {",
                     "\tif false && len(spawns) != 8 {")
W_PUB_NODECOUNT = (GRKT,
                   "\tif published := countKind(events, graph.EventNodeObserved); published != 8 {",
                   "\tif published := countKind(events, graph.EventNodeObserved); false && published != 8 {")
W_STATE_COUNT = (GRKT,
                 "\tif states := countKind(events, graph.EventStateObserved); states != 2 {",
                 "\tif states := countKind(events, graph.EventStateObserved); false && states != 2 {")
W_HEARTBEAT_COUNT = (GRKT,
                     "\tif beats := countKind(events, graph.EventHeartbeatObserved); beats != 2 {",
                     "\tif beats := countKind(events, graph.EventHeartbeatObserved); false && beats != 2 {")
W_EXIT_COUNT = (GRKT,
                "\tif exits := countKind(events, graph.EventExitObserved); exits != 2 {",
                "\tif exits := countKind(events, graph.EventExitObserved); false && exits != 2 {")
W_LIFE_EDGE_COUNT = (GRKT,
                     "\tif edges := countKind(events, graph.EventRelationshipObserved); edges != 6 {",
                     "\tif edges := countKind(events, graph.EventRelationshipObserved); false && edges != 6 {")
# Added after the first run of S-G12a: publishing mains first does not only
# change the dark child's role, it also makes the child pass find that node id
# already taken, which moves the skip counter. Two independent witnesses of one
# rule; predicting one and meeting the other is the finding.
W_WELLFORMED_METAS = (GRKT,
                      "\tif skipped := scanner.skippedChildren.Load(); skipped != 0 {",
                      "\tif skipped := scanner.skippedChildren.Load(); false && skipped != 0 {")
W_STALE_COUNT = (GRKT,
                 "\tif stale := collector.staleTerminals.Load(); stale != 2 {",
                 "\tif stale := collector.staleTerminals.Load(); false && stale != 2 {")

W_ORD_REL = (GRKT,
             "\tif ordinary.Relationship != graph.RelationshipID(grokRunningChild) || ordinary.Location != runningPath {",
             "\tif false && (ordinary.Relationship != graph.RelationshipID(grokRunningChild) || ordinary.Location != runningPath) {")
W_RELABEL_REL = (GRKT,
                 "\tif relabel.Relationship != graph.RelationshipID(grokRelabelID) {",
                 "\tif false && relabel.Relationship != graph.RelationshipID(grokRelabelID) {")
W_RELABEL_CHILD = (GRKT,
                   "\tif relabel.ChildSessionID != grokRelabeledChild {",
                   "\tif false && relabel.ChildSessionID != grokRelabeledChild {")
W_DARK_SPAWN_PARENT = (GRKT,
                       "\tif dark.ParentID != mustGrokID(t, grokHostSession) || dark.Location != darkChildPath {",
                       "\tif false && (dark.ParentID != mustGrokID(t, grokHostSession) || dark.Location != darkChildPath) {")
W_MISMATCH_EDGE = (GRKT,
                   "\tif _, ok := spawnByChild(spawns, mustGrokID(t, grokMismatchedChild)); ok {",
                   "\tif _, ok := spawnByChild(spawns, mustGrokID(t, grokMismatchedChild)); false && ok {")
W_MISMATCH_CHILD = (GRKT,
                    "\tif _, ok := nodeByID(nodes, mustGrokID(t, grokMismatchedChild)); !ok {",
                    "\tif _, ok := nodeByID(nodes, mustGrokID(t, grokMismatchedChild)); false && !ok {")
W_SPAWN_ABSENT = (GRKT,
                  '\t\tif seen := countNodeID(nodes, mustGrokID(t, absent.session)); seen != 0 {\n\t\t\tt.Fatalf("grok-skipped-child-synthesizes-no-parent',
                  '\t\tif seen := countNodeID(nodes, mustGrokID(t, absent.session)); false && seen != 0 {\n\t\t\tt.Fatalf("grok-skipped-child-synthesizes-no-parent')
W_NO_DUP_PARENT = (GRKT,
                   "\tif seen := countNodeID(nodes, parent); seen != 1 {",
                   "\tif seen := countNodeID(nodes, parent); false && seen != 1 {")
W_SPAWN_NODECOUNT = (GRKT,
                     "\tif len(nodes) != 7 {",
                     "\tif false && len(nodes) != 7 {")
W_SPAWN_EDGECOUNT = (GRKT,
                     "\tif len(spawns) != 4 {",
                     "\tif false && len(spawns) != 4 {")
W_SKIPPED_CHILDREN = (GRKT,
                      "\tif skipped := scanner.skippedChildren.Load(); skipped != 2 {",
                      "\tif skipped := scanner.skippedChildren.Load(); false && skipped != 2 {")
W_SKIPPED_SPAWNS = (GRKT,
                    "\tif skipped := scanner.skippedSpawns.Load(); skipped != 1 {",
                    "\tif skipped := scanner.skippedSpawns.Load(); false && skipped != 1 {")
W_SEAM_SELF_EDGE = (GRKT,
                    "\t\tif spawn.ParentID == spawn.ChildID {",
                    "\t\tif false && spawn.ParentID == spawn.ChildID {")

W_ENC_LOCATION = (GRKT,
                  "\tif live.Location != encodedPath {",
                  "\tif false && live.Location != encodedPath {")
W_ENC_CONTENT = (GRKT,
                 '\tif live.Model != grokModel || live.Project != "x" {',
                 '\tif false && (live.Model != grokModel || live.Project != "x") {')
W_ENC_SPACED_PRESENT = (GRKT,
                        "\tspaced, ok := nodeByID(nodes, mustGrokID(t, grokSecondSession))\n\tif !ok {",
                        "\tspaced, ok := nodeByID(nodes, mustGrokID(t, grokSecondSession))\n\tif false && !ok {")
W_ENC_SPACED_CONTENT = (GRKT,
                        '\tif spaced.Location != spacedPath || spaced.Project != "Obsidian Vault" {',
                        '\tif false && (spaced.Location != spacedPath || spaced.Project != "Obsidian Vault") {')
W_ENC_SKIPCOUNT = (GRKT,
                   "\tif skipped := scanner.skippedSummaries.Load(); skipped != 1 {",
                   "\tif skipped := scanner.skippedSummaries.Load(); false && skipped != 1 {")
W_ENC_NODECOUNT = (GRKT,
                   "\tif len(nodes) != 3 {",
                   "\tif false && len(nodes) != 3 {")

W_MAP_ROSTER_STATE = (GRKT,
                      "\t\tif !ok || child.State != graph.StateActive {\n\t\t\tt.Fatalf(\"grok-map-roster-parses",
                      "\t\tif false && (!ok || child.State != graph.StateActive) {\n\t\t\tt.Fatalf(\"grok-map-roster-parses")
W_MAP_ROSTER_COUNT = (GRKT,
                      "\t\tif skipped := scanner.skippedRoster.Load(); skipped != 0 {\n\t\t\tt.Fatalf(\"grok-map-roster-parses",
                      "\t\tif skipped := scanner.skippedRoster.Load(); false && skipped != 0 {\n\t\t\tt.Fatalf(\"grok-map-roster-parses")
W_ROSTER_SKIPCOUNT = (GRKT,
                      "\t\t\tif skipped := scanner.skippedRoster.Load(); skipped != broken.skipped {",
                      "\t\t\tif skipped := scanner.skippedRoster.Load(); false && skipped != broken.skipped {")
W_MASS_VANISH = (GRKT,
                 '\t\t\t\tif node.Exit != "" {',
                 '\t\t\t\tif false && node.Exit != "" {')
# Added after the second run of S-G18b: counting a missing roster moves the
# empty-home assertion too, which checks the same counter from the other side.
# Two witnesses of one rule, and only one of them was predicted.
W_EMPTY_HOME = (GRKT,
                "\t\tif len(nodes) != 0 || len(spawns) != 0 || scanner.skippedRoster.Load() != 0 || scanner.skippedSummaries.Load() != 0 {",
                "\t\tif false && (len(nodes) != 0 || len(spawns) != 0 || scanner.skippedRoster.Load() != 0 || scanner.skippedSummaries.Load() != 0) {")
W_SCAN_ERR = (GRKT,
              "\tnodes, spawns, err := scanner.scan(now)\n\tif err != nil {",
              "\tnodes, spawns, err := scanner.scan(now)\n\tif false && err != nil {")

CYCLES = [
    # --- what a main is, and what one must never be -------------------------
    ("S-G1a-prod", [P_NAME_FROM_AGENT], T_MAINS, "RED", "grok-build-agent-names-the-harness rule violated"),
    ("S-G1a-weak", [P_NAME_FROM_AGENT, W_NAME_LIVE, W_DARK_CONTENT, W_NONBUILD], T_MAINS, "GREEN", None),
    ("S-G1b-prod", [P_SHORT_PREFIX], T_MAINS, "RED", "grok-non-build-agent-is-unnamed rule violated"),
    ("S-G1b-weak", [P_SHORT_PREFIX, W_NONBUILD], T_MAINS, "GREEN", None),
    ("S-G2a-prod", [P_NO_KIND_GATE], T_MAINS, "RED", "grok-dark-walk-admits-only-live-mains rule violated"),
    ("S-G2a-weak", [P_NO_KIND_GATE, W_MAINS_ABSENT, W_MAINS_NODECOUNT, W_SUBAGENT_COUNT], T_MAINS, "GREEN", None),
    ("S-G2b-prod", [P_NO_SUBAGENT_COUNT], T_MAINS, "RED", "grok-subagent-summaries-counted rule violated"),
    ("S-G2b-weak", [P_NO_SUBAGENT_COUNT, W_SUBAGENT_COUNT], T_MAINS, "GREEN", None),
    ("S-G3a-prod", [P_BUCKET_MTIME_BOUND], T_MAINS, "RED", "grok-dark-walk-reaches-a-stale-bucket rule violated"),
    ("S-G3c-prod", [P_NO_STAT_GATE], T_MAINS, "RED", "grok-dark-walk-admits-only-live-mains rule violated"),
    ("S-G3c-weak", [P_NO_STAT_GATE, W_MAINS_ABSENT, W_MAINS_NODECOUNT], T_MAINS, "GREEN", None),
    ("S-G3d-prod", [P_STAT_GATE_EVERYWHERE], T_MAINS, "RED", "grok-roster-route-is-not-gated-on-file-mtime rule violated"),
    ("S-G3d-weak", [P_STAT_GATE_EVERYWHERE, W_QUIET_PRESENT, W_QUIET_CONTENT, W_MAINS_NODECOUNT], T_MAINS, "GREEN", None),
    ("S-G3b-prod", [P_WIDE_HORIZON], T_MAINS, "RED", "grok-dark-walk-admits-only-live-mains rule violated"),
    ("S-G3b-weak", [P_WIDE_HORIZON, W_MAINS_ABSENT, W_MAINS_NODECOUNT], T_MAINS, "GREEN", None),
    ("S-G4a-prod", [P_MAIN_LOCATION_DIR], T_MAINS, "RED", "grok-main-location-is-summary-file rule violated"),
    ("S-G4a-weak", [P_MAIN_LOCATION_DIR, W_MAIN_LOCATION, W_HOST_REACH_CONTENT, W_QUIET_CONTENT, W_DARK_CONTENT, W_NONBUILD], T_MAINS, "GREEN", None),
    ("S-G4b-prod", [P_PROJECT_FROM_PATH], T_MAINS, "RED", "grok-main-content-from-summary rule violated"),
    ("S-G4b-weak", [P_PROJECT_FROM_PATH, W_MAIN_CONTENT, W_DARK_CONTENT], T_MAINS, "GREEN", None),
    ("S-G4c-prod", [P_TASKNAME_FROM_AGENT], T_MAINS, "RED", "grok-main-content-from-summary rule violated"),
    ("S-G4c-weak", [P_TASKNAME_FROM_AGENT, W_MAIN_CONTENT], T_MAINS, "GREEN", None),
    ("S-G5a-prod", [P_STARTED_IGNORES_ROSTER], T_MAINS, "RED", "grok-roster-startedat-is-opened-at rule violated"),
    ("S-G5a-weak", [P_STARTED_IGNORES_ROSTER, W_LIVE_STARTED, W_QUIET_CONTENT], T_MAINS, "GREEN", None),
    ("S-G5b-prod", [P_NO_CREATED_AT], T_MAINS, "RED", "grok-dark-startedat-is-created-at rule violated"),
    ("S-G5b-weak", [P_NO_CREATED_AT, W_DARK_STARTED], T_MAINS, "GREEN", None),
    ("S-G6a-prod", [P_MAIN_CLAIMS_ACTIVE], T_MAINS, "RED", "grok-main-makes-no-state-or-terminal-claim rule violated"),
    ("S-G6a-weak", [P_MAIN_CLAIMS_ACTIVE, W_MAIN_NOCLAIM, W_HOST_REACH_CONTENT, W_DARK_NOCLAIM], T_MAINS, "GREEN", None),
    ("S-G7a-prod", [P_NO_ROSTER_PATH], T_MAINS, "RED", "grok-roster-startedat-is-opened-at rule violated"),
    ("S-G7b-prod", [P_NO_ROSTER_PATH, W_LIVE_STARTED], T_MAINS, "RED", "grok-roster-route-is-not-gated-on-file-mtime rule violated"),

    # --- the child lifecycle, and the two halves of the state rule ----------
    ("S-G8a-prod", [P_ACTIVE_WITHOUT_ROSTER], T_LIFE, "RED", "grok-running-child-of-roster-absent-parent-claims-nothing rule violated"),
    ("S-G8a-weak", [P_ACTIVE_WITHOUT_ROSTER, W_DARKCHILD_CLAIM, W_STATE_COUNT, W_HEARTBEAT_COUNT], T_LIFE, "GREEN", None),
    ("S-G8b-prod", [P_NEVER_ACTIVE], T_LIFE, "RED", "grok-running-child-of-roster-parent-is-active rule violated"),
    ("S-G8b-weak", [P_NEVER_ACTIVE, W_RUNNING_ACTIVE, W_LONGRUN_STATE, W_STATE_COUNT, W_HEARTBEAT_COUNT], T_LIFE, "GREEN", None),
    ("S-G9a-prod", [P_CANCELLED_COMPLETES], T_LIFE, "RED", "grok-cancelled-status-claims-nothing rule violated"),
    ("S-G9a-weak", [P_CANCELLED_COMPLETES, W_CANCELLED, W_EXIT_COUNT], T_LIFE, "GREEN", None),
    ("S-G9b-prod", [P_FAILED_COMPLETES], T_LIFE, "RED", "grok-failed-status-is-the-only-failed-terminal rule violated"),
    ("S-G9b-weak", [P_FAILED_COMPLETES, W_FAILED], T_LIFE, "GREEN", None),
    ("S-G9c-prod", [P_EXITAT_FROM_START], T_LIFE, "RED", "grok-exitat-is-completed-at rule violated"),
    ("S-G9c-weak", [P_EXITAT_FROM_START, W_EXITAT, W_PUB_NODECOUNT, W_EXIT_COUNT, W_LIFE_EDGE_COUNT, W_STALE_COUNT], T_LIFE, "GREEN", None),
    ("S-G10a-prod", [P_WIDE_CHILD_HORIZON], T_LIFE, "RED", "grok-child-horizon-skips-stale rule violated"),
    ("S-G10a-weak", [P_WIDE_CHILD_HORIZON, W_BURIED_ABSENT, W_LIFE_NODECOUNT, W_LIFE_SPAWNCOUNT, W_STALE_COUNT], T_LIFE, "GREEN", None),
    ("S-G10b-prod", [P_META_MTIME_ONLY], T_LIFE, "RED", "grok-long-running-child-stays-visible rule violated"),
    ("S-G11a-prod", [P_CHILD_ROLE_PRIMARY], T_LIFE, "RED", "grok-child-identity rule violated"),
    ("S-G11a-weak", [P_CHILD_ROLE_PRIMARY, W_CHILD_IDENTITY, W_CHILD_WINS], T_LIFE, "GREEN", None),
    ("S-G11b-prod", [P_CHILD_SHORTID_FROM_PARENT], T_LIFE, "RED", "grok-child-identity rule violated"),
    ("S-G11b-weak", [P_CHILD_SHORTID_FROM_PARENT, W_CHILD_IDENTITY], T_LIFE, "GREEN", None),
    ("S-G11c-prod", [P_CHILD_NAME_FROM_DESC], T_LIFE, "RED", "grok-child-content-from-meta rule violated"),
    ("S-G11c-weak", [P_CHILD_NAME_FROM_DESC, W_CHILD_CONTENT, W_CHILD_WINS], T_LIFE, "GREEN", None),
    ("S-G11d-prod", [P_CHILD_LOCATION_DIR], T_LIFE, "RED", "grok-child-location-is-meta-file rule violated"),
    ("S-G11d-weak", [P_CHILD_LOCATION_DIR, W_CHILD_LOCATION, W_COMPLETED_LOCATION, W_CHILD_WINS, W_SYNTH_MINIMAL], T_LIFE, "GREEN", None),
    ("S-G12a-prod", [P_MAINS_FIRST_A, P_MAINS_FIRST_B], T_LIFE, "RED", "grok-child-record-outranks-its-own-summary rule violated"),
    ("S-G12a-weak", [P_MAINS_FIRST_A, P_MAINS_FIRST_B, W_CHILD_WINS, W_WELLFORMED_METAS], T_LIFE, "GREEN", None),
    ("S-G13a-prod", [P_NEVER_SYNTHESIZE], T_LIFE, "RED", "grok-stale-parent-synthesized-for-live-child rule violated"),
    ("S-G13a-weak", [P_NEVER_SYNTHESIZE, W_HOSTPARENT_PRESENT, W_SYNTH_MINIMAL, W_LIFE_NODECOUNT, W_PUB_NODECOUNT, W_LIFE_EDGE_COUNT], T_LIFE, "GREEN", None),
    ("S-G13b-prod", [P_SYNTH_PARENT_NAMED], T_LIFE, "RED", "grok-synthesized-parent-is-minimal rule violated"),
    ("S-G13b-weak", [P_SYNTH_PARENT_NAMED, W_SYNTH_MINIMAL], T_LIFE, "GREEN", None),
    ("S-G13c-prod", [P_SYNTH_PARENT_VANISHES], T_LIFE, "RED", "grok-roster-absence-is-not-death rule violated"),
    ("S-G13c-weak", [P_SYNTH_PARENT_VANISHES, W_NOT_DEATH, W_EXIT_COUNT], T_LIFE, "GREEN", None),

    # --- edges, labels, and the endpoints they need -------------------------
    ("S-G14a-prod", [P_REL_FROM_CHILD], T_SPAWN, "RED", "grok-relationship-is-subagent-id rule violated"),
    ("S-G14a-weak", [P_REL_FROM_CHILD, W_RELABEL_REL], T_SPAWN, "GREEN", None),
    ("S-G14b-prod", [P_REL_FROM_PARENT], T_SPAWN, "RED", "grok-relationship-is-subagent-id rule violated"),
    ("S-G14b-weak", [P_REL_FROM_PARENT, W_ORD_REL, W_RELABEL_REL], T_SPAWN, "GREEN", None),
    ("S-G14c-prod", [P_CHILD_SESSION_FROM_SUBAGENT_ID], T_SPAWN, "RED", "grok-child-node-is-child-session-id rule violated"),
    ("S-G14c-weak", [P_CHILD_SESSION_FROM_SUBAGENT_ID, W_RELABEL_CHILD], T_SPAWN, "GREEN", None),
    ("S-G14d-prod", [P_SPAWN_LOCATION_DIR], T_SPAWN, "RED", "grok-relationship-is-subagent-id rule violated"),
    ("S-G14d-weak", [P_SPAWN_LOCATION_DIR, W_ORD_REL, W_DARK_SPAWN_PARENT], T_SPAWN, "GREEN", None),
    ("S-G15a-prod", [P_NO_SELF_GUARD], T_SPAWN, "RED", "grok-spawn-fixture-edge-count rule violated"),
    ("S-G15a-weak", [P_NO_SELF_GUARD, W_SPAWN_EDGECOUNT, W_SKIPPED_CHILDREN, W_SEAM_SELF_EDGE], T_SPAWN, "GREEN", None),
    ("S-G15b-prod", [P_NO_PARENT_MATCH], T_SPAWN, "RED", "grok-meta-parent-must-match-its-directory rule violated"),
    ("S-G15b-weak", [P_NO_PARENT_MATCH, W_MISMATCH_EDGE, W_SPAWN_EDGECOUNT, W_SKIPPED_SPAWNS], T_SPAWN, "GREEN", None),
    ("S-G15c-prod", [P_MISMATCH_DROPS_CHILD_A, P_MISMATCH_DROPS_CHILD_B], T_SPAWN, "RED", "grok-mismatched-parent-still-publishes-the-child rule violated"),
    ("S-G15c-weak", [P_MISMATCH_DROPS_CHILD_A, P_MISMATCH_DROPS_CHILD_B, W_MISMATCH_CHILD, W_SPAWN_NODECOUNT], T_SPAWN, "GREEN", None),
    ("S-G16a-prod", [P_ALWAYS_SYNTHESIZE], T_SPAWN, "RED", "grok-published-parent-not-duplicated rule violated"),
    ("S-G16a-weak", [P_ALWAYS_SYNTHESIZE, W_NO_DUP_PARENT, W_SPAWN_NODECOUNT], T_SPAWN, "GREEN", None),
    ("S-G16b-prod", [P_ANCHOR_IGNORES_WINDOW], T_SPAWN, "RED", "grok-skipped-child-synthesizes-no-parent rule violated"),
    ("S-G16b-weak", [P_ANCHOR_IGNORES_WINDOW, W_SPAWN_ABSENT, W_SPAWN_NODECOUNT], T_SPAWN, "GREEN", None),

    # --- the path arithmetic ------------------------------------------------
    ("S-G17a-prod", [P_CLAUDE_SLUG], T_ENC, "RED", "grok-roster-path-is-percent-encoded-cwd rule violated"),
    ("S-G17a-weak", [P_CLAUDE_SLUG, W_ENC_LOCATION, W_ENC_CONTENT, W_ENC_SPACED_PRESENT, W_ENC_SPACED_CONTENT, W_ENC_SKIPCOUNT, W_ENC_NODECOUNT], T_ENC, "GREEN", None),
    ("S-G17b-prod", [P_DROP_LEADING_SLASH], T_ENC, "RED", "grok-roster-path-is-percent-encoded-cwd rule violated"),
    ("S-G17b-weak", [P_DROP_LEADING_SLASH, W_ENC_LOCATION, W_ENC_CONTENT, W_ENC_SPACED_PRESENT, W_ENC_SPACED_CONTENT, W_ENC_SKIPCOUNT, W_ENC_NODECOUNT], T_ENC, "GREEN", None),
    ("S-G17c-prod", [P_SLASH_ONLY], T_ENC, "RED", "grok-spaced-cwd-is-reachable-by-roster-path rule violated"),
    ("S-G17c-weak", [P_SLASH_ONLY, W_ENC_SPACED_PRESENT, W_ENC_SPACED_CONTENT, W_ENC_SKIPCOUNT, W_ENC_NODECOUNT], T_ENC, "GREEN", None),
    ("S-G17d-prod", [P_NO_ROSTER_MISS_COUNT], T_ENC, "RED", "grok-unreadable-roster-summary-counted rule violated"),
    ("S-G17d-weak", [P_NO_ROSTER_MISS_COUNT, W_ENC_SKIPCOUNT], T_ENC, "GREEN", None),

    # --- the roster is evidence of life and never of death ------------------
    ("S-G18a-prod", [P_NO_MAP_ROSTER], T_ROSTER, "RED", "grok-map-roster-parses rule violated"),
    ("S-G18a-weak", [P_NO_MAP_ROSTER, W_MAP_ROSTER_STATE, W_MAP_ROSTER_COUNT], T_ROSTER, "GREEN", None),
    ("S-G18b-prod", [P_MISSING_ROSTER_COUNTED], T_ROSTER, "RED", "grok-unreadable-roster-counted rule violated"),
    ("S-G18b-weak", [P_MISSING_ROSTER_COUNTED, W_ROSTER_SKIPCOUNT, W_EMPTY_HOME], T_ROSTER, "GREEN", None),
    ("S-G18c-prod", [P_ABSENT_ROSTER_VANISHES], T_ROSTER, "RED", "grok-no-roster-is-not-a-mass-vanish rule violated"),
    ("S-G18c-weak", [P_ABSENT_ROSTER_VANISHES, W_MASS_VANISH], T_ROSTER, "GREEN", None),
    ("S-G18d-prod", [P_NO_ROSTER_NO_WALK], T_ROSTER, "RED", "grok-roster-case-canary rule violated"),
    ("S-G18e-prod", [P_EMPTY_HOME_ERRORS], T_ROSTER, "RED", "grok-scan-tolerates-disk rule violated"),
    ("S-G18e-weak", [P_EMPTY_HOME_ERRORS, W_SCAN_ERR], T_ROSTER, "GREEN", None),

    # --- judged by the production reconciler --------------------------------
    ("S-G19a-shadow-prod", [P_REL_FROM_PARENT], T_SHADOW, "RED", "grok-shadow-relationship-is-subagent-id rule violated"),
    ("S-G19b-shadow-prod", [P_NEVER_SYNTHESIZE], T_SHADOW, "RED", "grok-shadow-spawn-edge rule violated"),
    ("S-G19c-shadow-prod", [P_NEVER_ACTIVE], T_SHADOW, "RED", "grok-shadow-node-content rule violated"),
]


def main():
    logf = open(LOG, "w")

    def emit(msg):
        print(msg)
        logf.write(msg + "\n")
        logf.flush()

    def sh(args, timeout=300):
        return subprocess.run(args, cwd=REPO, capture_output=True, text=True, timeout=timeout)

    def porcelain():
        # This driver's own log lives in the repo on purpose, so the evidence is
        # re-runnable from a clean checkout rather than from a scratch directory.
        # It is therefore the one path expected to be dirty while the run is in
        # progress, and the only one excluded here: the check still has to see
        # any plant that failed to restore.
        lines = sh(["git", "status", "--porcelain"]).stdout.strip().splitlines()
        return "\n".join(l for l in lines if "tests/sabotage-native-task4/" not in l).strip()

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

    results = []
    for cid, patches, rx, expect, phrase in CYCLES:
        emit(f"\n===== {cid} expect={expect} run={rx}")
        if not all(apply_patch(*p) for p in patches):
            sh(["git", "checkout", "--"] + ALL_FILES)
            sys.exit(1)
        # Source files only. This driver's own log lives in the repo, so a
        # whole-tree diffstat would be non-empty whether or not a plant landed.
        diff = sh(["git", "diff", "--stat", "--", GRK, GRKT]).stdout.strip()
        if not diff:
            emit("ABORT: plant left no diff in the source under test")
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


if __name__ == "__main__":
    main()
