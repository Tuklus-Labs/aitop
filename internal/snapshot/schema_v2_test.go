package snapshot

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const schema2DraftURI = "https://json-schema.org/draft/2020-12/schema"
const jsonschemaModule = "github.com/santhosh-tekuri/jsonschema"

func repositoryRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "env", "GOMOD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("schema-2-repository-root rule violated: go env GOMOD err=%v", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull || !filepath.IsAbs(gomod) {
		t.Fatalf("schema-2-repository-root rule violated: GOMOD=%q abs=%t", gomod, filepath.IsAbs(gomod))
	}
	if _, err := os.Stat(gomod); err != nil {
		t.Fatalf("schema-2-repository-root rule violated: GOMOD=%q stat err=%v", gomod, err)
	}
	return filepath.Dir(gomod)
}

func schema2Path(t *testing.T) string {
	t.Helper()
	return filepath.Join(repositoryRoot(t), "schema", "aitop-v2.schema.json")
}

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(repositoryRoot(t), "internal", "snapshot", "testdata", name)
}

func compileSchemaV2(t *testing.T) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	sch, err := c.Compile(schema2Path(t))
	if err != nil {
		t.Fatalf("schema-2-compile rule violated: path=%s err=%v", schema2Path(t), err)
	}
	return sch
}

func loadJSONValue(t *testing.T, path string) any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("schema-2-load-json rule violated: path=%s err=%v", path, err)
	}
	defer f.Close()
	v, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatalf("schema-2-load-json rule violated: path=%s unmarshal err=%v", path, err)
	}
	return v
}

func loadGoldenObject(t *testing.T, name string) map[string]any {
	t.Helper()
	v := loadJSONValue(t, testdataPath(t, name))
	obj, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("schema-2-golden-object rule violated: name=%s type=%T", name, v)
	}
	return obj
}

func cloneJSON(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("schema-2-clone-json rule violated: err=%v", err)
	}
	cloned, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("schema-2-clone-json rule violated: unmarshal err=%v", err)
	}
	return cloned
}

func TestSchema2UsesDraft202012AndClosedObjects(t *testing.T) {
	raw, err := os.ReadFile(schema2Path(t))
	if err != nil {
		t.Fatalf("schema-2-draft-2020-12-and-closed-objects rule violated: read err=%v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema-2-draft-2020-12-and-closed-objects rule violated: unmarshal err=%v", err)
	}
	if got, _ := schema["$schema"].(string); got != schema2DraftURI {
		t.Fatalf("schema-2-draft-2020-12-and-closed-objects rule violated: $schema=%q want=%q", got, schema2DraftURI)
	}
	var open []string
	walkSchemaObjects("$", schema, func(path string, obj map[string]any) {
		if !isJSONSchemaObjectType(obj) {
			return
		}
		if obj["additionalProperties"] != false {
			open = append(open, path)
		}
	})
	if len(open) != 0 {
		t.Fatalf("schema-2-draft-2020-12-and-closed-objects rule violated: open objects=%v", open)
	}
}

func TestSchema2RequiresExactTopLevelFields(t *testing.T) {
	raw, err := os.ReadFile(schema2Path(t))
	if err != nil {
		t.Fatalf("schema-2-requires-exact-top-level-fields rule violated: read err=%v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema-2-requires-exact-top-level-fields rule violated: unmarshal err=%v", err)
	}
	want := []string{"schema", "at", "host", "rows", "graph"}
	gotReq := stringSlice(schema["required"])
	if !reflect.DeepEqual(gotReq, want) {
		t.Fatalf("schema-2-requires-exact-top-level-fields rule violated: required=%v want=%v", gotReq, want)
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		t.Fatalf("schema-2-requires-exact-top-level-fields rule violated: properties=<nil> want=%v", want)
	}
	var gotProps []string
	for _, name := range want {
		if _, ok := props[name]; !ok {
			t.Fatalf("schema-2-requires-exact-top-level-fields rule violated: missing property=%s keys=%v", name, mapKeys(props))
		}
		gotProps = append(gotProps, name)
	}
	if len(props) != len(want) {
		t.Fatalf("schema-2-requires-exact-top-level-fields rule violated: extra properties keys=%v want=%v", mapKeys(props), want)
	}
	if _, ok := props["host"]; !ok {
		t.Fatalf("schema-2-requires-exact-top-level-fields rule violated: host missing from properties")
	}
	_ = gotProps
}

func TestSchema2RequiresNonNullArrays(t *testing.T) {
	raw, err := os.ReadFile(schema2Path(t))
	if err != nil {
		t.Fatalf("schema-2-requires-non-null-arrays rule violated: read err=%v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema-2-requires-non-null-arrays rule violated: unmarshal err=%v", err)
	}
	for _, tc := range []struct {
		name string
		path []string
	}{
		{"rows", []string{"properties", "rows"}},
		{"nodes", []string{"$defs", "dumpGraphV2", "properties", "nodes"}},
		{"edges", []string{"$defs", "dumpGraphV2", "properties", "edges"}},
		{"gaps", []string{"$defs", "dumpGraphV2", "properties", "gaps"}},
	} {
		node := lookupSchema(schema, tc.path...)
		if typeOf(node) != "array" {
			t.Fatalf("schema-2-requires-non-null-arrays rule violated: field=%s type=%v want=array", tc.name, node["type"])
		}
	}
	sch := compileSchemaV2(t)
	doc := loadGoldenObject(t, "schema2-empty.json")
	graph, _ := doc["graph"].(map[string]any)
	for _, field := range []string{"nodes", "edges", "gaps"} {
		mut := cloneJSON(t, doc).(map[string]any)
		g := cloneJSON(t, graph).(map[string]any)
		g[field] = nil
		mut["graph"] = g
		if err := sch.Validate(mut); err == nil {
			t.Fatalf("schema-2-requires-non-null-arrays rule violated: field=%s null validated", field)
		}
	}
	mutRows := cloneJSON(t, doc).(map[string]any)
	mutRows["rows"] = nil
	if err := sch.Validate(mutRows); err == nil {
		t.Fatalf("schema-2-requires-non-null-arrays rule violated: field=rows null validated")
	}
	var emitted map[string]any
	if err := json.Unmarshal(mustWriteJSON(t, schema2EmptySnapshot()), &emitted); err != nil {
		t.Fatalf("schema-2-requires-non-null-arrays rule violated: write unmarshal err=%v", err)
	}
	graphObj, _ := emitted["graph"].(map[string]any)
	for _, field := range []string{"nodes", "edges", "gaps"} {
		arr, ok := graphObj[field].([]any)
		if !ok || arr == nil {
			t.Fatalf("schema-2-requires-non-null-arrays rule violated: emitted field=%s value=%v", field, graphObj[field])
		}
	}
}

func TestSchema2ValidatesEmptyGolden(t *testing.T) {
	sch := compileSchemaV2(t)
	doc := loadJSONValue(t, testdataPath(t, "schema2-empty.json"))
	if err := sch.Validate(doc); err != nil {
		t.Fatalf("schema-2-validates-empty-golden rule violated: err=%v", err)
	}
	obj, _ := doc.(map[string]any)
	graph, _ := obj["graph"].(map[string]any)
	if _, ok := graph["at"]; !ok {
		t.Fatalf("schema-2-validates-empty-golden rule violated: graph.at missing keys=%v", mapKeys(graph))
	}
	written, err := jsonschema.UnmarshalJSON(bytes.NewReader(mustWriteJSON(t, schema2EmptySnapshot())))
	if err != nil {
		t.Fatalf("schema-2-validates-empty-golden rule violated: write unmarshal err=%v", err)
	}
	if err := sch.Validate(written); err != nil {
		t.Fatalf("schema-2-validates-empty-golden rule violated: write json err=%v", err)
	}
}

func TestSchema2ValidatesFullGolden(t *testing.T) {
	sch := compileSchemaV2(t)
	doc := loadJSONValue(t, testdataPath(t, "schema2-full.json"))
	if err := sch.Validate(doc); err != nil {
		t.Fatalf("schema-2-validates-full-golden rule violated: err=%v", err)
	}
	written, err := jsonschema.UnmarshalJSON(bytes.NewReader(mustWriteJSON(t, schema2FullSnapshot())))
	if err != nil {
		t.Fatalf("schema-2-validates-full-golden rule violated: write unmarshal err=%v", err)
	}
	if err := sch.Validate(written); err != nil {
		t.Fatalf("schema-2-validates-full-golden rule violated: write json err=%v", err)
	}
}

func TestSchema2RejectsUnknownAndNullProperties(t *testing.T) {
	sch := compileSchemaV2(t)
	base := loadGoldenObject(t, "schema2-empty.json")
	unknown := cloneJSON(t, base).(map[string]any)
	unknown["canary"] = "aitop-canary"
	if err := sch.Validate(unknown); err == nil {
		t.Fatalf("schema-2-rejects-unknown-and-null-properties rule violated: unknown canary validated")
	}
	nullHost := cloneJSON(t, base).(map[string]any)
	nullHost["host"] = nil
	if err := sch.Validate(nullHost); err == nil {
		t.Fatalf("schema-2-rejects-unknown-and-null-properties rule violated: null host validated")
	}
	full := loadGoldenObject(t, "schema2-full.json")
	nullState := cloneJSON(t, full).(map[string]any)
	graph, _ := nullState["graph"].(map[string]any)
	nodes, _ := graph["nodes"].([]any)
	node, _ := nodes[0].(map[string]any)
	node["proven_name"] = nil
	nodes[0] = node
	graph["nodes"] = nodes
	nullState["graph"] = graph
	if err := sch.Validate(nullState); err == nil {
		t.Fatalf("schema-2-rejects-unknown-and-null-properties rule violated: null proven_name validated")
	}
}

func TestSchema1FixtureContainsActualCanary(t *testing.T) {
	raw, err := os.ReadFile(testdataPath(t, "schema1-control.json"))
	if err != nil {
		t.Fatalf("schema-1-fixture-contains-actual-canary rule violated: read err=%v", err)
	}
	if !bytes.Contains(raw, []byte("aitop-canary")) {
		t.Fatalf("schema-1-fixture-contains-actual-canary rule violated: token=aitop-canary missing bytes=%d", len(raw))
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("schema-1-fixture-contains-actual-canary rule violated: unmarshal err=%v", err)
	}
	if got, _ := doc["canary"].(string); got != "aitop-canary" {
		t.Fatalf("schema-1-fixture-contains-actual-canary rule violated: canary=%q want=aitop-canary", got)
	}
}

func TestSchema2OmitsCanary(t *testing.T) {
	for _, name := range []string{"schema2-empty.json", "schema2-full.json"} {
		raw, err := os.ReadFile(testdataPath(t, name))
		if err != nil {
			t.Fatalf("schema-2-omits-canary rule violated: name=%s read err=%v", name, err)
		}
		if bytes.Contains(raw, []byte("canary")) || bytes.Contains(raw, []byte("aitop-canary")) {
			t.Fatalf("schema-2-omits-canary rule violated: name=%s still contains canary", name)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("schema-2-omits-canary rule violated: name=%s unmarshal err=%v", name, err)
		}
		if _, ok := doc["canary"]; ok {
			t.Fatalf("schema-2-omits-canary rule violated: name=%s has canary key value=%v", name, doc["canary"])
		}
	}
}

func TestStateSchemaEncodesConditionalRules(t *testing.T) {
	sch := compileSchemaV2(t)
	full := loadGoldenObject(t, "schema2-full.json")
	cases := []struct {
		name string
		idx  int
		mut  func(map[string]any)
	}{
		{"source-without-since", 0, func(st map[string]any) {
			delete(st, "since")
			delete(st, "valid_until")
		}},
		{"since-without-source", 0, func(st map[string]any) {
			delete(st, "source")
			delete(st, "valid_until")
		}},
		{"valid-until-without-pair", 0, func(st map[string]any) {
			delete(st, "source")
			delete(st, "since")
			st["valid_until"] = "2026-08-26T12:15:00Z"
		}},
		{"completed-without-pair", 1, func(st map[string]any) {
			delete(st, "source")
			delete(st, "since")
		}},
		{"failed-without-pair", 2, func(st map[string]any) {
			delete(st, "source")
			delete(st, "since")
		}},
		{"vanished-without-pair", 3, func(st map[string]any) {
			delete(st, "source")
			delete(st, "since")
		}},
		{"completed-with-valid-until", 1, func(st map[string]any) {
			st["valid_until"] = "2026-08-26T12:15:00Z"
		}},
		{"failed-with-valid-until", 2, func(st map[string]any) {
			st["valid_until"] = "2026-08-26T12:15:00Z"
		}},
		{"vanished-with-valid-until", 3, func(st map[string]any) {
			st["valid_until"] = "2026-08-26T12:15:00Z"
		}},
		{"approval-with-valid-until", 0, func(st map[string]any) { st["value"] = "approval" }},
		{"blocked-with-valid-until", 0, func(st map[string]any) { st["value"] = "blocked" }},
		{"passive-with-valid-until", 0, func(st map[string]any) {
			src, _ := st["source"].(map[string]any)
			src["authority"] = "passive"
			st["source"] = src
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := cloneJSON(t, full).(map[string]any)
			st := nodeState(t, doc, tc.idx)
			tc.mut(st)
			setNodeState(t, doc, tc.idx, st)
			if err := sch.Validate(doc); err == nil {
				t.Fatalf("state-schema-encodes-conditional-rules rule violated: case=%s validated", tc.name)
			}
		})
	}
}

func TestSchemaValidatorAbsentFromProductionDependencies(t *testing.T) {
	root := repositoryRoot(t)
	cmd := exec.Command("go", "list", "-deps", "./cmd/aitop")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("schema-validator-absent-from-production-dependencies rule violated: go list err=%v", err)
	}
	if bytes.Contains(out, []byte(jsonschemaModule)) {
		t.Fatalf("schema-validator-absent-from-production-dependencies rule violated: module=%s present in go list -deps ./cmd/aitop dir=%s", jsonschemaModule, root)
	}
}

func TestSchemaValidatorImportedOnlyByTests(t *testing.T) {
	root := repositoryRoot(t)
	var hits []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		name := info.Name()
		if info.IsDir() {
			switch name {
			case ".git", "vendor", ".worktrees":
				return filepath.SkipDir
			}
			if name == "pkg" && strings.Contains(path, string(filepath.Separator)+"mod") {
				return filepath.SkipDir
			}
			if strings.Contains(path, string(filepath.Separator)+"pkg"+string(filepath.Separator)+"mod") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			if strings.Contains(imp.Path.Value, jsonschemaModule) {
				rel, _ := filepath.Rel(root, path)
				hits = append(hits, rel+" "+imp.Path.Value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("schema-validator-imported-only-by-tests rule violated: walk err=%v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("schema-validator-imported-only-by-tests rule violated: production imports=%v", hits)
	}
}

func walkSchemaObjects(path string, v any, fn func(string, map[string]any)) {
	obj, ok := v.(map[string]any)
	if !ok {
		if arr, ok := v.([]any); ok {
			for i, item := range arr {
				walkSchemaObjects(path+"/"+itoa(i), item, fn)
			}
		}
		return
	}
	fn(path, obj)
	for k, child := range obj {
		walkSchemaObjects(path+"/"+k, child, fn)
	}
}

func isJSONSchemaObjectType(obj map[string]any) bool {
	switch typ := obj["type"].(type) {
	case string:
		return typ == "object"
	case []any:
		for _, item := range typ {
			if s, ok := item.(string); ok && s == "object" {
				return true
			}
		}
	}
	return false
}

func stringSlice(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func lookupSchema(root map[string]any, path ...string) map[string]any {
	cur := any(root)
	for _, key := range path {
		obj, _ := cur.(map[string]any)
		cur = obj[key]
	}
	obj, _ := cur.(map[string]any)
	return obj
}

func typeOf(node map[string]any) string {
	s, _ := node["type"].(string)
	return s
}

func nodeState(t *testing.T, doc map[string]any, idx int) map[string]any {
	t.Helper()
	graph, _ := doc["graph"].(map[string]any)
	nodes, _ := graph["nodes"].([]any)
	if idx < 0 || idx >= len(nodes) {
		t.Fatalf("state-schema-encodes-conditional-rules rule violated: idx=%d nodes=%d", idx, len(nodes))
	}
	node, _ := nodes[idx].(map[string]any)
	st, _ := node["state"].(map[string]any)
	if st == nil {
		t.Fatalf("state-schema-encodes-conditional-rules rule violated: idx=%d state missing", idx)
	}
	return st
}

func setNodeState(t *testing.T, doc map[string]any, idx int, st map[string]any) {
	t.Helper()
	graph, _ := doc["graph"].(map[string]any)
	nodes, _ := graph["nodes"].([]any)
	node, _ := nodes[idx].(map[string]any)
	node["state"] = st
	nodes[idx] = node
	graph["nodes"] = nodes
	doc["graph"] = graph
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
