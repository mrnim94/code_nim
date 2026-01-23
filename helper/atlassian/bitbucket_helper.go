package atlassian

import "code_nim/model"

// Bitbucket exposes the operations your app cares about.
// ctx lets the caller cancel / set timeouts.
type Bitbucket interface {
	FetchAllPullRequests(username, appPassword, workspace, repoSlug string) ([]model.PullRequest, error)
	FetchPullRequestDiff(prID int, workspace, repoSlug, username, appPassword string) (string, error)
	FetchPullRequestCommits(prID int, workspace, repoSlug, username, appPassword string) ([]model.PullRequestCommit, error)
	FetchDiffBetweenCommits(workspace, repoSlug, fromHash, toHash, username, appPassword string) (string, error)
	ParseDiff(diff string) []map[string]interface{}
	FetchPullRequestComments(prID int, workspace, repoSlug, username, appPassword string) ([]model.PullRequestComment, error)
	PushPullRequestComment(prID int, workspace, repoSlug, username, appPassword, commentText string) error
	// PushPullRequestInlineComment posts a comment on a specific file and line in the PR
	// Bitbucket Cloud API expects the path, fromLine (source/old file), and toLine (destination/new file)
	// For added lines, fromLine should be 0; for deleted lines, toLine should be 0
	PushPullRequestInlineComment(prID int, workspace, repoSlug, username, appPassword, path string, fromLine, toLine int, content string) error
}
