package note

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/geektime/response"
)

func TestRender(t *testing.T) {
	notes := []response.V3Note{
		{
			ID:      16106191,
			Note:    "test",
			Part:    "服务降级，来应",
			NewNote: `{"type":"note","list":[{"g":12,"a":29,"b":"28b130a8","c":16,"d":23,"e":0,"f":"服务降级，来应","h":"note","i":"","ver":"1"}]}`,
			Ctime:   1791426147,
		},
		{
			ID:      16106189,
			Part:    "缓存穿透是指",
			NewNote: `{"type":"line","list":[{"g":40,"a":164,"b":"6f85f6c4","c":0,"d":77,"e":0,"f":"缓存穿透是指","h":"line","i":"","ver":"1"}]}`,
			Ctime:   1791425969,
		},
		{
			ID:      3,
			Part:    "同段更靠前",
			NewNote: `{"type":"line","list":[{"g":12,"c":2}]}`,
		},
	}
	got := render("26 | 缓存异常", notes)
	want := "# 26 | 缓存异常\n\n- 同段更靠前\n- 服务降级，来应\n  - note: test\n- 缓存穿透是指\n"
	if got != want {
		t.Errorf("render() =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderEmpty(t *testing.T) {
	if got := render("标题", nil); got != "# 标题\n" {
		t.Errorf("render() = %q", got)
	}
}

func TestRenderMultilineAndLeadingDigit(t *testing.T) {
	notes := []response.V3Note{
		{
			ID:      1,
			Part:    "第一行\n\n  2. 第二行  \r\n\r\n300毫秒\n1、中文顿号\n1.5倍性能",
			Note:    "1. 笔记第一行\n\n2. 笔记第二行\n10) 括号序号\n普通笔记行",
			NewNote: `{"type":"note","list":[{"g":1,"c":0}]}`,
		},
		{
			ID:      2,
			Part:    "0. 单独数字开头",
			NewNote: `{"type":"line","list":[{"g":2,"c":0}]}`,
		},
		{
			ID:      3,
			Note:    "纯笔记第一行\n2. 纯笔记第二行",
			NewNote: `{"type":"note","list":[{"g":3,"c":0}]}`,
		},
	}
	got := render("测试多行", notes)
	want := "# 测试多行\n\n" +
		"- 第一行\n" +
		"- 2\\. 第二行\n" +
		"- 300毫秒\n" +
		"- 1、中文顿号\n" +
		"- 1.5倍性能\n" +
		"  - note: 1. 笔记第一行\n" +
		"  - 2\\. 笔记第二行\n" +
		"  - 10\\) 括号序号\n" +
		"  - 普通笔记行\n" +
		"- 0\\. 单独数字开头\n" +
		"- note: 纯笔记第一行\n" +
		"- 2\\. 纯笔记第二行\n"
	if got != want {
		t.Errorf("render() =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatListItem(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"1. 第一点", `1\. 第一点`},
		{"10) 第十点", `10\) 第十点`},
		{"1.", `1\.`},
		{"1.5倍", "1.5倍"},
		{"300毫秒", "300毫秒"},
		{"- 子项", `\- 子项`},
		{"* 星号项", `\* 星号项`},
		{"+ 加号项", `\+ 加号项`},
		{"-10", "-10"},
		{"*加粗*", "*加粗*"},
		{"> 引用文本", `\> 引用文本`},
		{">无空格引用", `\>无空格引用`},
		{"# 一级标题", `\# 一级标题`},
		{"### 三级标题", `\### 三级标题`},
		{"#include <stdio.h>", "#include <stdio.h>"},
		{"```go", "\\```go"},
		{"~~~", `\~~~`},
		{"---", `\---`},
		{"***", `\***`},
		{"___", `\___`},
		{"- - -", `\- - -`},
		{"普通文本", "普通文本"},
	}
	for _, tt := range tests {
		if got := formatListItem(tt.in); got != tt.want {
			t.Errorf("formatListItem(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderCourse(t *testing.T) {
	sec1 := renderSection("01 | 第一讲", 2, []response.V3Note{
		{
			ID:      1,
			Part:    "重点内容一",
			Note:    "我的心得",
			NewNote: `{"type":"note","list":[{"g":1,"c":0}]}`,
		},
	})
	sec2 := renderSection("02 | 第二讲", 2, nil)
	sec3 := renderSection("03 | 第三讲", 2, []response.V3Note{
		{
			ID:      2,
			Part:    "重点内容三",
			NewNote: `{"type":"line","list":[{"g":2,"c":0}]}`,
		},
	})

	got := renderCourse("Redis核心技术与实战", []string{sec1, sec2, sec3})
	want := "# Redis核心技术与实战\n\n## 01 | 第一讲\n\n- 重点内容一\n  - note: 我的心得\n\n## 02 | 第二讲\n\n## 03 | 第三讲\n\n- 重点内容三\n"
	if got != want {
		t.Errorf("renderCourse() =\n%q\nwant\n%q", got, want)
	}
}

func TestGetTitlePrefix(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"26 | 缓存异常（下）：如何解决缓存雪崩、击穿、穿透难题？", "26-缓存异常（下）"},
		{"01 | 初识 Redis：它能用来做什么？", "01-初识Redis"},
		{"开篇词 | 为什么要学Redis？", "开篇词-为什么要学R"},
		{"结课测试", "结课测试"},
		{"   ", "img"},
	}
	for _, tt := range tests {
		got := getTitlePrefix(tt.title, 10)
		if got != tt.want {
			t.Errorf("getTitlePrefix(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}

func TestGetImageExt(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://static001.geekbang.org/resource/image/44/11/abc.jpg?x-oss-process=image", ".jpg"},
		{"https://static001.geekbang.org/resource/image/44/11/abc.png", ".png"},
		{"https://static001.geekbang.org/resource/image/44/11/abc", ".jpg"},
	}
	for _, tt := range tests {
		got := getImageExt(tt.url)
		if got != tt.want {
			t.Errorf("getImageExt(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

// TestRenderSampleFile renders a captured API response when NOTE_SAMPLE is set,
// e.g. NOTE_SAMPLE=../../../notelist.json go test ./internal/note -run Sample -v
func TestRenderSampleFile(t *testing.T) {
	path := os.Getenv("NOTE_SAMPLE")
	if path == "" {
		t.Skip("NOTE_SAMPLE not set")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var res response.V3NoteListResponse
	if err := json.Unmarshal(b, &res); err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + render("sample", res.Data.List))
}

func TestDownloadAllWaitsBetweenNoteListAndArticleInfo(t *testing.T) {
	var events []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case geektime.V3NoteListPath:
			events = append(events, "note_list")
			_, _ = w.Write([]byte(`{"code":0,"data":{"page":{"more":false},"list":[{"id":1,"part":"重点","note":"笔记","new_note":"{\"type\":\"note\",\"list\":[{\"g\":1,\"c\":0}]}"}]}}`))
		case geektime.V1ArticlePath:
			events = append(events, "article_info")
			_, _ = w.Write([]byte(`{"code":0,"data":{"article_content":"<p>text</p>"}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := geektime.NewClient(nil)
	client.SetBaseURL(srv.URL)
	client.SetRequestInterval(0, 0)

	course := geektime.Course{
		ID:    1001,
		Title: "测试专栏",
		Articles: []geektime.Article{
			{AID: 1, Title: "01 | 第一讲"},
			{AID: 2, Title: "02 | 第二讲"},
		},
	}

	tmpDir := t.TempDir()
	err := DownloadAll(
		context.Background(),
		client,
		course,
		tmpDir,
		nil,
		func() {
			events = append(events, "wait")
		},
	)
	if err != nil {
		t.Fatalf("DownloadAll failed: %v", err)
	}

	wantEvents := []string{
		"note_list",
		"wait",
		"article_info",
		"wait",
		"note_list",
		"wait",
		"article_info",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Errorf("events = %v, want %v", events, wantEvents)
	}

	outPath := filepath.Join(tmpDir, "测试专栏-笔记.md")
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("expected output file %s to exist: %v", outPath, err)
	}
}
