package checker

import (
	"testing"
)

func TestParseJSON_Object(t *testing.T) {
	root, err := parseJSON([]byte(`{"data":{"version":"1.0.0"}}`))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := Step(root, []any{"data", "version"})
	if v != "1.0.0" {
		t.Fatalf("got %v", v)
	}
}

func TestParseJSON_Array(t *testing.T) {
	root, err := parseJSON([]byte(`[{"tag":"v1"},{"tag":"v2"}]`))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := Step(root, []any{0, "tag"})
	if v != "v1" {
		t.Fatalf("got %v", v)
	}
}

func TestParseXML_Basic(t *testing.T) {
	root, err := parseXML([]byte(`<root><version>1.0</version></root>`))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := Step(root, []any{"root", "version"})
	s := toString(v)
	if s != "1.0" {
		t.Fatalf("got %q", s)
	}
}

func TestParseXML_Attribute(t *testing.T) {
	root, err := parseXML([]byte(`<r><item url="https://ex.com"/></r>`))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := Step(root, []any{"r", "item", "-url"})
	if v != "https://ex.com" {
		t.Fatalf("got %v", v)
	}
}

func TestParseXML_Nested(t *testing.T) {
	root, err := parseXML([]byte(`<r><ch><item><name>A</name><url>u</url></item></ch></r>`))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := Step(root, []any{"r", "ch", "item", "url"})
	s := toString(v)
	if s != "u" {
		t.Fatalf("got %q", s)
	}
}

func TestRegexMatch(t *testing.T) {
	body := `<b>1.5.3.170</b>`
	v, err := matchRegex(body, `<b>(\d+\.\d+\.\d+\.\d+)</b>`)
	if err != nil {
		t.Fatal(err)
	}
	if v != "1.5.3.170" {
		t.Fatalf("got %q", v)
	}
}

func TestRegexNoMatch(t *testing.T) {
	_, err := matchRegex("no match", `(\d+)`)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractJSONValue(t *testing.T) {
	body := []byte(`{"data":{"version":"2.0"}}`)
	v, err := extractJSONValue(body, []any{"data", "version"}, "")
	if err != nil {
		t.Fatal(err)
	}
	s := toString(v)
	if s != "2.0" {
		t.Fatalf("got %q", s)
	}
}

func TestExtractXMLValue(t *testing.T) {
	body := []byte(`<r><v>3.0</v></r>`)
	v, err := extractXMLValue(body, []any{"r", "v"}, "")
	if err != nil {
		t.Fatal(err)
	}
	s := toString(v)
	if s != "3.0" {
		t.Fatalf("got %q", s)
	}
}

func TestExtractRegexValue(t *testing.T) {
	body := []byte("version: 4.0")
	v, err := extractRegexValue(body, `(\d+\.\d+)`)
	if err != nil {
		t.Fatal(err)
	}
	if v != "4.0" {
		t.Fatalf("got %v", v)
	}
}

func TestExtractSelectorValue(t *testing.T) {
	body := []byte(`<html><span class="v">5.0</span><a class="dl" href="/x.zip">DL</a></html>`)
	v, err := extractSelectorValue(body, map[string]any{"selector": ".v"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if v != "5.0" {
		t.Fatalf("got %v", v)
	}
}

func TestExtractSelectorAttr(t *testing.T) {
	body := []byte(`<html><a class="dl" href="/x.zip">DL</a></html>`)
	v, err := extractSelectorValue(body, map[string]any{"selector": ".dl", "attr": "href"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if v != "/x.zip" {
		t.Fatalf("got %v", v)
	}
}

func TestExtractSelectorRegex(t *testing.T) {
	body := []byte(`<html><span class="v">Version 6.0.1</span></html>`)
	v, err := extractSelectorValue(body, map[string]any{"selector": ".v", "regex": `(\d+\.\d+\.\d+)`}, "")
	if err != nil {
		t.Fatal(err)
	}
	if v != "6.0.1" {
		t.Fatalf("got %v", v)
	}
}

func TestExtractXPathValue(t *testing.T) {
	body := []byte(`<html><span class="v">7.0</span><a class="dl" href="/x">DL</a></html>`)
	v, err := extractXPathValue(body, "span.v/text()", "")
	if err != nil {
		t.Fatal(err)
	}
	if v != "7.0" {
		t.Fatalf("got %v", v)
	}
}

func TestExtractXPathAttr(t *testing.T) {
	body := []byte(`<html><a class="dl" href="/x">DL</a></html>`)
	v, err := extractXPathValue(body, ".dl/@href", "")
	if err != nil {
		t.Fatal(err)
	}
	if v != "/x" {
		t.Fatalf("got %v", v)
	}
}
