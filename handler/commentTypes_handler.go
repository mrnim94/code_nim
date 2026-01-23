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
func (ar *AutoReviewPRHandler) PostSummaryComment(auto *model.AutoReviewPR, pr *model.PullRequest, diff string, lastReviewedHash, latestCommitHash string) (bool, error) {

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
	if lastReviewedHash != "" && latestCommitHash != "" && lastReviewedHash != latestCommitHash {
		head = fmt.Sprintf("Summary by Nim (new commits since %s)\n\n", shortHash(lastReviewedHash))
	}
	marker := reviewBotMarker
	if latestCommitHash != "" {
		marker = fmt.Sprintf("%s\n\n<!-- auto-review-base:%s -->", reviewBotMarker, latestCommitHash)
	}
	body := head + helper.FormatSummaryBody(summaryText) + "\n\n" + marker
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
	totalCommentCount int,
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

	outOfRange := 0
	deletedLine := 0
	emptySnippet := 0
	emptyBody := 0
	commandBody := 0
	missingLocation := 0
	duplicateCount := 0
	aiCount := 0
	filteredCount := 0
	postedCount := 0

	maxInline := auto.MaxInlineComments
	if maxInline <= 0 {
		maxInline = 100
	}
	maxTotal := auto.MaxTotalComments
	if maxTotal <= 0 {
		maxTotal = 200
	}
	remainingByInline := maxInline - len(existingInlineComments)
	if remainingByInline <= 0 {
		log.Infof("Inline review limit reached for PR #%d (max=%d, existing=%d); skipping new comments", pr.ID, maxInline, len(existingInlineComments))
		return 0, nil
	}
	remainingByTotal := maxTotal - totalCommentCount
	if remainingByTotal <= 0 {
		log.Infof("Total comment limit reached for PR #%d (max=%d, total=%d); skipping new comments", pr.ID, maxTotal, totalCommentCount)
		return 0, nil
	}
	remaining := remainingByInline
	if remainingByTotal < remaining {
		remaining = remainingByTotal
	}
	for _, file := range parsed {
		if postedCount >= remaining {
			log.Infof("Reached comment cap for PR #%d (inlineMax=%d, totalMax=%d); stopping", pr.ID, maxInline, maxTotal)
			break
		}
		filePosted := 0
		fileOutOfRange := 0
		fileDeleted := 0
		fileMissing := 0
		fileDup := 0
		fileEmptyBody := 0
		fileCommand := 0
		fileAiCount := 0
		fileInvalidAI := false
		fileAIError := false
		filePath := file["path"].(string)
		log.Debugf("Check File path %s", filePath)
		hunks := file["hunks"].([]map[string]interface{})
		allLines, lineMap := helper.BuildDiffSnippetAndLineMap(hunks)
		if len(allLines) == 0 {
			emptySnippet++
			log.Infof("Posted 0 inline comments for file %s (emptyDiffSnippet)", filePath)
			continue
		}
		prompt := helper.CreatePrompt(filePath, allLines, pr)

		// Call AI provider (Gemini or self) based on configuration
		comments, err := helper.GetAIResponse(prompt, auto)

		// Add small delay after AI API call to prevent rate limiting
		time.Sleep(1 * time.Second)

		if err != nil {
			log.Errorf("AI error for file %s in PR #%d: %v", filePath, pr.ID, err)
			fileAIError = true
			log.Infof("Posted 0 inline comments for file %s (aiError=true)", filePath)
			continue
		}
		fileAiCount = len(comments)
		aiCount += fileAiCount
		if fileAiCount == 0 {
			fileInvalidAI = true
		}

		for i := range comments {
			// Use anchor text to correct the index if present
			if comments[i].Anchor != "" {
				idx := helper.NearestMatchingLineIndex(allLines, comments[i].Anchor, comments[i].Position-1)
				if idx >= 0 && idx < len(lineMap) {
					comments[i].Position = idx + 1
				}
			}
			// Map AI diff index (1-based within provided snippet) to file lines
			if comments[i].Position <= 0 || comments[i].Position > len(lineMap) {
				log.Debugf("Skip comment with out-of-range position %d for file %s", comments[i].Position, filePath)
				comments[i].Position = 0
				fileOutOfRange++
				outOfRange++
				continue
			}
			mapping := lineMap[comments[i].Position-1]
			if mapping.ToLine <= 0 {
				// Deleted lines have no destination; skip commenting on them
				log.Debugf("Skip comment on deleted line (no destination) at diff idx %d for file %s", comments[i].Position, filePath)
				comments[i].Position = 0
				fileDeleted++
				deletedLine++
				continue
			}
			comments[i].Path = filePath
			comments[i].Position = mapping.ToLine   // destination/new file line
			comments[i].FromLine = mapping.FromLine // source/old file line (-1 for added lines)
		}

		for _, c := range comments {
			if postedCount >= remaining {
				log.Infof("Reached comment cap for PR #%d (inlineMax=%d, totalMax=%d); stopping", pr.ID, maxInline, maxTotal)
				break
			}
			if c.Body == "" {
				fileEmptyBody++
				emptyBody++
				continue
			}
			if helper.LooksLikeCommand(c.Body) {
				fileCommand++
				commandBody++
				continue
			}
			filteredCount++
			if c.Path == "" || c.Position <= 0 {
				fileMissing++
				missingLocation++
				continue
			}
			key := fmt.Sprintf("%s:%d", c.Path, c.Position)
			if existingInlineComments[key] {
				log.Debugf("Skipping duplicate inline comment at %s", key)
				fileDup++
				duplicateCount++
				continue
			}

			formattedBody := helper.FormatReviewBody(c.Body)
			if !strings.Contains(formattedBody, reviewBotMarker) {
				formattedBody = formattedBody + "\n\n" + reviewBotMarker
			}
			// Convert FromLine: -1 means added line (no source), use 0 for API
			fromLineForAPI := c.FromLine
			if fromLineForAPI < 0 {
				fromLineForAPI = 0
			}
			err := ar.Bitbucket.PushPullRequestInlineComment(
				pr.ID,
				auto.Workspace,
				auto.RepoSlug,
				auto.Username,
				auto.AppPassword,
				c.Path,
				fromLineForAPI, // from line in old/source file
				c.Position,     // to line in new/destination file
				formattedBody,
			)
			if err != nil {
				log.Errorf("Failed to post inline comment: %v", err)
			} else {
				log.Debugf("✓ Posted inline comment on %s at line %d (from=%d, to=%d)", c.Path, c.Position, fromLineForAPI, c.Position)
				postedCount++
				filePosted++
				existingInlineComments[key] = true
			}
		}
		if filePosted == 0 && (fileAiCount > 0 || fileInvalidAI || fileAIError) {
			log.Infof("Posted 0 inline comments for file %s (ai=%d, dup=%d, deleted=%d, outOfRange=%d, missingLocation=%d, empty=%d, command=%d, invalidAI=%t, aiError=%t)",
				filePath,
				fileAiCount,
				fileDup,
				fileDeleted,
				fileOutOfRange,
				fileMissing,
				fileEmptyBody,
				fileCommand,
				fileInvalidAI,
				fileAIError,
			)
		}
	}
	if postedCount > 0 {
		log.Infof("✓ Posted %d inline review comments for PR #%d", postedCount, pr.ID)
	} else {
		log.Infof("No inline comments posted for PR #%d (ai=%d, filtered=%d, empty=%d, command=%d, outOfRange=%d, deleted=%d, missingLocation=%d, dup=%d, emptySnippet=%d)",
			pr.ID,
			aiCount,
			filteredCount,
			emptyBody,
			commandBody,
			outOfRange,
			deletedLine,
			missingLocation,
			duplicateCount,
			emptySnippet,
		)
	}
	return postedCount, nil
}
