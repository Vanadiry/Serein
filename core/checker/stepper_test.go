package checker

import "testing"

func TestStep(t *testing.T) {
	tree, err := parseJSON([]byte(`{
        "tag": "v1.2",
        "assets": [
            {"name": "app-setup.exe", "url": "u1"},
            {"name": "app-arm64.dmg", "url": "u2"}
        ]
    }`))
	if err != nil {
		t.Fatal(err)
	}

	if v, err := Step(tree, []any{"tag"}); err != nil || v != "v1.2" {
		t.Fatalf("tag = %v, %v", v, err)
	}
	// 数组按 name~正则 筛选
	if v, err := Step(tree, []any{"assets", "name~arm64.*dmg", "url"}); err != nil || v != "u2" {
		t.Fatalf("regex filter = %v, %v", v, err)
	}
	// 数组按 key=值 精确筛选
	if v, err := Step(tree, []any{"assets", "name=app-setup.exe", "url"}); err != nil || v != "u1" {
		t.Fatalf("exact filter = %v, %v", v, err)
	}
	// 负索引
	arr, _ := parseJSON([]byte(`[10, 20, 30]`))
	if v, err := Step(arr, []any{float64(-1)}); err != nil || v != float64(30) {
		t.Fatalf("负索引 = %v, %v", v, err)
	}
	// 越界 / 缺 key
	if _, err := Step(arr, []any{float64(5)}); err == nil {
		t.Fatal("越界应报错")
	}
	if _, err := Step(tree, []any{"nope"}); err == nil {
		t.Fatal("缺 key 应报错")
	}
}

func TestStepMulti(t *testing.T) {
	tree, _ := parseJSON([]byte(`{"a": "1", "b": "2"}`))
	got, err := StepMulti(tree, [][]any{{"a"}, {"b"}}, "-")
	if err != nil || got != "1-2" {
		t.Fatalf("StepMulti = %q, %v", got, err)
	}
}
