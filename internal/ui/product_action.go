package ui

import (
	"fmt"

	"github.com/manifoldco/promptui"
	"github.com/nicoxiang/geektime-downloader/internal/geektime"
)

const (
	ActionReSelectCourse = iota
	ActionDownloadAll
	ActionDownloadAllNotes
	ActionSelectArticle
	ActionSelectArticleNote
)

type articleOpsOption struct {
	Text  string
	Value int
}

func ProductAction(product geektime.Course) (int, error) {
	options := make([]articleOpsOption, 3)
	options[0] = articleOpsOption{"重新选择课程", ActionReSelectCourse}
	if geektime.IsTextCourse(product) {
		options[1] = articleOpsOption{"下载当前专栏所有文章", ActionDownloadAll}
		options[2] = articleOpsOption{"下载当前专栏所有笔记", ActionDownloadAllNotes}
		options = append(options, articleOpsOption{"选择文章", ActionSelectArticle})
		options = append(options, articleOpsOption{"选择文章下载笔记", ActionSelectArticleNote})
	} else {
		options[1] = articleOpsOption{"下载所有视频", ActionDownloadAll}
		options[2] = articleOpsOption{"选择视频", ActionSelectArticle}
	}
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "{{ `>` | red }} {{ .Text | red }}",
		Inactive: "{{if eq .Value 0}} {{ .Text | green }} {{else}} {{ .Text }} {{end}}",
	}
	prompt := promptui.Select{
		Label:        fmt.Sprintf("当前选中的专栏为: %s, 请继续选择：", product.Title),
		Items:        options,
		Templates:    templates,
		Size:         len(options),
		HideSelected: true,
		Stdout:       NoBellStdout,
	}
	index, _, err := prompt.Run()
	if err != nil {
		return 0, err
	}
	return options[index].Value, nil
}
