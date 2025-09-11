package model

type ReviewComment struct {
	Body     string `json:"body"`
	Path     string `json:"path"`
	Position int    `json:"position"`
	Anchor   string `json:"anchor,omitempty"`
}
type ReviewResponse struct {
	Reviews []struct {
		LineNumber    int    `json:"lineNumber"`
		ReviewComment string `json:"reviewComment"`
		LineText      string `json:"lineText,omitempty"`
	} `json:"reviews"`
}
