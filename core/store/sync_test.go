package store

import "testing"

func TestSafeRelPath(t *testing.T) {
	base := "/tmp/base"
	cases := map[string]bool{
		"a/b.toml":         true,
		"sub/_source.json": true,
		"a.toml":           true,
		"../x":             false,
		"a/../../x":        false,
		"/abs":             false,
		"":                 false,
		"./":               false,
	}
	for rel, want := range cases {
		if _, got := safeRelPath(base, rel); got != want {
			t.Errorf("safeRelPath(%q) = %v, want %v", rel, got, want)
		}
	}
}
