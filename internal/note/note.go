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
	parts     []string
	notes     []string
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

		if sec := renderSection(article.Title, 2, res.Data.List, false); sec != "" {
			sections = append(sections, sec)
		}

		// Space out NoteList and V1ArticleInfo so each article does not fire two API calls back-to-back
		if waitFn != nil {
			waitFn()
		}

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
	select {
	case <-ctx.Done():
		return context.Canceled
	default:
	}

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
	return renderSection(title, 1, notes, true)
}

// renderCourse merges all non-empty article sections under a course title
func renderCourse(courseTitle string, articleSections []string) string {
	nonEmpty := make([]string, 0, len(articleSections))
	for _, sec := range articleSections {
		if sec != "" {
			nonEmpty = append(nonEmpty, sec)
		}
	}

	var sb strings.Builder
	sb.WriteString("# " + courseTitle + "\n")
	if len(nonEmpty) == 0 {
		return sb.String()
	}
	sb.WriteString("\n")
	for i, sec := range nonEmpty {
		sb.WriteString(sec)
		if i < len(nonEmpty)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// renderSection builds markdown for an article at the specified heading level.
// When includeEmpty is false, articles with no renderable notes return an empty string.
func renderSection(title string, level int, notes []response.V3Note, includeEmpty bool) string {
	items := make([]item, 0, len(notes))
	for _, n := range notes {
		if it, ok := toItem(n); ok {
			items = append(items, it)
		}
	}
	if len(items) == 0 && !includeEmpty {
		return ""
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
			if len(it.parts) > 0 {
				for _, line := range it.parts {
					sb.WriteString("- " + formatListItem(line) + "\n")
				}
				writeNoteItems(&sb, "  ", it.notes)
			} else if len(it.notes) > 0 {
				writeNoteItems(&sb, "", it.notes)
			}
		}
	}
	return sb.String()
}

func writeNoteItems(sb *strings.Builder, indent string, notes []string) {
	if len(notes) == 1 {
		sb.WriteString(indent + "- note: " + notes[0] + "\n")
		return
	}
	if len(notes) > 1 {
		sb.WriteString(indent + "- note\n")
		for _, line := range notes {
			sb.WriteString(indent + "  - " + formatListItem(line) + "\n")
		}
	}
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

	partLines := splitLines(n.Part)
	var noteLines []string
	if isNote {
		noteLines = splitLines(n.Note)
	}

	if len(partLines) == 0 && len(noteLines) == 0 {
		return item{}, false
	}

	return item{
		parts:     partLines,
		notes:     noteLines,
		paragraph: paragraph,
		offset:    offset,
		ctime:     n.Ctime,
	}, true
}

// splitLines splits text into trimmed non-empty lines so each line can be
// rendered as its own markdown list item.
func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	raw := strings.Split(s, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// formatListItem escapes leading CommonMark block/list markers so ProcessOn
// renders the line cleanly as plain text inside a single list item instead of
// misparsing it as a nested list, heading, blockquote, thematic break, or code fence.
func formatListItem(s string) string {
	if s == "" {
		return s
	}

	// Ordered list marker: digits followed by '.' or ')' and space/tab/EOL
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 && i < len(s) && (s[i] == '.' || s[i] == ')') {
		if i+1 == len(s) || s[i+1] == ' ' || s[i+1] == '\t' {
			return s[:i] + `\` + s[i:]
		}
	}

	// Blockquote marker: '>' (CommonMark does not require a trailing space)
	if s[0] == '>' {
		return `\` + s
	}

	// Unordered list marker: '-', '*', '+' followed by space/tab/EOL
	if s[0] == '-' || s[0] == '*' || s[0] == '+' {
		if len(s) == 1 || s[1] == ' ' || s[1] == '\t' {
			return `\` + s
		}
	}

	// ATX heading marker: 1-6 '#' followed by space/tab/EOL
	if s[0] == '#' {
		j := 0
		for j < len(s) && j < 6 && s[j] == '#' {
			j++
		}
		if j == len(s) || s[j] == ' ' || s[j] == '\t' {
			return `\` + s
		}
	}

	// Fenced code block: 3+ '`' or '~'
	if strings.HasPrefix(s, "```") || strings.HasPrefix(s, "~~~") {
		return `\` + s
	}

	// Thematic break: 3+ '-', '*', or '_' (ignoring spaces/tabs)
	if isThematicBreak(s) {
		return `\` + s
	}

	return s
}

func isThematicBreak(s string) bool {
	if s[0] != '-' && s[0] != '*' && s[0] != '_' {
		return false
	}
	marker := s[0]
	count := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case marker:
			count++
		case ' ', '\t':
		default:
			return false
		}
	}
	return count >= 3
}
