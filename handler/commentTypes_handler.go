package handler

import (
	"code_nim/helper"
	"code_nim/log"
	"code_nim/model"
	"fmt"
	"strings"
	"time"
)

// ensureSummaryComment generates and posts a summary comment if one doesn't already exist.
// Returns (posted, error). If hasSummaryAlready is true, it only logs and returns (false, nil).
func (ar *AutoReviewPRHandler) PostSummaryComment(auto *model.AutoReviewPR, pr *model.PullRequest, diff string) (bool, error) {

	log.Infof("No summary found for PR #%d, generating one...", pr.ID)
	summaryPrompt := helper.CreateSummaryPrompt(pr, diff)
	summaryText, sumErr := helper.GetAISummary(summaryPrompt, auto)
	if sumErr != nil {
		log.Errorf("AI summary error for PR #%d: %v", pr.ID, sumErr)
		return false, sumErr
	}

	trimmed := strings.TrimSpace(summaryText)
	if trimmed == "" {
		log.Warnf("AI returned empty summary text for PR #%d", pr.ID)
		return false, nil
	}
	log.Debugf("AI summary response length: %d chars (first 100): %s", len(trimmed), trimmed[:min(100, len(trimmed))])

	head := "Summary by Nim\n\n"
	body := head + formatSummaryBody(summaryText)
	log.Debugf("Posting summary comment with body length: %d", len(body))
	if err := ar.Bitbucket.PushPullRequestComment(pr.ID, auto.Workspace, auto.RepoSlug, auto.Username, auto.AppPassword, body); err != nil {
		log.Errorf("Failed to post summary comment: %v", err)
		return false, err
	}
	log.Infof("✓ Posted summary comment for PR #%d", pr.ID)
	return true, nil
}

// ensureInlineReviewComments generates and posts inline review comments if they don't already exist.
// Returns (postedCount, error). Skips when skipInline is true or hasInlineAlready is true.
func (ar *AutoReviewPRHandler) ensureInlineReviewComments(
	auto *model.AutoReviewPR,
	pr *model.PullRequest,
	diff string,
	existingInlineComments map[string]bool,
	skipInline bool,
	hasInlineAlready bool,
) (int, error) {
	if skipInline {
		log.Infof("Skipping inline review for PR #%d due to reviewer presence in displayNames", pr.ID)
		return 0, nil
	}
	if hasInlineAlready {
		log.Infof("Inline review already exists for PR #%d, skipping", pr.ID)
		return 0, nil
	}

	log.Infof("No inline review found for PR #%d, generating one...", pr.ID)
	parsed := ar.Bitbucket.ParseDiff(diff)

	var allComments []model.ReviewComment
	for _, file := range parsed {
		filePath := file["path"].(string)
		log.Debugf("Check File path %s", filePath)
		hunks := file["hunks"].([]map[string]interface{})
		allLines, toLineMap := buildDiffSnippetAndLineMap(hunks)
		if len(allLines) == 0 {
			continue
		}
		prompt := helper.CreatePrompt(filePath, allLines, pr)

		// Call AI provider (Gemini or self) based on configuration
		comments, err := helper.GetAIResponse(prompt, auto)

		// Add small delay after AI API call to prevent rate limiting
		time.Sleep(1 * time.Second)

		if err != nil {
			log.Errorf("AI error for file %s in PR #%d: %v", filePath, pr.ID, err)
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
	for _, c := range allComments {
		if c.Body == "" {
			continue
		}
		if looksLikeCommand(c.Body) {
			continue
		}
		filteredComments = append(filteredComments, c)
	}

	postedCount := 0
	for _, c := range filteredComments {
		if c.Path == "" || c.Position <= 0 {
			continue
		}
		key := fmt.Sprintf("%s:%d", c.Path, c.Position)
		if existingInlineComments[key] {
			log.Debugf("Skipping duplicate inline comment at %s", key)
			continue
		}

		formattedBody := formatReviewBody(c.Body)
		err := ar.Bitbucket.PushPullRequestInlineComment(
			pr.ID,
			auto.Workspace,
			auto.RepoSlug,
			auto.Username,
			auto.AppPassword,
			c.Path,
			c.Position,
			formattedBody,
		)
		if err != nil {
			log.Errorf("Failed to post inline comment: %v", err)
		} else {
			log.Debugf("✓ Posted inline comment on %s at line %d", c.Path, c.Position)
			postedCount++
		}
	}
	if postedCount > 0 {
		log.Infof("✓ Posted %d inline review comments for PR #%d", postedCount, pr.ID)
	}
	return postedCount, nil
}
