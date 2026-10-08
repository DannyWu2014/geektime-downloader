package note

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nicoxiang/geektime-downloader/internal/geektime"
	"github.com/nicoxiang/geektime-downloader/internal/geektime/response"
	"github.com/nicoxiang/geektime-downloader/internal/markdown"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/downloader"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/filenamify"
	"github.com/nicoxiang/geektime-downloader/internal/pkg/logger"
)

const (
	// FolderName sub folder under column dir to save notes
	FolderName = "笔记"
	// ImagesFolderName sub folder under column dir to save images
	ImagesFolderName = "配图"
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
	part      string
	note      string
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

// DownloadAll fetches notes of all articles in the course, merges them into
// <columnDir>/<course title>-笔记.md, and downloads article illustrations into <columnDir>/配图/.
func DownloadAll(
	ctx context.Context,
	client *geektime.Client,
	course geektime.Course,
	columnDir string,
	onProgress func(current, total int, articleTitle string),
	waitFn func(),
) error {
	select {
	case <-ctx.Done():
		return context.Canceled
	default:
	}

	logger.Infof("Begin download all course notes and images, courseID: %d, title: %s, total articles: %d",
		course.ID, course.Title, len(course.Articles))

	imagesDir := filepath.Join(columnDir, ImagesFolderName)
	sections := make([]string, 0, len(course.Articles))
	for i, article := range course.Articles {
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}

		if onProgress != nil {
			onProgress(i+1, len(course.Articles), article.Title)
		}

		res, err := client.NoteList(article.AID)
		if err != nil {
			return err
		}
		if res.Data.Page.More {
			logger.Warnf("Article notes has more pages, only first page is saved, articleID: %d", article.AID)
		}

		sec := renderSection(article.Title, 2, res.Data.List)
		sections = append(sections, sec)

		// Download article illustrations directly into imagesDir
		if err := downloadArticleImages(ctx, client, article.AID, article.Title, imagesDir); err != nil {
			return err
		}

		if i < len(course.Articles)-1 && waitFn != nil {
			waitFn()
		}
	}

	content := renderCourse(course.Title, sections)
	fileName := filepath.Join(columnDir, filenamify.Filenamify(course.Title)+"-笔记"+MDExtension)
	if err := os.WriteFile(fileName, []byte(content), 0o644); err != nil {
		return err
	}

	logger.Infof("Finish download all course notes and images, courseID: %d, title: %s", course.ID, course.Title)
	return nil
}

func downloadArticleImages(
	ctx context.Context,
	client *geektime.Client,
	articleID int,
	articleTitle string,
	imagesDir string,
) error {
	articleInfo, err := client.V1ArticleInfo(articleID)
	if err != nil {
		return err
	}
	if articleInfo.Data.ArticleContent == "" {
		return nil
	}

	imageURLs, err := markdown.ExtractImageURLs(articleInfo.Data.ArticleContent)
	if err != nil {
		logger.Warnf("Failed to extract image URLs from article, articleID: %d, err: %v", articleID, err)
		return nil
	}
	if len(imageURLs) == 0 {
		return nil
	}

	if err := os.MkdirAll(imagesDir, os.ModePerm); err != nil {
		return err
	}

	prefix := getTitlePrefix(articleTitle, 10)
	headers := make(map[string]string, 2)
	headers[geektime.Origin] = geektime.DefaultBaseURL
	headers[geektime.UserAgent] = geektime.DefaultUserAgent

	seen := make(map[string]bool)
	imgIndex := 1
	for _, imgURL := range imageURLs {
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}
		if seen[imgURL] {
			continue
		}
		seen[imgURL] = true

		ext := getImageExt(imgURL)
		fileName := fmt.Sprintf("%s_%02d%s", prefix, imgIndex, ext)
		targetPath := filepath.Join(imagesDir, fileName)

		if _, err := downloader.DownloadFileConcurrently(ctx, targetPath, imgURL, headers, 1); err != nil {
			logger.Warnf("Failed to download image %s for articleID %d: %v", imgURL, articleID, err)
		}
		imgIndex++
	}
	return nil
}

// getTitlePrefix extracts up to maxRunes safe characters from title to use as image file prefix
func getTitlePrefix(title string, maxRunes int) string {
	cleaned := filenamify.Filenamify(title)
	runes := []rune(cleaned)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	prefix := strings.Trim(string(runes), "-_ ")
	if prefix == "" {
		prefix = "img"
	}
	return prefix
}

func getImageExt(imageURL string) string {
	parsedURL, err := url.ParseRequestURI(imageURL)
	if err != nil {
		return ".jpg"
	}
	ext := strings.ToLower(path.Ext(parsedURL.Path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".tiff":
		return ext
	default:
		return ".jpg"
	}
}

// render builds single article markdown content
func render(title string, notes []response.V3Note) string {
	return renderSection(title, 1, notes)
}

// renderCourse merges all article sections under a course title
func renderCourse(courseTitle string, articleSections []string) string {
	var sb strings.Builder
	sb.WriteString("# " + courseTitle + "\n\n")
	for i, sec := range articleSections {
		sb.WriteString(sec)
		if i < len(articleSections)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// renderSection builds markdown for an article at the specified heading level
func renderSection(title string, level int, notes []response.V3Note) string {
	items := make([]item, 0, len(notes))
	for _, n := range notes {
		if it, ok := toItem(n); ok {
			items = append(items, it)
		}
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
	prefix := strings.Repeat("#", level)
	sb.WriteString(prefix + " " + title + "\n")
	if len(items) > 0 {
		sb.WriteString("\n")
		for _, it := range items {
			if it.part != "" {
				sb.WriteString("- " + it.part + "\n")
				if it.note != "" {
					sb.WriteString("  - note: " + it.note + "\n")
				}
			} else if it.note != "" {
				sb.WriteString("- note: " + it.note + "\n")
			}
		}
	}
	return sb.String()
}

func toItem(n response.V3Note) (item, bool) {
	paragraph := int(^uint(0) >> 1) // unknown position goes last
	offset := 0

	var nn newNote
	isNote := n.Note != ""
	if err := json.Unmarshal([]byte(n.NewNote), &nn); err == nil {
		if nn.Type != "" {
			isNote = nn.Type == typeNote
		}
		if len(nn.List) > 0 {
			paragraph = nn.List[0].G
			offset = nn.List[0].C
		}
	} else if n.NewNote != "" {
		logger.Warnf("Failed to parse new_note, noteID: %d, err: %v", n.ID, err)
	}

	partText := oneLine(n.Part)
	var noteText string
	if isNote && strings.TrimSpace(n.Note) != "" {
		noteText = oneLine(n.Note)
	}

	if partText == "" && noteText == "" {
		return item{}, false
	}

	return item{
		part:      partText,
		note:      noteText,
		paragraph: paragraph,
		offset:    offset,
		ctime:     n.Ctime,
	}, true
}

// oneLine keeps a note inside a single markdown list item
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "<br>")
}
