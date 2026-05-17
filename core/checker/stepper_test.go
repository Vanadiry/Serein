package checker

import (
	"testing"
)

func TestStepObjectKey(t *testing.T) {
	root := map[string]any{
		"data": map[string]any{"version": "1.0.0"},
	}
	v, err := Step(root, []any{"data", "version"})
	if err != nil {
		t.Fatal(err)
	}
	if v != "1.0.0" {
		t.Fatalf("got %v, want 1.0.0", v)
	}
}

func TestStepArrayIndex(t *testing.T) {
	root := []any{
		map[string]any{"tag_name": "v1.0"},
		map[string]any{"tag_name": "v0.9"},
	}
	v, err := Step(root, []any{0, "tag_name"})
	if err != nil {
		t.Fatal(err)
	}
	if v != "v1.0" {
		t.Fatalf("got %v, want v1.0", v)
	}
}

func TestStepNegativeIndex(t *testing.T) {
	root := []any{map[string]any{"v": "a"}, map[string]any{"v": "b"}, map[string]any{"v": "c"}}
	v, err := Step(root, []any{-1, "v"})
	if err != nil {
		t.Fatal(err)
	}
	if v != "c" {
		t.Fatalf("got %v, want c", v)
	}
}

func TestStepExactFilter(t *testing.T) {
	root := map[string]any{
		"assets": []any{
			map[string]any{"name": "win.exe", "url": "https://w.in"},
			map[string]any{"name": "mac.zip", "url": "https://m.in"},
		},
	}
	v, err := Step(root, []any{"assets", "name=mac.zip", "url"})
	if err != nil {
		t.Fatal(err)
	}
	if v != "https://m.in" {
		t.Fatalf("got %v", v)
	}
}

func TestStepRegexFilter(t *testing.T) {
	root := map[string]any{
		"assets": []any{
			map[string]any{"name": "win.exe", "url": "https://w.in"},
			map[string]any{"name": "mac-arm64.zip", "url": "https://m.in"},
		},
	}
	v, err := Step(root, []any{"assets", "name~mac", "url"})
	if err != nil {
		t.Fatal(err)
	}
	if v != "https://m.in" {
		t.Fatalf("got %v", v)
	}
}

func TestStepFirstMatch(t *testing.T) {
	root := map[string]any{
		"assets": []any{
			map[string]any{"name": "mac.zip", "url": "first"},
			map[string]any{"name": "mac.zip", "url": "second"},
		},
	}
	v, _ := Step(root, []any{"assets", "name=mac.zip", "url"})
	if v != "first" {
		t.Fatalf("got %v, want first", v)
	}
}

func TestStepMultiJoin(t *testing.T) {
	root := map[string]any{
		"v": map[string]any{"major": 26, "minor": 4, "patch": 0},
	}
	s, err := StepMulti(root, [][]any{
		{"v", "major"},
		{"v", "minor"},
		{"v", "patch"},
	}, ".")
	if err != nil {
		t.Fatal(err)
	}
	if s != "26.4.0" {
		t.Fatalf("got %q", s)
	}
}

func TestStepKeyNotFound(t *testing.T) {
	_, err := Step(map[string]any{}, []any{"nope"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStepIndexOutOfRange(t *testing.T) {
	_, err := Step([]any{}, []any{0})
	if err == nil {
		t.Fatal("expected error")
	}
}
