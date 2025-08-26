package model

type ReviewComment struct {
	Body     string `json:"body"`
	Path     string `json:"path"`
	Position int    `json:"position"`
}
type ReviewResponse struct {
	Reviews []struct {
		LineNumber    int    `json:"lineNumber"`
		ReviewComment string `json:"reviewComment"`
	} `json:"reviews"`
}
