package handler

import (
	"code_nim/helper"
	"code_nim/helper/atlassian"
	"code_nim/log"
	"code_nim/model"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
)

// Helper function for min operation
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// normalizeUsername lowers case and removes common separators to handle minor differences
// such as "thang-tran" vs "thang.tran" vs "Thang_Tran".
func isConfiguredDisplayName(name string, list []string) bool {
	n := strings.TrimSpace(name)
	for _, dn := range list {
		if strings.TrimSpace(dn) == n {
			return true
		}
	}
	return false
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

	s, err := gocron.NewScheduler()
	if err != nil {
		log.Errorf("Failed to create scheduler: %v", err)
		return
	}

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
			log.Errorf("Error rotating session: %v", err)
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

			// Summary-only mode flag: when true, we will generate summary but skip inline review
			skipInlineByDisplayName := false

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
				log.Infof("Author is in ignore list → summary-only mode for PR #%d", pullRequest.ID)
				skipInlineByDisplayName = true
			}

			log.Infof("Starting review process for PR #%d by %s", pullRequest.ID, pullRequest.Author.DisplayName)
			comments, err := ar.Bitbucket.FetchPullRequestComments(pullRequest.ID, auto.Workspace, auto.RepoSlug, auto.Username, auto.AppPassword)
			if err != nil {
				log.Errorf("Error Pull Comments: %v", err)
				return err
			}

			// Check for existing summary and inline review comments independently
			hasSummary := false
			hasInlineReview := false
			existingInlineComments := make(map[string]bool)

			for i2, comment := range comments {
				log.Debugf("Check Comment of %s - %s in PR : %d - %d", comment.User.Username, comment.User.DisplayName, pullRequest.ID, i2)

				// Detect an already-posted summary in general comments (not inline)
				if comment.Content.Raw != "" && comment.Inline == nil {
					lc := strings.ToLower(strings.TrimSpace(comment.Content.Raw))
					if strings.HasPrefix(lc, "## summary") ||
						strings.Contains(lc, "summary by ") ||
						strings.Contains(lc, "- **new features**") ||
						strings.Contains(lc, "- **bug fixes**") ||
						strings.Contains(lc, "- **documentation**") ||
						strings.Contains(lc, "- **refactor**") ||
						strings.Contains(lc, "- **performance**") ||
						strings.Contains(lc, "- **tests**") ||
						strings.Contains(lc, "- **chores**") {
						hasSummary = true
					}
				}

				// If a commenter with a configured displayName says 'LGTM', skip inline review.
				if isConfiguredDisplayName(comment.User.DisplayName, auto.DisplayNames) {
					lcBody := strings.ToLower(strings.TrimSpace(comment.Content.Raw))
					if strings.Contains(lcBody, "lgtm") ||
						strings.Contains(lcBody, "why:") ||
						strings.Contains(lcBody, "how (step-by-step):") ||
						strings.Contains(lcBody, "suggested change (before/after):") ||
						strings.Contains(lcBody, "suggested change") || // fallback
						strings.Contains(lcBody, "notes:") {
						skipInlineByDisplayName = true
						log.Debugf("Reviewer %s signaled LGTM; will skip inline review", comment.User.DisplayName)
					}
				}

				// Detect existing inline review comments posted by the bot (to avoid duplicates).
				// Only count comments authored by the bot account (username match).
				if comment.Inline != nil && comment.User.Username == auto.Username {
					hasInlineReview = true
					key := fmt.Sprintf("%s:%d", comment.Inline.Path, comment.Inline.To)
					existingInlineComments[key] = true
					log.Debugf("Found existing inline review (by bot) at %s:%d", comment.Inline.Path, comment.Inline.To)
				}
			}

			// Fetch diff for both summary and inline review
			log.Debugf("Check Diff PR: %d - %d", pullRequest.ID, i)
			diff, err := ar.Bitbucket.FetchPullRequestDiff(pullRequest.ID, auto.Workspace, auto.RepoSlug, auto.Username, auto.AppPassword)
			if err != nil {
				log.Errorf("Error fetching diff: %v", err)
				return err
			}

			if !hasSummary {
				// STEP 1: Check and post summary comment if it doesn't exist
				_, _ = ar.PostSummaryComment(&auto, &pullRequest, diff)
			} else {
				log.Infof("Summary already exists for PR #%d, skipping", pullRequest.ID)
			}

			// STEP 2: Check and post inline review comments if they don't exist (delegated)
			_, _ = ar.ensureInlineReviewComments(&auto, &pullRequest, diff, existingInlineComments, skipInlineByDisplayName, hasInlineReview)
		}

		duration := time.Since(startTime)
		log.Infof("Review PR Handler completed for %s/%s in %v", auto.Workspace, auto.RepoSlug, duration)
		return nil
	}

	for i, review := range cfg.AutoReviewPRs {
		review := review
		log.Info("Setup Review ", i, " ==> ", review.Cron)
		_, err := s.NewJob(
			gocron.CronJob(review.Cron, true),
			gocron.NewTask(func() { _ = reviewTask(review) }),
		)
		if err != nil {
			log.Error(err)
		}
	}
	s.Start()
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

// formatReviewBody enforces proper markdown formatting with paragraph breaks for better rendering
func formatReviewBody(body string) string {
	if body == "" {
		return body
	}

	// List of headings that should start on new paragraphs
	headings := []string{
		"Why:",
		"How (step-by-step):",
		"Suggested change (Before/After):",
		"Notes:",
	}

	formatted := body

	// Use double newlines for proper markdown paragraph breaks
	for _, heading := range headings {
		// Replace " Heading:" with proper paragraph break
		spacedHeading := " " + heading
		properHeading := "\n\n" + heading
		formatted = strings.ReplaceAll(formatted, spacedHeading, properHeading)

		// Handle cases where heading appears without preceding space
		// but avoid double-replacing already formatted headings
		if !strings.Contains(formatted, properHeading) {
			formatted = strings.ReplaceAll(formatted, heading, properHeading)
		}
	}

	// Clean up excessive newlines (more than 2 consecutive)
	for strings.Contains(formatted, "\n\n\n") {
		formatted = strings.ReplaceAll(formatted, "\n\n\n", "\n\n")
	}

	// Remove leading newlines if they exist
	formatted = strings.TrimLeft(formatted, "\n")

	// Ensure proper spacing after colons and before bullets
	formatted = strings.ReplaceAll(formatted, ":\n  -", ":\n\n  -")
	formatted = strings.ReplaceAll(formatted, ":\n-", ":\n\n-")

	// Improve code block formatting with proper spacing
	formatted = strings.ReplaceAll(formatted, "~~~go\n//", "~~~go\n\n//")
	formatted = strings.ReplaceAll(formatted, "~~~\n~~~", "~~~\n\n~~~")

	// Ensure proper spacing around code blocks
	formatted = strings.ReplaceAll(formatted, "):\n~~~", "):\n\n~~~")

	return formatted
}

// formatSummaryBody enforces newlines around headers and bullets for PR summary
func formatSummaryBody(body string) string {
	if body == "" {
		return body
	}
	formatted := strings.ReplaceAll(body, "\r\n", "\n")
	headers := []string{
		"**New Features**",
		"**Bug Fixes**",
		"**Documentation**",
		"**Refactor**",
		"**Performance**",
		"**Tests**",
		"**Chores**",
	}
	// Ensure each header stands alone and followed by a blank line
	for _, h := range headers {
		// cases like "**Header** -" or "**Header**-" -> header + blank line + "-"
		formatted = strings.ReplaceAll(formatted, h+" - ", h+"\n\n- ")
		formatted = strings.ReplaceAll(formatted, h+"- ", h+"\n\n- ")
		formatted = strings.ReplaceAll(formatted, h+" -", h+"\n\n- ")
		// if header is followed immediately by text, force newline
		formatted = strings.ReplaceAll(formatted, h+" ", h+"\n\n")
	}
	// Handle plain (non-bold) headers that AI may emit like "New Features - ..."
	plain := []string{"New Features", "Bug Fixes", "Documentation", "Refactor", "Performance", "Tests", "Chores"}
	for _, h := range plain {
		// Convert inline header+bullet to bold header on its own line then bullet list
		formatted = strings.ReplaceAll(formatted, h+" - ", "**"+h+"**\n\n- ")
		formatted = strings.ReplaceAll(formatted, h+"- ", "**"+h+"**\n\n- ")
		formatted = strings.ReplaceAll(formatted, h+": - ", "**"+h+"**\n\n- ")
		formatted = strings.ReplaceAll(formatted, h+": ", "**"+h+"**\n\n")
		// If header followed by text without dash, still break line
		formatted = strings.ReplaceAll(formatted, h+" ", "**"+h+"**\n\n")
	}
	// Convert inline bullet separators " - " to real newlines
	formatted = strings.ReplaceAll(formatted, " - ", "\n- ")
	// Collapse triple blank lines
	for strings.Contains(formatted, "\n\n\n") {
		formatted = strings.ReplaceAll(formatted, "\n\n\n", "\n\n")
	}
	return formatted
}
