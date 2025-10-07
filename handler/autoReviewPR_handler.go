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
	"sync"
	"time"
)

// Helper function for min operation
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type AutoReviewPRHandler struct {
	Bitbucket atlassian.Bitbucket
	mutex     sync.Mutex // Prevents concurrent review executions
	isRunning bool       // Flag to track if review is currently running
}

func (ar *AutoReviewPRHandler) HandlerAutoReviewPR() {
	var cfg model.Task
	helper.LoadConfigFile(&cfg)
	log.Info("Init Review PullRequest Handler")

	s := gocron.NewScheduler(time.UTC)
	
	// Enable singleton mode to prevent overlapping job executions at the gocron level
	s.SingletonMode()

	reviewTask := func(auto model.AutoReviewPR) error {
		// Check if another review is already running (thread-safe check)
		ar.mutex.Lock()
		if ar.isRunning {
			ar.mutex.Unlock()
			log.Infof("Skipping review execution - another review process is already running for %s/%s", auto.Workspace, auto.RepoSlug)
			return nil
		}
		ar.isRunning = true
		ar.mutex.Unlock()
		
		// Ensure we reset the running flag when done
		defer func() {
			ar.mutex.Lock()
			ar.isRunning = false
			ar.mutex.Unlock()
			log.Info("Review PR Handler completed - lock released")
		}()
		
		startTime := time.Now()
		log.Infof("Start Review PR Handler for %s/%s (acquired lock)", auto.Workspace, auto.RepoSlug)
		allPR, err := ar.Bitbucket.FetchAllPullRequests(auto.Username, auto.AppPassword, auto.Workspace, auto.RepoSlug)
		if err != nil {
			log.Error("Error rotating session: %v", err)
			return err
		}
		log.Infof("Fetched %d pull requests for review", len(allPR))
		for i, pullRequest := range allPR {
			log.Infof("Processing PR #%d: '%s' by %s", pullRequest.ID, pullRequest.Title, pullRequest.Author.DisplayName)
			
			// Add small delay between PRs to reduce API load and prevent rate limiting
			if i > 0 {
				time.Sleep(2 * time.Second)
				log.Debugf("Added delay before processing PR #%d", pullRequest.ID)
			}

			ignorePROfName := false
			for _, displayNameConfig := range auto.IgnorePullRequestOf.DisplayNames {
				log.Debugf("Checking if PR author '%s' matches ignore list entry '%s'", pullRequest.Author.DisplayName, displayNameConfig)
				if displayNameConfig == pullRequest.Author.DisplayName {
					log.Infof("Will ignore PR #%d by %s (matches ignore list)", pullRequest.ID, displayNameConfig)
					ignorePROfName = true
					break // No need to check other ignore entries once we found a match
				}
			}
			if ignorePROfName {
				log.Infof("Skipping PR #%d (author is in ignore list)", pullRequest.ID)
				continue
			}

			log.Infof("Starting review process for PR #%d by %s", pullRequest.ID, pullRequest.Author.DisplayName)
			comments, err := ar.Bitbucket.FetchPullRequestComments(pullRequest.ID, auto.Workspace, auto.RepoSlug, auto.Username, auto.AppPassword)
			if err != nil {
				log.Error("Error Pull Comments: %v", err)
				return err
			}
			
			// Check if the user or any of their display names have already commented
			userCommented := false
			
			for i2, comment := range comments {
				log.Debugf("Check Comment of %s - %s in PR : %d - %d", comment.User.Username, comment.User.DisplayName, pullRequest.ID, i2)
				
				isUserComment := comment.User.Username == auto.Username
				if !isUserComment {
					// Check against all display names
					for _, displayName := range auto.DisplayNames {
						if comment.User.DisplayName == displayName {
							isUserComment = true
							break
						}
					}
				}
				
				if isUserComment && comment.Content.Raw != "" {
					log.Debugf("User %s already commented on PR #%d", comment.User.Username, pullRequest.ID)
					if comment.Inline != nil {
						log.Debugf("Found existing inline comment at %s:%d", comment.Inline.Path, comment.Inline.To)
					} else {
						log.Debugf("Found existing general PR comment")
					}
					userCommented = true
					break // Skip the entire PR if user has made ANY comment (inline or general)
				}
			}
			
			if userCommented {
				log.Infof("Skipping PR #%d - user has already reviewed (commented)", pullRequest.ID)
				continue // Skip review if user already commented anywhere on the PR
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
				
				// Use configured model or default to gemini-2.5-flash
				model := auto.GeminiModel
				if model == "" {
					model = "gemini-2.5-flash" // Default model
					log.Debugf("No model specified in config, using default: %s", model)
				} else {
					log.Debugf("Using configured Gemini model: %s", model)
				}
				
				comments, err := helper.GetAIResponseOfGemini(prompt, auto.GeminiKey, model)
				
				// Add small delay after Gemini API call to prevent rate limiting
				time.Sleep(1 * time.Second)
				
				if err != nil {
					log.Errorf("AI error for file %s in PR #%d: %v", filePath, pullRequest.ID, err)
					
					// Check if it's a rate limit error and provide specific guidance
					errStr := err.Error()
					if strings.Contains(errStr, "rate limit exceeded") {
						log.Errorf("Rate limit hit for PR #%d. Consider:")
						log.Error("  1. Reducing the number of PRs processed per run")
						log.Error("  2. Adding delays between API calls")
						log.Error("  3. Upgrading your Gemini API plan")
						log.Error("  4. Processing only smaller PRs to reduce token usage")
						// Consider breaking the loop if rate limited to avoid more failures
						log.Infof("Skipping remaining files for PR #%d due to rate limit", pullRequest.ID)
						break // Skip remaining files for this PR
					}
					
					log.Debugf("Prompt that caused the error (first 200 chars): %s", prompt[:min(200, len(prompt))])
					continue
				}
				for i := range comments {
					// Use anchor text to correct the index if present
					if comments[i].Anchor != "" {
						idx := nearestMatchingLineIndex(allLines, comments[i].Anchor, comments[i].Position-1)
						if idx >= 0 && idx < len(toLineMap) {
							comments[i].Position = idx + 1
						}
					}
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
					
					// Since we skip entire PRs where user has already commented,
					// we don't need to check for existing inline comments here
					
					// Ensure section headings like Why/How render on their own lines
					formattedBody := formatReviewBody(comment.Body)
					err := ar.Bitbucket.PushPullRequestInlineComment(
						pullRequest.ID,
						auto.Workspace,
						auto.RepoSlug,
						auto.Username,
						auto.AppPassword,
						comment.Path,
						comment.Position,
						formattedBody,
					)
					if err != nil {
						log.Errorf("Failed to post inline comment: %v", err)
					} else {
						log.Debugf("Posted new inline comment on %s at line %d", comment.Path, comment.Position)
					}
				}
			}
		}

		duration := time.Since(startTime)
		log.Infof("Review PR Handler completed for %s/%s in %v", auto.Workspace, auto.RepoSlug, duration)
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

// nearestMatchingLineIndex finds the nearest index in diffLines whose content contains the anchor.
// It searches first at the hinted index, then walks outward.
func nearestMatchingLineIndex(diffLines []string, anchor string, hintIdx int) int {
	if len(diffLines) == 0 || anchor == "" {
		return -1
	}
	// Normalize anchor for comparison (trim and remove leading +/- for robustness)
	normAnchor := strings.TrimSpace(anchor)
	strip := func(s string) string {
		s = strings.TrimSpace(s)
		if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
			return strings.TrimSpace(s[1:])
		}
		return s
	}
	normAnchor = strip(normAnchor)

	inBounds := func(i int) bool { return i >= 0 && i < len(diffLines) }
	match := func(i int) bool {
		line := strip(diffLines[i])
		return strings.Contains(line, normAnchor)
	}

	// Clamp hint
	if hintIdx < 0 {
		hintIdx = 0
	}
	if hintIdx >= len(diffLines) {
		hintIdx = len(diffLines) - 1
	}
	// Check hint position first
	if inBounds(hintIdx) && match(hintIdx) {
		return hintIdx
	}
	// Expand search radius
	for radius := 1; radius < len(diffLines); radius++ {
		l := hintIdx - radius
		r := hintIdx + radius
		if inBounds(l) && match(l) {
			return l
		}
		if inBounds(r) && match(r) {
			return r
		}
	}
	return -1
}

// formatReviewBody enforces line breaks before key headings to improve rendering
func formatReviewBody(body string) string {
	if body == "" {
		return body
	}
	
	// List of headings that should start on new lines
	headings := []string{
		"Why:",
		"How (step-by-step):",
		"Suggested change (Before/After):",
		"Notes:",
	}
	
	formatted := body
	
	// Simple and reliable approach: replace any occurrence of these headings
	// with newline + heading, then clean up any duplicate newlines
	for _, heading := range headings {
		// Replace both spaced and non-spaced versions
		formatted = strings.ReplaceAll(formatted, " "+heading, "\n"+heading)
		formatted = strings.ReplaceAll(formatted, heading, "\n"+heading)
	}
	
	// Clean up duplicate newlines
	formatted = strings.ReplaceAll(formatted, "\n\n", "\n")
	
	// Remove leading newline if it exists (in case first word was a heading)
	formatted = strings.TrimPrefix(formatted, "\n")
	
	return formatted
}
