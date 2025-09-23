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

// GeminiErrorResponse represents the error response structure from Gemini API
type GeminiErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}
