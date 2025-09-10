package handler

import (
	"code_nim/helper"
	"code_nim/helper/atlassian"
	"code_nim/log"
	"code_nim/model"
	"fmt"
	"github.com/go-co-op/gocron"
	"strconv"
	"strings"
	"time"
)

type AutoReviewPRHandler struct {
	Bitbucket atlassian.Bitbucket
}

func (ar AutoReviewPRHandler) HandlerAutoReviewPR() {
	var cfg model.Task
	helper.LoadConfigFile(&cfg)
	log.Info("Init Review PullRequest Handler")

	s := gocron.NewScheduler(time.UTC)

	reviewTask := func(auto model.AutoReviewPR) error {
		log.Info("Start Review PR Handler")
		allPR, err := ar.Bitbucket.FetchAllPullRequests(auto.Username, auto.AppPassword, auto.Workspace, auto.RepoSlug)
		if err != nil {
			log.Error("Error rotating session: %v", err)
			return err
		}
		//fmt.Println(allPR)
		for i, pullRequest := range allPR {
			comments, err := ar.Bitbucket.FetchPullRequestComments(pullRequest.ID, auto.Workspace, auto.RepoSlug, auto.Username, auto.AppPassword)
			if err != nil {
				log.Error("Error Pull Comments: %v", err)
				return err
			}
			userCommented := false
			// Check if the user or any of their display names have already commented
			for i2, comment := range comments {
				log.Debugf("Check Comment of %s - %s in PR : %d - %d", comment.User.Username, comment.User.DisplayName, pullRequest.ID, i2)
				if comment.User.Username == auto.Username && comment.Content.Raw != "" {
					log.Debugf("User %s Commented", auto.Username)
					userCommented = true
					break
				}
				// Check against all display names
				for _, displayName := range auto.DisplayNames {
					if comment.User.DisplayName == displayName && comment.Content.Raw != "" {
						log.Debugf("User %s Commented", displayName)
						log.Debugf("Comment is : %s", comment.Content.Raw)
						userCommented = true
						break
					}
				}
				if userCommented {
					break
				}
			}
			if userCommented {
				continue // Skip review if user already commented with non-empty content
			}
			log.Debugf("Check Diff PR: %d - %d", pullRequest.ID, i)
			diff, err := ar.Bitbucket.FetchPullRequestDiff(pullRequest.ID, auto.Workspace, auto.RepoSlug, auto.Username, auto.AppPassword)
			if err != nil {
				log.Error("Error rotating session: %v", err)
				return err
			}
			parsed := ar.Bitbucket.ParseDiff(string(diff))
			var allComments []model.ReviewComment
			for _, file := range parsed {
				filePath := file["path"].(string)
				log.Debugf("Check File path %s", filePath)
				hunks := file["hunks"].([]map[string]interface{})
				allLines, toLineMap := buildDiffSnippetAndLineMap(hunks)
				if len(allLines) == 0 {
					continue
				}
				prompt := helper.CreatePrompt(filePath, allLines, &pullRequest)
				comments, err := helper.GetAIResponseOfGemini(prompt, auto.GeminiKey, "gemini-2.5-flash")
				if err != nil {
					log.Printf("AI error: %v", err)
					continue
				}
				for i := range comments {
					// Map AI diff index (1-based within provided snippet) to destination file line
					if comments[i].Position <= 0 || comments[i].Position > len(toLineMap) {
						log.Debugf("Skip comment with out-of-range position %d for file %s", comments[i].Position, filePath)
						comments[i].Position = 0
						continue
					}
					mapped := toLineMap[comments[i].Position-1]
					if mapped <= 0 {
						// Deleted lines have no destination; skip
						log.Debugf("Skip comment on deleted line (no destination) at diff idx %d for file %s", comments[i].Position, filePath)
						comments[i].Position = 0
						continue
					}
					comments[i].Path = filePath
					comments[i].Position = mapped
				}
				allComments = append(allComments, comments...)
			}
			// Filter comments: no empty body and no command-like content
			filteredComments := make([]model.ReviewComment, 0, len(allComments))
			for _, comment := range allComments {
				if comment.Body == "" {
					continue
				}
				// Simple check for command-like content (e.g., lines starting with $ or common shell commands)
				if looksLikeCommand(comment.Body) {
					continue
				}
				filteredComments = append(filteredComments, comment)
			}
			if len(filteredComments) > 0 {
				fmt.Printf("Comments: %+v\n", filteredComments)
				for _, comment := range filteredComments {
					if comment.Path == "" || comment.Position <= 0 {
						continue
					}
					err := ar.Bitbucket.PushPullRequestInlineComment(
						pullRequest.ID,
						auto.Workspace,
						auto.RepoSlug,
						auto.Username,
						auto.AppPassword,
						comment.Path,
						comment.Position,
						comment.Body,
					)
					if err != nil {
						log.Errorf("Failed to post inline comment: %v", err)
					}
				}
			}
		}

		return nil
	}

	for i, review := range cfg.AutoReviewPRs {
		review := review
		log.Info("Setup Review ", i, " ==> ", review.Cron)
		_, err := s.Cron(review.Cron).Do(reviewTask, review)
		if err != nil {
			log.Error(err)
		}
	}
	s.StartAsync()
}

// looksLikeCommand checks if the comment body looks like a shell command or user command
func looksLikeCommand(body string) bool {
	// Add more sophisticated checks as needed
	commandIndicators := []string{"$", "#!/bin/", "sudo ", "rm ", "ls ", "cd ", "echo ", "cat ", "touch ", "mkdir ", "curl ", "wget ", "python ", "go run", "npm ", "yarn ", "git ", "exit", "shutdown", "reboot"}
	for _, indicator := range commandIndicators {
		if len(body) >= len(indicator) && body[:len(indicator)] == indicator {
			return true
		}
	}
	return false
}

// buildDiffSnippetAndLineMap flattens hunks for the AI prompt and builds a mapping
// from snippet index (1-based in AI output) to destination file line (to-line).
// For lines not present on destination (deleted '-' lines), the map value is <= 0.
func buildDiffSnippetAndLineMap(hunks []map[string]interface{}) ([]string, []int) {
    var snippet []string
    var toLineMap []int
    for _, h := range hunks {
        header, _ := h["header"].(string)
        lines, _ := h["lines"].([]string)
        // Parse header like: @@ -a,b +c,d @@
        // Extract c (start line on destination)
        destStart := 0
        if parts := strings.Split(header, "+"); len(parts) > 1 {
            // parts[1] like: c,d @@ ...
            right := parts[1]
            // trim up to first space or '@'
            if idx := strings.IndexAny(right, " @"); idx >= 0 {
                right = right[:idx]
            }
            if idx := strings.Index(right, ","); idx >= 0 {
                right = right[:idx]
            }
            if v, err := strconv.Atoi(strings.TrimSpace(right)); err == nil {
                destStart = v
            }
        }
        destLine := destStart
        for _, ln := range lines {
            snippet = append(snippet, ln)
            if strings.HasPrefix(ln, "+") || (!strings.HasPrefix(ln, "+") && !strings.HasPrefix(ln, "-")) {
                // added or context line advances destination
                if strings.HasPrefix(ln, "+") {
                    toLineMap = append(toLineMap, destLine)
                    destLine++
                } else {
                    // context line
                    toLineMap = append(toLineMap, destLine)
                    destLine++
                }
            } else if strings.HasPrefix(ln, "-") {
                // removed line: no destination line
                toLineMap = append(toLineMap, -1)
            } else {
                toLineMap = append(toLineMap, -1)
            }
        }
    }
    return snippet, toLineMap
}
