#!/usr/bin/env python3
"""Sabotage driver for Task 3 (native codex thread and fork scanner).

Protocol, identical to Tasks 1 and 2: the code is committed FIRST, each cycle
plants an exact-match mutation, proves the plant is present (replace count == 1
per patch and a non-empty git diff), runs the focused test with -count=1,
restores with `git checkout --`, proves the restore (porcelain empty), and
re-runs the focused test green. A cycle that does not match its prediction stops
the run for inspection rather than being recorded and moved past.

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
LOG = os.path.join(REPO, "tests/sabotage-native-task3/sabotage_runs_native_task3.log")

COD = "internal/native/codex.go"
CODT = "internal/native/codex_test.go"
ALL_FILES = [COD, CODT, "internal/native/claude.go", "internal/native/claude_test.go",
             "internal/native/native.go", "internal/native/native_test.go"]
PKG = "./internal/native"

T_ID = "^TestCodexScannerNodeUsesOwnThreadID$"
T_SPAWN = "^TestCodexScannerSpawnFromParentThreadID$"
T_SKIP = "^TestCodexScannerSkipsMalformedAndStale$"
T_DIRS = "^TestCodexScannerScansOnlyRecentDateDirs$"
T_CLAIM = "^TestCodexCollectorMakesNoStateOrTerminalClaims$"
T_SHADOW = "^TestCodexCollectorLandsForkInRealShadow$"

# --- production mutations -------------------------------------------------

# A1: the full trap. Key the node on session_id, the ROOT of the run.
P_NODE_FROM_SESSION = (COD,
                       "\t\tid, err := graph.CodexThreadID(rollout.meta.ID)",
                       "\t\tid, err := graph.CodexThreadID(rollout.meta.SessionID)")

# A2: half the trap. Keep the right NodeID, take the SHORT id from session_id.
# The short id is what feeds the invocation incarnation, so this binds a
# thread's incarnation to the root's while every node id still looks correct.
P_SHORTID_FROM_SESSION = (COD,
                          "\t\t\tSessionID: rollout.meta.ID,\n\t\t\tRuntime:   types.RuntimeCodex,",
                          "\t\t\tSessionID: rollout.meta.SessionID,\n\t\t\tRuntime:   types.RuntimeCodex,")

# A3: read the nickname from a sibling key inside the same spawn record.
P_NICK_FROM_ROLE = (COD,
                    '\t\t\tAgentNickname string `json:"agent_nickname"`',
                    '\t\t\tAgentNickname string `json:"agent_role"`')

# A4: derive the project from where the rollout FILE lives rather than from the
# thread's cwd. Deleting the field instead would remove the only use of the join
# import and fail the build, which is a fact about the compiler rather than about
# the assertion under test.
P_PROJECT_FROM_PATH = (COD,
                       "\t\t\tProject:   join.ProjectName(rollout.meta.CWD),\n\t\t\tLocation:  rollout.path,",
                       "\t\t\tProject:   join.ProjectName(filepath.Dir(rollout.path)),\n\t\t\tLocation:  rollout.path,")

# A5: locate the sighting at the directory rather than the file.
P_LOCATION_DIR = (COD,
                  "\t\t\tProject:   join.ProjectName(rollout.meta.CWD),\n\t\t\tLocation:  rollout.path,",
                  "\t\t\tProject:   join.ProjectName(rollout.meta.CWD),\n\t\t\tLocation:  filepath.Dir(rollout.path),")

# A6: claim liveness a rollout cannot witness.
P_CLAIMS_ACTIVE = (COD,
                   "\t\t\tLocation:  rollout.path,\n\t\t\t// No state claim",
                   "\t\t\tLocation:  rollout.path,\n\t\t\tState:     graph.StateActive,\n\t\t\t// No state claim")

# B1: take the parent from session_id instead of parent_thread_id, which hangs
# every depth-2 thread straight off the root and flattens the tree.
P_PARENT_FROM_SESSION = (COD,
                         "\t\tparent := rollout.meta.ParentThreadID",
                         "\t\tparent := rollout.meta.SessionID")

# B2: label the edge with the root instead of the child. The label is still a
# well-formed short id, so the core's seam guard cannot see it.
P_RELATIONSHIP_FROM_SESSION = (COD,
                               "\t\t\tRelationship:    graph.RelationshipID(rollout.meta.ID),",
                               "\t\t\tRelationship:    graph.RelationshipID(rollout.meta.SessionID),")

# B3: type `source` as the object it is on a subagent thread, and read it
# directly. A user thread's source is the bare string "vscode", so the whole
# record fails to decode. Two sites because they are one authoring choice: the
# field cannot change type without the reader changing with it, and a plant that
# only touched one would not compile, which is a fact about the compiler rather
# than about the assertion.
P_SOURCE_TYPED = (COD,
                  "\tSource json.RawMessage `json:\"source\"`",
                  "\tSource codexSpawnSource `json:\"source\"`")
P_SOURCE_READ_DIRECT = (COD,
                        "\tvar source codexSpawnSource\n\tif err := json.Unmarshal(meta.Source, &source); err != nil {\n\t\treturn \"\"\n\t}\n\treturn source.Subagent.ThreadSpawn.AgentNickname",
                        "\treturn meta.Source.Subagent.ThreadSpawn.AgentNickname")

# B4: synthesize the parent unconditionally, on top of its own rollout.
P_ALWAYS_SYNTHESIZE = (COD,
                       "\t\tif !published[parentID] {\n\t\t\tpublished[parentID] = true",
                       "\t\tif true {\n\t\t\tpublished[parentID] = true")

# B5: give the synthesized parent the CHILD's project.
P_PARENT_INHERITS_PROJECT = (COD,
                             "\t\t\t\tRole:      types.RolePrimary,\n\t\t\t\tLocation:  rollout.path,",
                             "\t\t\t\tRole:      types.RolePrimary,\n\t\t\t\tProject:   join.ProjectName(rollout.meta.CWD),\n\t\t\t\tLocation:  rollout.path,")

# B6: anchor the synthesized parent on the directory rather than the child file.
P_PARENT_ANCHOR_DIR = (COD,
                       "\t\t\t\tRole:      types.RolePrimary,\n\t\t\t\tLocation:  rollout.path,",
                       "\t\t\t\tRole:      types.RolePrimary,\n\t\t\t\tLocation:  filepath.Dir(rollout.path),")

# B7: never synthesize a parent, so a spawn edge names an endpoint nobody
# published and the reconciler drops it (it creates no placeholders).
P_NO_SYNTHESIS = (COD,
                  "\t\tif !published[parentID] {\n\t\t\tpublished[parentID] = true",
                  "\t\tif false {\n\t\t\tpublished[parentID] = true")

# B8: invert the "no parent named" guard, so a thread that names a parent is the
# one skipped and no edge is built at all.
P_PARENT_GUARD_INVERTED = (COD,
                           "\t\tif parent == \"\" {\n\t\t\tcontinue\n\t\t}",
                           "\t\tif parent != \"\" {\n\t\t\tcontinue\n\t\t}")

# C1: build edges from every rollout READ rather than every thread PUBLISHED, so
# a child whose own id was unusable still leaves a synthesized parent behind.
P_SPAWNS_FROM_ALL = (COD,
                     "\tfor _, rollout := range observed {\n\t\tparent := rollout.meta.ParentThreadID",
                     "\tfor _, rollout := range rollouts {\n\t\tparent := rollout.meta.ParentThreadID")

# C2: trust the record without checking whether it decoded. Only the
# wrong-typed-id fixture can see this: encoding/json validates the whole input
# before decoding, so a SYNTAX error populates nothing and the type gate below
# catches it either way. A TYPE error leaves `type` set and slips past.
P_IGNORE_JSON_ERROR = (COD,
                       "\tif err := json.Unmarshal(line, &record); err != nil || record.Type != codexSessionMetaType {",
                       "\tjson.Unmarshal(line, &record)\n\tif record.Type != codexSessionMetaType {")

# C3: accept whatever record happens to be first.
P_ANY_FIRST_RECORD = (COD,
                      "\tif err := json.Unmarshal(line, &record); err != nil || record.Type != codexSessionMetaType {",
                      "\tif err := json.Unmarshal(line, &record); err != nil {")

# C4: raise the first-line cap past the oversized fixture, the edit somebody
# makes when a prompt grows. 128 KiB rather than something enormous: the fixture
# is a fixed 96 KiB, so this is the smallest doubling that actually admits it,
# and a plant that cannot admit the fixture measures nothing.
P_WIDE_LINE_CAP = (COD,
                   "\tcodexMaxFirstLine = 64 << 10",
                   "\tcodexMaxFirstLine = 128 << 10")

# C5: stop counting a session_meta whose id cannot name a node.
P_NO_THREAD_SKIP_COUNT = (COD,
                          "\t\tif err != nil {\n\t\t\ts.skippedThreads.Add(1)\n\t\t\tcontinue\n\t\t}",
                          "\t\tif err != nil {\n\t\t\tcontinue\n\t\t}")

# C6: widen the horizon 100x, so a rollout nobody has touched in hours is
# published again and drags a synthesized parent back with it.
P_WIDE_HORIZON = (COD,
                  "\t\t\tif now.Sub(info.ModTime()) > nativeHorizon {",
                  "\t\t\tif now.Sub(info.ModTime()) > 100*nativeHorizon {")

# D1: widen the walk by three days.
P_WIDE_WALK = (COD,
               "\t\tlocal, local.AddDate(0, 0, -1),\n\t\tnow.UTC(), now.UTC().AddDate(0, 0, -1),",
               "\t\tlocal, local.AddDate(0, 0, -1), local.AddDate(0, 0, -3),\n\t\tnow.UTC(), now.UTC().AddDate(0, 0, -1), now.UTC().AddDate(0, 0, -3),")

# D2: walk only the local pair.
P_LOCAL_ONLY = (COD,
                "\t\tlocal, local.AddDate(0, 0, -1),\n\t\tnow.UTC(), now.UTC().AddDate(0, 0, -1),",
                "\t\tlocal, local.AddDate(0, 0, -1),")

# D3: normalize the local pair to UTC, collapsing the set to the UTC pair --
# which is not where codex writes. Deleting the local entries instead would leave
# `local` declared and unused and fail the build, proving something about Go
# rather than about the assertion.
P_UTC_ONLY = (COD,
              "\t\tlocal, local.AddDate(0, 0, -1),\n\t\tnow.UTC(), now.UTC().AddDate(0, 0, -1),",
              "\t\tlocal.UTC(), local.UTC().AddDate(0, 0, -1),\n\t\tnow.UTC(), now.UTC().AddDate(0, 0, -1),")

# D4: drop the dedup, so the same directory is walked twice whenever the local
# and UTC dates agree, which is most of the day in most zones.
P_NO_DEDUP = (COD,
              "\t\tif seen[dir] {\n\t\t\tcontinue\n\t\t}\n",
              "")

# --- weakened assertions --------------------------------------------------
#
# Every weakening keeps its original condition and its init statement, guarded
# by `false &&`. A weakening that DELETES the condition also deletes the only
# use of some local, and the compile error that follows is a fact about Go's
# unused-variable rule rather than about the assertion: the plant never runs and
# the cycle proves nothing. Deleting the whole block is used only where nothing
# else references what it declares.

W_SHORTID = (CODT,
             '\tif node.SessionID != codexNestedThread {\n\t\tt.Fatalf("codex-node-short-id-is-own-thread-id rule violated: sessionID=%q want=%q id=%s", node.SessionID, codexNestedThread, node.ID)\n\t}\n',
             '')
W_NAME = (CODT,
          "\tif node.Name != codexNestedNick {",
          "\tif false && node.Name != codexNestedNick {")
W_PROJECT = (CODT,
             '\tif node.Project != "aitop" {',
             '\tif false && node.Project != "aitop" {')
# "the location is somewhere on the path" -- true of the file, and equally true
# of the directory the plant substitutes for it.
W_LOCATION_ID = (CODT,
                 "\tif node.Location != path {",
                 "\tif !strings.HasPrefix(path, node.Location) {")
W_NOCLAIM_ID = (CODT,
                '\tif node.State != "" || node.Exit != "" || node.ExitAt != nil {',
                '\tif false && (node.State != "" || node.Exit != "" || node.ExitAt != nil) {')
W_NOCLAIM_COLLECTOR = (CODT,
                       "\tfor _, banned := range []graph.EventKind{graph.EventStateObserved, graph.EventExitObserved, graph.EventHeartbeatObserved} {",
                       "\tfor _, banned := range []graph.EventKind{} {")
W_ROOT_IS_NEVER_NODE = (CODT,
                        "\tif seen := countNodeID(nodes, root); seen != 0 {",
                        "\tif seen := countNodeID(nodes, root); false && seen != 0 {")
W_ID_NODECOUNT = (CODT,
                  "\tif len(nodes) != 2 {\n\t\tt.Fatalf(\"codex-node-count rule violated:",
                  "\tif false && len(nodes) != 2 {\n\t\tt.Fatalf(\"codex-node-count rule violated:")
W_PARENT_PRESENT = (CODT,
                    "\tif _, ok := nodeByID(nodes, mid); !ok {",
                    "\tif _, ok := nodeByID(nodes, mid); false && !ok {")
W_SPAWN_PARENT_ID = (CODT,
                     "\tif spawn.ParentID != mid || spawn.ParentSessionID != codexMidThread {",
                     "\tif false && (spawn.ParentID != mid || spawn.ParentSessionID != codexMidThread) {")
W_RELATIONSHIP_ID = (CODT,
                     "\tif spawn.Relationship != graph.RelationshipID(codexNestedThread) {",
                     "\tif false && spawn.Relationship != graph.RelationshipID(codexNestedThread) {")
W_USER_IDENTITY = (CODT,
                   '\tif userNode.Role != types.RolePrimary || userNode.Name != "" || userNode.Project != "aitop" || userNode.Location != userPath {',
                   '\tif false && (userNode.Role != types.RolePrimary || userNode.Name != "" || userNode.Project != "aitop" || userNode.Location != userPath) {')
W_NOT_DUPLICATED = (CODT,
                    "\tif seen := countNodeID(nodes, user); seen != 1 {",
                    "\tif seen := countNodeID(nodes, user); false && seen != 1 {")
W_ROOT_SYNTHESIZED = (CODT,
                      "\trootNode, ok := nodeByID(nodes, root)\n\tif !ok {",
                      "\trootNode, ok := nodeByID(nodes, root)\n\tif false && !ok {")
W_SPAWN_NODECOUNT = (CODT,
                     "\tif len(nodes) != 4 {",
                     "\tif false && len(nodes) != 4 {")
W_MINIMAL_PARENT = (CODT,
                    '\tif rootNode.Role != types.RolePrimary || rootNode.Name != "" || rootNode.Project != "" {',
                    '\tif false && (rootNode.Role != types.RolePrimary || rootNode.Name != "" || rootNode.Project != "") {')
W_PARENT_ANCHOR = (CODT,
                   "\tif rootNode.SessionID != codexRootThread || rootNode.Location != secondPath {",
                   "\tif false && (rootNode.SessionID != codexRootThread || rootNode.Location != secondPath) {")
W_SKIP_NODECOUNT = (CODT,
                    "\tif len(nodes) != 1 {",
                    "\tif false && len(nodes) != 1 {")
W_SKIP_SPAWNCOUNT = (CODT,
                     "\tif len(spawns) != 0 {",
                     "\tif false && len(spawns) != 0 {")
W_SKIP_ABSENT = (CODT,
                 "\t\tif seen := countNodeID(nodes, absent.id); seen != 0 {",
                 "\t\tif seen := countNodeID(nodes, absent.id); false && seen != 0 {")
W_ROLLOUT_SKIPS = (CODT,
                   "\tif skipped := scanner.skippedRollouts.Load(); skipped != 4 {",
                   "\tif skipped := scanner.skippedRollouts.Load(); false && skipped != 4 {")
W_THREAD_SKIPS = (CODT,
                  "\tif skipped := scanner.skippedThreads.Load(); skipped != 1 {",
                  "\tif skipped := scanner.skippedThreads.Load(); false && skipped != 1 {")
W_DIRS_ABSENT = (CODT,
                 "\t\tif seen := countNodeID(nodes, mustThreadID(t, absent.thread)); seen != 0 {",
                 "\t\tif seen := countNodeID(nodes, mustThreadID(t, absent.thread)); false && seen != 0 {")
# Disambiguated by its own message: two tests assert a node count of 2, and a
# patch that matched both would be a patch aimed at neither.
W_DIRS_NODECOUNT = (CODT,
                    "\tif len(nodes) != 2 {\n\t\tt.Fatalf(\"codex-walk-skips-old-date-dirs rule violated:",
                    "\tif false && len(nodes) != 2 {\n\t\tt.Fatalf(\"codex-walk-skips-old-date-dirs rule violated:")
# Added after the first run of S-X8a: a widened walk moves the helper's entry
# COUNT as well as the fixture's node count, and the two are independent
# witnesses of one rule. Predicting one and meeting the other is the finding.
W_DIRS_BOUNDED = (CODT,
                  "\t\tif len(dirs) < 2 || len(dirs) > 4 {",
                  "\t\tif false && (len(dirs) < 2 || len(dirs) > 4) {")
W_DIRS_COVER = (CODT,
                "\t\t\tif !seen[dir] {",
                "\t\t\tif false && !seen[dir] {")
W_DIRS_DEDUP = (CODT,
                "\t\t\tif seen[dir] {\n\t\t\t\tt.Fatalf(\"codex-date-dirs-deduped rule violated:",
                "\t\t\tif false && seen[dir] {\n\t\t\t\tt.Fatalf(\"codex-date-dirs-deduped rule violated:")
W_COLLECTOR_EDGES = (CODT,
                     "\tif edges := countKind(events, graph.EventRelationshipObserved); edges != 1 {",
                     "\tif edges := countKind(events, graph.EventRelationshipObserved); false && edges != 1 {")
W_COLLECTOR_ORDER = (CODT,
                     "\tif !parentSeen || !childSeen || parentIndex >= edgeIndex || childIndex >= edgeIndex {",
                     "\tif false && (!parentSeen || !childSeen || parentIndex >= edgeIndex || childIndex >= edgeIndex) {")

CYCLES = [
    # --- the named disk trap ------------------------------------------------
    ("S-X1a-prod", [P_NODE_FROM_SESSION], T_ID, "RED", "codex-node-is-own-thread-id rule violated"),
    ("S-X1b-prod", [P_SHORTID_FROM_SESSION], T_ID, "RED", "codex-node-short-id-is-own-thread-id rule violated"),
    ("S-X1b-weak", [P_SHORTID_FROM_SESSION, W_SHORTID], T_ID, "GREEN", None),
    ("S-X2a-prod", [P_PARENT_FROM_SESSION], T_ID, "RED", "codex-session-id-is-never-a-node rule violated"),
    ("S-X2a-weak", [P_PARENT_FROM_SESSION, W_ROOT_IS_NEVER_NODE, W_PARENT_PRESENT, W_SPAWN_PARENT_ID], T_ID, "GREEN", None),
    ("S-X3a-prod", [P_RELATIONSHIP_FROM_SESSION], T_ID, "RED", "codex-relationship-is-own-thread-id rule violated"),
    ("S-X3a-weak", [P_RELATIONSHIP_FROM_SESSION, W_RELATIONSHIP_ID], T_ID, "GREEN", None),
    ("S-X3b-shadow-prod", [P_RELATIONSHIP_FROM_SESSION], T_SHADOW, "RED", "codex-shadow-relationship-is-thread-id rule violated"),

    # --- node content -------------------------------------------------------
    ("S-X4a-prod", [P_NICK_FROM_ROLE], T_ID, "RED", "codex-name-from-spawn-nickname rule violated"),
    ("S-X4a-weak", [P_NICK_FROM_ROLE, W_NAME], T_ID, "GREEN", None),
    ("S-X4b-prod", [P_PROJECT_FROM_PATH], T_ID, "RED", "codex-project-from-cwd rule violated"),
    ("S-X4b-weak", [P_PROJECT_FROM_PATH, W_PROJECT], T_ID, "GREEN", None),
    ("S-X4c-prod", [P_LOCATION_DIR], T_ID, "RED", "codex-location-is-source-file rule violated"),
    ("S-X4c-weak", [P_LOCATION_DIR, W_LOCATION_ID], T_ID, "GREEN", None),

    # --- the v1 limit: no liveness claims -----------------------------------
    ("S-X5a-prod", [P_CLAIMS_ACTIVE], T_ID, "RED", "codex-makes-no-state-or-terminal-claim rule violated"),
    ("S-X5a-weak", [P_CLAIMS_ACTIVE, W_NOCLAIM_ID], T_ID, "GREEN", None),
    ("S-X5b-collector-prod", [P_CLAIMS_ACTIVE], T_CLAIM, "RED", "codex-makes-no-state-or-terminal-claim rule violated"),
    ("S-X5b-collector-weak", [P_CLAIMS_ACTIVE, W_NOCLAIM_COLLECTOR], T_CLAIM, "GREEN", None),

    # --- the two shapes of parent endpoint ----------------------------------
    ("S-X6a-prod", [P_SOURCE_TYPED, P_SOURCE_READ_DIRECT], T_SPAWN, "RED", "codex-user-thread-identity rule violated"),
    ("S-X6a-weak", [P_SOURCE_TYPED, P_SOURCE_READ_DIRECT, W_USER_IDENTITY], T_SPAWN, "GREEN", None),
    ("S-X6b-prod", [P_ALWAYS_SYNTHESIZE], T_SPAWN, "RED", "codex-scanned-parent-not-duplicated rule violated"),
    ("S-X6b-weak", [P_ALWAYS_SYNTHESIZE, W_NOT_DUPLICATED, W_SPAWN_NODECOUNT], T_SPAWN, "GREEN", None),
    ("S-X6c-prod", [P_PARENT_INHERITS_PROJECT], T_SPAWN, "RED", "codex-synthesized-parent-is-minimal rule violated"),
    ("S-X6c-weak", [P_PARENT_INHERITS_PROJECT, W_MINIMAL_PARENT], T_SPAWN, "GREEN", None),
    ("S-X6d-prod", [P_PARENT_ANCHOR_DIR], T_SPAWN, "RED", "codex-synthesized-parent-anchored-on-child-file rule violated"),
    ("S-X6d-weak", [P_PARENT_ANCHOR_DIR, W_PARENT_ANCHOR], T_SPAWN, "GREEN", None),
    ("S-X6e-prod", [P_NO_SYNTHESIS], T_SPAWN, "RED", "codex-unscanned-parent-synthesized rule violated"),
    ("S-X6e-weak", [P_NO_SYNTHESIS, W_ROOT_SYNTHESIZED, W_MINIMAL_PARENT, W_PARENT_ANCHOR, W_SPAWN_NODECOUNT], T_SPAWN, "GREEN", None),
    ("S-X6g-prod", [P_PARENT_GUARD_INVERTED], T_CLAIM, "RED", "codex-collector-publishes-spawn-edge rule violated"),
    ("S-X6g-weak", [P_PARENT_GUARD_INVERTED, W_COLLECTOR_EDGES, W_COLLECTOR_ORDER], T_CLAIM, "GREEN", None),
    ("S-X6f-shadow-prod", [P_NO_SYNTHESIS], T_SHADOW, "RED", "codex-shadow-spawn-edge rule violated"),

    # --- reasons to read nothing, and the rule that binds them --------------
    ("S-X7a-prod", [P_SPAWNS_FROM_ALL], T_SKIP, "RED", "codex-skipped-child-synthesizes-no-parent rule violated"),
    ("S-X7a-weak", [P_SPAWNS_FROM_ALL, W_SKIP_NODECOUNT, W_SKIP_SPAWNCOUNT, W_SKIP_ABSENT], T_SKIP, "GREEN", None),
    ("S-X7b-prod", [P_WIDE_HORIZON], T_SKIP, "RED", "codex-skipped-child-synthesizes-no-parent rule violated"),
    ("S-X7b-weak", [P_WIDE_HORIZON, W_SKIP_NODECOUNT, W_SKIP_SPAWNCOUNT, W_SKIP_ABSENT], T_SKIP, "GREEN", None),
    ("S-X7c-prod", [P_IGNORE_JSON_ERROR], T_SKIP, "RED", "codex-unreadable-rollouts-counted rule violated"),
    ("S-X7c-weak", [P_IGNORE_JSON_ERROR, W_ROLLOUT_SKIPS, W_THREAD_SKIPS], T_SKIP, "GREEN", None),
    ("S-X7d-prod", [P_ANY_FIRST_RECORD], T_SKIP, "RED", "codex-unreadable-rollouts-counted rule violated"),
    ("S-X7d-weak", [P_ANY_FIRST_RECORD, W_ROLLOUT_SKIPS, W_THREAD_SKIPS], T_SKIP, "GREEN", None),
    ("S-X7e-prod", [P_WIDE_LINE_CAP], T_SKIP, "RED", "codex-skipped-child-synthesizes-no-parent rule violated"),
    ("S-X7e-weak", [P_WIDE_LINE_CAP, W_SKIP_NODECOUNT, W_SKIP_SPAWNCOUNT, W_SKIP_ABSENT, W_ROLLOUT_SKIPS], T_SKIP, "GREEN", None),
    ("S-X7f-prod", [P_NO_THREAD_SKIP_COUNT], T_SKIP, "RED", "codex-unusable-thread-ids-counted rule violated"),
    ("S-X7f-weak", [P_NO_THREAD_SKIP_COUNT, W_THREAD_SKIPS], T_SKIP, "GREEN", None),

    # --- the walk bound -----------------------------------------------------
    ("S-X8a-prod", [P_WIDE_WALK], T_DIRS, "RED", "codex-walk-skips-old-date-dirs rule violated"),
    ("S-X8a-weak", [P_WIDE_WALK, W_DIRS_ABSENT, W_DIRS_NODECOUNT, W_DIRS_BOUNDED], T_DIRS, "GREEN", None),
    ("S-X8b-prod", [P_LOCAL_ONLY], T_DIRS, "RED", "codex-date-dirs-cover-utc-and-local rule violated"),
    ("S-X8b-weak", [P_LOCAL_ONLY, W_DIRS_COVER], T_DIRS, "GREEN", None),
    ("S-X8c-prod", [P_UTC_ONLY], T_DIRS, "RED", "codex-date-dirs-cover-utc-and-local rule violated"),
    ("S-X8c-weak", [P_UTC_ONLY, W_DIRS_COVER], T_DIRS, "GREEN", None),
    ("S-X8d-prod", [P_NO_DEDUP], T_DIRS, "RED", "codex-date-dirs-deduped rule violated"),
    ("S-X8d-weak", [P_NO_DEDUP, W_DIRS_DEDUP], T_DIRS, "GREEN", None),
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
        # The log this driver is writing lives in the repo on purpose, so the
        # evidence is re-runnable from a clean checkout rather than from someone's
        # scratch directory. It is therefore the one path that is expected to be
        # dirty while the run is in progress, and the only one excluded here: the
        # check still has to see any plant that failed to restore.
        lines = sh(["git", "status", "--porcelain"]).stdout.strip().splitlines()
        return "\n".join(l for l in lines if "tests/sabotage-native-task3/" not in l).strip()

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
        # whole-tree diffstat would be non-empty whether or not a plant landed,
        # and the check would certify nothing.
        diff = sh(["git", "diff", "--stat", "--"] + [COD, CODT]).stdout.strip()
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
