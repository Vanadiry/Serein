package checker

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func nest(depth int) []byte {
	var b bytes.Buffer
	for i := 0; i < depth; i++ {
		b.WriteString("<a>")
	}
	for i := 0; i < depth; i++ {
		b.WriteString("</a>")
	}
	return b.Bytes()
}

func flat(n int) []byte {
	var b bytes.Buffer
	b.WriteString("<r>")
	for i := 0; i < n; i++ {
		b.WriteString("<a>1</a>")
	}
	b.WriteString("</r>")
	return b.Bytes()
}

// flat(n) 的元素总数是 n+1（n 个子元素 + 根 <r>），根元素也计入配额

// 深度超限必须返回错误，继续递归会 stack overflow
// 实测修复前深度 100 万、7MB 输入会 fatal error: stack overflow，整进程死
func TestParseXMLDepthLimit(t *testing.T) {
	old := xmlMaxDepth
	xmlMaxDepth = 256
	t.Cleanup(func() { xmlMaxDepth = old })

	// 恰好在上限内
	if _, err := parseXML(nest(256)); err != nil {
		t.Errorf("深度 256 应通过: %v", err)
	}
	// 刚超一层
	_, err := parseXML(nest(257))
	if err == nil {
		t.Fatal("深度 257 应被拒绝")
	}
	if !strings.Contains(err.Error(), "嵌套深度") {
		t.Errorf("错误信息应说明是深度: %v", err)
	}
}

// 真实攻击载荷：8MB 以内的深层嵌套必须被挡住，且不 panic
func TestParseXMLRejectsHostileDepth(t *testing.T) {
	body := nest(1_000_000) // ~7MB，在 maxFetchBytes(8MB) 之内
	if len(body) > 8<<20 {
		t.Fatalf("载荷 %d 字节超出 maxFetchBytes，测的不是真实可达输入", len(body))
	}
	if _, err := parseXML(body); err == nil {
		t.Fatal("100 万层嵌套应被拒绝")
	} else {
		t.Logf("拒绝: %v", err)
	}
}

// 节点数超限必须返回错误，8MB 输入放大成 478MB 堆
func TestParseXMLNodeLimit(t *testing.T) {
	old := xmlMaxNodes
	xmlMaxNodes = 1000
	t.Cleanup(func() { xmlMaxNodes = old })

	// flat(999) = 1000 个元素（含根），恰好等于上限
	if _, err := parseXML(flat(999)); err != nil {
		t.Errorf("1000 个元素应通过: %v", err)
	}
	if _, err := parseXML(flat(1000)); err == nil {
		t.Fatal("1001 个元素应被拒绝")
	} else if !strings.Contains(err.Error(), "元素总数") {
		t.Errorf("错误信息应说明是节点数: %v", err)
	}

	// 默认上限下，8MB 载荷也应被挡住
	body := flat(1_000_000) // ~8MB
	if len(body) > 8<<20 {
		t.Fatalf("载荷 %d 字节超出 maxFetchBytes", len(body))
	}
	if _, err := parseXML(body); err == nil {
		t.Fatal("默认上限下 100 万节点应被拒绝")
	}
}

// 真实 feed 形态必须照常解析，不能被上限误伤
func TestParseXMLRealisticFeeds(t *testing.T) {
	cases := map[string]string{
		"RSS 2.0": `<?xml version="1.0"?><rss version="2.0"><channel>
			<title>Blog</title>
			<link>https://example.com</link>
			<item><title>Post 1</title><link>https://example.com/1</link><description>hi</description></item>
			<item><title>Post 2</title><link>https://example.com/2</link><description>hi</description></item>
		</channel></rss>`,
		"Atom": `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom">
			<title>Feed</title>
			<entry><title>E1</title><link href="https://example.com/1"/></entry>
		</feed>`,
		"VSIX manifest": `<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011">
			<Metadata>
				<Identity Language="en-US" Id="a.b" Version="1.0.0" Publisher="p"/>
			</Metadata>
			<Installation><InstallationTarget Id="Microsoft.VisualStudio.Code"/></Installation>
		</PackageManifest>`,
		"OPML 嵌套 outline": `<opml version="2.0"><body>
			<outline text="1"><outline text="2"><outline text="3"><outline text="4"/></outline></outline></outline>
		</body></opml>`,
		"带属性": `<root><item id="7" type="x">v</item></root>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := parseXML([]byte(body))
			if err != nil {
				t.Fatalf("真实 feed 不应被上限误伤: %v", err)
			}
			if got == nil {
				t.Fatal("解析结果为空")
			}
			t.Logf("%s → %T", name, got)
		})
	}
}

// 上限必须可覆盖，且默认常量不变
func TestParseXMLLimitsHaveHeadroom(t *testing.T) {
	// 真实 feed 最深也就 OPML 的几十层
	if xmlMaxDepth < 64 {
		t.Errorf("xmlMaxDepth = %d，对真实 feed（OPML 可达几十层）余量不足", xmlMaxDepth)
	}
	// 节点上限应把单份文档堆占用钉在百 MB 量级
	if xmlMaxNodes > 1000000 {
		t.Errorf("xmlMaxNodes = %d，按实测 ~478 字节/节点会放到 ~478MB", xmlMaxNodes)
	}
	fmt.Fprintf(os.Stderr, "xmlMaxDepth=%d xmlMaxNodes=%d\n", xmlMaxDepth, xmlMaxNodes)
}
