package note

import (
	"encoding/json"
	"os"
	"testing"

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
	want := "# 26 | 缓存异常\n\n- 同段更靠前\n- note: test\n- 缓存穿透是指\n"
	if got != want {
		t.Errorf("render() =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderEmpty(t *testing.T) {
	if got := render("标题", nil); got != "# 标题\n" {
		t.Errorf("render() = %q", got)
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
