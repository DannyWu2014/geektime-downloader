package note

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/geektime/response"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/filenamify"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
)

const (
	// FolderName sub folder under column dir to save notes
	FolderName = "笔记"
	// MDExtension ...
	MDExtension = ".md"

	typeNote = "note"
)

// newNote is the JSON payload stored in V3Note.NewNote, e.g.
// {"type":"note","list":[{"g":12,"a":29,"c":16,"d":23,"f":"服务降级，来应","h":"note"}]}
// g: paragraph index, c: offset in paragraph
type newNote struct {
	Type string `json:"type"`
	List []struct {
		G int `json:"g"`
		C int `json:"c"`
	} `json:"list"`
}

type item struct {
	text      string
	paragraph int
	offset    int
	ctime     int64
}

// Download fetches notes of one article and writes them to
// <columnDir>/笔记/<article title>.md. A file containing only the title
// is written when the article has no notes.
func Download(ctx context.Context, client *geektime.Client, article geektime.Article, columnDir string) error {
	select {
	case <-ctx.Done():
		return context.Canceled
	default:
	}

	logger.Infof("Begin download article notes, articleID: %d, title: %s", article.AID, article.Title)

	res, err := client.NoteList(article.AID)
	if err != nil {
		return err
	}
	if res.Data.Page.More {
		logger.Warnf("Article notes has more pages, only first page is saved, articleID: %d", article.AID)
	}

	dir := filepath.Join(columnDir, FolderName)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}
	fileName := filepath.Join(dir, filenamify.Filenamify(article.Title)+MDExtension)
	if err := os.WriteFile(fileName, []byte(render(article.Title, res.Data.List)), 0o644); err != nil {
		return err
	}

	logger.Infof("Finish download article notes, articleID: %d, title: %s, count: %d", article.AID, article.Title, len(res.Data.List))
	return nil
}

// render builds markdown content, notes sorted by position in article
func render(title string, notes []response.V3Note) string {
	items := make([]item, 0, len(notes))
	for _, n := range notes {
		items = append(items, toItem(n))
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.paragraph != b.paragraph {
			return a.paragraph < b.paragraph
		}
		if a.offset != b.offset {
			return a.offset < b.offset
		}
		return a.ctime < b.ctime
	})

	var sb strings.Builder
	sb.WriteString("# " + title + "\n")
	if len(items) > 0 {
		sb.WriteString("\n")
	}
	for _, it := range items {
		sb.WriteString("- " + it.text + "\n")
	}
	return sb.String()
}

func toItem(n response.V3Note) item {
	it := item{
		text:      oneLine(n.Part),
		paragraph: int(^uint(0) >> 1), // unknown position goes last
		ctime:     n.Ctime,
	}

	var nn newNote
	isNote := n.Note != ""
	if err := json.Unmarshal([]byte(n.NewNote), &nn); err == nil {
		if nn.Type != "" {
			isNote = nn.Type == typeNote
		}
		if len(nn.List) > 0 {
			it.paragraph = nn.List[0].G
			it.offset = nn.List[0].C
		}
	} else if n.NewNote != "" {
		logger.Warnf("Failed to parse new_note, noteID: %d, err: %v", n.ID, err)
	}

	if isNote {
		it.text = "note: " + oneLine(n.Note)
	}
	return it
}

// oneLine keeps a note inside a single markdown list item
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "<br>")
}
