package atlassian

import "code_nim/model"

// Bitbucket exposes the operations your app cares about.
// ctx lets the caller cancel / set timeouts.
type Bitbucket interface {
	FetchAllPullRequests(username, appPassword, workspace, repoSlug string) ([]model.PullRequest, error)
	FetchPullRequestDiff(prID int, workspace, repoSlug, username, appPassword string) (string, error)
	ParseDiff(diff string) []map[string]interface{}
	FetchPullRequestComments(prID int, workspace, repoSlug, username, appPassword string) ([]model.PullRequestComment, error)
	PushPullRequestComment(prID int, workspace, repoSlug, username, appPassword, commentText string) error
}
