package handler

import (
	"code_nim/helper"
	"code_nim/helper/atlassian"
	"code_nim/log"
	"code_nim/model"
	"fmt"
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

const reviewMarkerPrefix = "<!-- auto-review-base:"
const reviewMarkerSuffix = "-->"
const reviewBotMarker = "<!-- auto-review-bot -->"

func hasBotMarker(raw string) bool {
	return strings.Contains(raw, reviewMarkerPrefix) || strings.Contains(raw, reviewBotMarker)
}

func extractLastReviewedHash(comments []model.PullRequestComment) string {
	var lastFound string
	for _, comment := range comments {
		if comment.Inline != nil {
			continue
		}
		raw := comment.Content.Raw
		start := strings.Index(raw, reviewMarkerPrefix)
		if start == -1 {
			continue
		}
		start += len(reviewMarkerPrefix)
		end := strings.Index(raw[start:], reviewMarkerSuffix)
		if end == -1 {
			continue
		}
		hash := strings.TrimSpace(raw[start : start+end])
		if hash != "" {
			lastFound = hash
		}
	}
	return lastFound
}

func shortHash(hash string) string {
	if len(hash) <= 7 {
		return hash
	}
	return hash[:7]
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
			skipAllByLGTM := false

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
			lastReviewedHash := ""

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

				// If a commenter says 'LGTM', pause all bot reviews for this PR.
				if comment.Inline == nil && !hasBotMarker(comment.Content.Raw) {
					lcBody := strings.ToLower(strings.TrimSpace(comment.Content.Raw))
					if strings.Contains(lcBody, "lgtm") {
						skipAllByLGTM = true
						log.Infof("LGTM detected by %s; will skip all reviews for PR #%d", comment.User.DisplayName, pullRequest.ID)
					}
				}

				// NOTE: Do not skip inline reviews just because a human reviewer left comments.
				// Inline review is only paused by explicit LGTM (see skipAllByLGTM).

				// Detect existing inline review comments posted by the bot (to avoid duplicates).
				// Use hidden marker to distinguish bot comments when accounts are shared.
				if comment.Inline != nil && hasBotMarker(comment.Content.Raw) {
					hasInlineReview = true
					key := fmt.Sprintf("%s:%d", comment.Inline.Path, comment.Inline.To)
					existingInlineComments[key] = true
					log.Debugf("Found existing inline review (by bot) at %s:%d", comment.Inline.Path, comment.Inline.To)
				}
			}
			if skipAllByLGTM {
				log.Infof("Skipping PR #%d because LGTM pause is active", pullRequest.ID)
				continue
			}
			lastReviewedHash = extractLastReviewedHash(comments)

			commits, err := ar.Bitbucket.FetchPullRequestCommits(pullRequest.ID, auto.Workspace, auto.RepoSlug, auto.Username, auto.AppPassword)
			if err != nil {
				log.Errorf("Error fetching commits for PR #%d: %v", pullRequest.ID, err)
			}
			latestCommitHash := ""
			if len(commits) > 0 {
				latestCommitHash = commits[0].Hash
			}
			hasNewCommits := false
			useDeltaDiff := false
			if latestCommitHash != "" {
				if lastReviewedHash == "" {
					hasNewCommits = true
				} else if lastReviewedHash != latestCommitHash {
					hasNewCommits = true
					for _, c := range commits {
						if c.Hash == lastReviewedHash {
							useDeltaDiff = true
							break
						}
					}
				}
			}
			if latestCommitHash == "" {
				log.Infof("PR #%d commit tracking unavailable; using full diff", pullRequest.ID)
			} else if hasNewCommits {
				if lastReviewedHash == "" {
					log.Infof("PR #%d has new commits; first review detected (latest %s)", pullRequest.ID, shortHash(latestCommitHash))
				} else {
					log.Infof("PR #%d has new commits since %s (latest %s)", pullRequest.ID, shortHash(lastReviewedHash), shortHash(latestCommitHash))
				}
			} else {
				log.Infof("PR #%d has no new commits since last review", pullRequest.ID)
			}

			// Fetch diff for both summary and inline review
			log.Debugf("Check Diff PR: %d - %d", pullRequest.ID, i)
			var diff string
			if useDeltaDiff {
				diff, err = ar.Bitbucket.FetchDiffBetweenCommits(auto.Workspace, auto.RepoSlug, lastReviewedHash, latestCommitHash, auto.Username, auto.AppPassword)
			} else {
				diff, err = ar.Bitbucket.FetchPullRequestDiff(pullRequest.ID, auto.Workspace, auto.RepoSlug, auto.Username, auto.AppPassword)
			}
			if err != nil {
				log.Errorf("Error fetching diff: %v", err)
				return err
			}
			if strings.TrimSpace(diff) == "" || !strings.Contains(diff, "diff --git") {
				if useDeltaDiff {
					log.Warnf("Delta diff empty for PR #%d; falling back to full PR diff", pullRequest.ID)
					diff, err = ar.Bitbucket.FetchPullRequestDiff(pullRequest.ID, auto.Workspace, auto.RepoSlug, auto.Username, auto.AppPassword)
					if err != nil {
						log.Errorf("Error fetching fallback full diff: %v", err)
						return err
					}
				}
				if strings.TrimSpace(diff) == "" {
					log.Warnf("PR #%d diff is empty after fallback; skipping review", pullRequest.ID)
					continue
				}
			}

			if !hasSummary || (hasNewCommits && latestCommitHash != "") {
				// STEP 1: Check and post summary comment if it doesn't exist
				_, _ = ar.PostSummaryComment(&auto, &pullRequest, diff, lastReviewedHash, latestCommitHash)
			} else {
				log.Infof("Summary already exists for PR #%d, skipping", pullRequest.ID)
			}

			// STEP 2: Check and post inline review comments if they don't exist (delegated)
			skipInlineDueToExisting := hasInlineReview && !hasNewCommits
			_, _ = ar.ensureInlineReviewComments(&auto, &pullRequest, diff, existingInlineComments, skipInlineByDisplayName, skipInlineDueToExisting)
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
