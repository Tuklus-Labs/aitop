package ui

import (
	"sort"
	"strings"

	"aitop/internal/graph"
	"aitop/internal/types"
)

// maxGraphEdgeLines is how many non-spawn edges one node advertises before the
// pane stops listing them. A chatty node would otherwise bury its own subtree.
const maxGraphEdgeLines = 3

type graphLineKind int

const (
	graphNodeLine graphLineKind = iota
	graphBackLine               // a node already drawn elsewhere in the forest
	graphEdgeLine               // a message or service edge hanging off a node
)

// graphLine is one painted line of the spawn forest after flattening. The view
// draws these and nothing else, so paint stays a pure function of the snapshot.
type graphLine struct {
	kind  graphLineKind
	node  graph.Node
	name  string
	label string // graphEdgeLine only: "msg→ worker"
	depth int
	last  bool // last child of its parent (└ vs ├)
}

// runtimeGroupRank orders the families the way the table groups them: claude,
// grok, codex, local, then everything else.
func runtimeGroupRank(rt types.Runtime) int {
	switch rt {
	case types.RuntimeClaude:
		return 0
	case types.RuntimeGrok:
		return 1
	case types.RuntimeCodex:
		return 2
	case types.RuntimeLocal:
		return 3
	default:
		return 4
	}
}

// graphNodeName is the ProvenName, or the tail of the ID when nothing named it.
func graphNodeName(n graph.Node) string {
	if n.ProvenName != "" {
		return n.ProvenName
	}
	id := string(n.ID)
	if i := strings.LastIndex(id, ":"); i >= 0 && i+1 < len(id) {
		return id[i+1:]
	}
	return id
}

func graphIsGhost(n graph.Node) bool { return n.GhostExpiresAt != nil }

// flattenGraph walks the spawn forest depth first and returns the lines to
// paint. It never trusts the reconciler's cycle rejection: a node is expanded
// at most once, so a fabricated cycle terminates and its second reach renders
// as a back reference. Nodes no spawn edge reaches are roots; nodes left over
// after the roots are walked (every member of a cycle, for instance) are swept
// up as roots of their own, so nothing in the store goes unpainted.
func flattenGraph(g *graph.Snapshot) []graphLine {
	if g == nil || len(g.Nodes) == 0 {
		return nil
	}
	byID := make(map[graph.NodeID]graph.Node, len(g.Nodes))
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}

	kids := map[graph.NodeID][]graph.NodeID{}
	spawned := map[graph.NodeID]bool{}
	side := map[graph.NodeID][]graph.Edge{}
	for _, e := range g.Edges {
		if _, ok := byID[e.Source]; !ok {
			continue
		}
		if _, ok := byID[e.Target]; !ok {
			continue
		}
		switch e.Type {
		case graph.EdgeSpawn, graph.EdgeLaunch:
			spawned[e.Target] = true
			kids[e.Source] = append(kids[e.Source], e.Target)
		case graph.EdgeMessage, graph.EdgeService:
			side[e.Source] = append(side[e.Source], e)
		}
	}

	order := func(ids []graph.NodeID) {
		sort.SliceStable(ids, func(i, j int) bool {
			a, b := byID[ids[i]], byID[ids[j]]
			if ra, rb := runtimeGroupRank(a.Runtime), runtimeGroupRank(b.Runtime); ra != rb {
				return ra < rb
			}
			if na, nb := graphNodeName(a), graphNodeName(b); na != nb {
				return na < nb
			}
			return ids[i] < ids[j]
		})
	}
	for id := range kids {
		order(kids[id])
	}
	for id := range side {
		edges := side[id]
		sort.SliceStable(edges, func(i, j int) bool {
			ni, nj := graphNodeName(byID[edges[i].Target]), graphNodeName(byID[edges[j].Target])
			if ni != nj {
				return ni < nj
			}
			return edges[i].Key < edges[j].Key
		})
	}

	var out []graphLine
	visited := map[graph.NodeID]bool{}
	var walk func(id graph.NodeID, depth int, last bool)
	walk = func(id graph.NodeID, depth int, last bool) {
		n := byID[id]
		if visited[id] {
			out = append(out, graphLine{kind: graphBackLine, name: graphNodeName(n), depth: depth, last: last})
			return
		}
		visited[id] = true
		out = append(out, graphLine{kind: graphNodeLine, node: n, name: graphNodeName(n), depth: depth, last: last})
		for i, e := range side[id] {
			if i >= maxGraphEdgeLines {
				break
			}
			out = append(out, graphLine{
				kind:  graphEdgeLine,
				label: graphEdgeLabel(e) + " " + graphNodeName(byID[e.Target]),
				depth: depth + 1,
			})
		}
		ks := kids[id]
		for i, k := range ks {
			walk(k, depth+1, i == len(ks)-1)
		}
	}

	var roots []graph.NodeID
	for _, n := range g.Nodes {
		if !spawned[n.ID] {
			roots = append(roots, n.ID)
		}
	}
	order(roots)
	for _, id := range roots {
		walk(id, 0, true)
	}
	// Whatever a cycle kept out of the forest still gets drawn.
	var orphans []graph.NodeID
	for _, n := range g.Nodes {
		if !visited[n.ID] {
			orphans = append(orphans, n.ID)
		}
	}
	order(orphans)
	for _, id := range orphans {
		if !visited[id] {
			walk(id, 0, true)
		}
	}
	return out
}

// graphEdgeLabel names the relation an annotation line is reporting. A service
// edge says so: calling it a message would be the pane telling a small lie
// about provenance, which is the one thing this view exists not to do.
func graphEdgeLabel(e graph.Edge) string {
	if e.Type == graph.EdgeService {
		return "svc→"
	}
	return "msg→"
}
