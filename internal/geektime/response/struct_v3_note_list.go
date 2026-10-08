package response

// V3NoteListResponse is the response of /serv/v3/note/list (underlines and notes of one article)
type V3NoteListResponse struct {
	Code int `json:"code"`
	Data struct {
		Page struct {
			More bool `json:"more"`
		} `json:"page"`
		List []V3Note `json:"list"`
	} `json:"data"`
}

// V3Note one underline or note record
type V3Note struct {
	ID        int    `json:"id"`
	ArticleID int    `json:"article_id"`
	Note      string `json:"note"`
	Part      string `json:"part"`
	NewNote   string `json:"new_note"`
	Ctime     int64  `json:"ctime"`
}
