package checker

import "testing"

func TestParseJSON(t *testing.T) {
	tree, err := parseJSON([]byte(`{"n": 12, "s": "x", "arr": [1, 2]}`))
	if err != nil {
		t.Fatal(err)
	}
	m := tree.(map[string]any)
	if m["n"] != float64(12) {
		t.Fatalf("数字应转为 float64，得到 %T(%v)", m["n"], m["n"])
	}
	if m["s"] != "x" {
		t.Fatalf("s = %v", m["s"])
	}
	if _, ok := m["arr"].([]any); !ok {
		t.Fatalf("arr 应为数组，得到 %T", m["arr"])
	}
}

func TestParseXML(t *testing.T) {
	tree, err := parseXML([]byte(`<root><version>1.2</version></root>`))
	if err != nil {
		t.Fatal(err)
	}
	if v, err := Step(tree, []any{"root", "version", "#text"}); err != nil || v != "1.2" {
		t.Fatalf("version = %v, %v", v, err)
	}
}
