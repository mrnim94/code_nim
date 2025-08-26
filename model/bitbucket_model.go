package model

// Structure for the pull request metadata
type PullRequest struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	CreatedOn   string `json:"created_on"`
	State       string `json:"state"`
	Author      struct {
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
	} `json:"author"`
}

type PullRequestComment struct {
	ID      int `json:"id"`
	Content struct {
		Raw string `json:"raw"` // The content of the comment
	} `json:"content"`
	User struct {
		DisplayName string `json:"display_name"` // The name of the author
		Username    string `json:"nickname"`     // The username of the author (used instead of `username` in the raw response)
	} `json:"user"`
}
