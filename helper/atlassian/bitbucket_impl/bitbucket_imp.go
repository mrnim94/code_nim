package bitbucket_impl

import (
	"code_nim/log"
	"code_nim/model"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Fetch the diff for a specific pull request
func (hc *HttpClient) FetchAllPullRequests(username, appPassword, workspace, repoSlug string) ([]model.PullRequest, error) {
	// Construct the API URL to get all pull requests for a specific repository
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests", workspace, repoSlug)
	log.Debugf("Fetching all pull requests from URL: %s", apiURL)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	// Add Basic Authentication header
	req.SetBasicAuth(username, appPassword)

	// Make the request for the diff
	resp, err := hc.http.Do(req)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	defer resp.Body.Close()

	// Check if the request was successful
	if resp.StatusCode != 200 {
		log.Errorf("Error: Expected status 200 but got %d", resp.StatusCode)
		return nil, err
	}

	// Print the raw response body for debugging
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	// Print the raw response (useful for debugging)
	//log.Debug("Raw API Response:", string(rawBody))

	// You can also use an anonymous struct if you prefer
	var result struct {
		Values  []model.PullRequest `json:"values"`
		Pagelen int                 `json:"pagelen"`
		Size    int                 `json:"size"`
		Page    int                 `json:"page"`
	}

	// Unmarshal the raw response into the result object
	if err := json.Unmarshal(rawBody, &result); err != nil {
		log.Error(err)
		return nil, err
	}

	// Log summary instead of full raw response to avoid massive logs
	log.Debugf("Parsed API response: %d pull requests (page %d, size %d)", len(result.Values), result.Page, result.Size)

	return result.Values, nil
}

func (hc *HttpClient) FetchPullRequestDiff(prID int, workspace, repoSlug, username, appPassword string) (string, error) {
	// Construct the API URL to get the diff for a specific pull request
	diffAPIURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%d/diff", workspace, repoSlug, prID)
	log.Debugf("Fetching diff from URL: %s", diffAPIURL) // Debugging line

	req, err := http.NewRequest("GET", diffAPIURL, nil)
	if err != nil {
		log.Fatal(err)
		return "", err
	}
	// Add Basic Authentication header
	req.SetBasicAuth(username, appPassword)

	// Make the request for the diff
	resp, err := hc.http.Do(req)
	if err != nil {
		log.Error(err)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Errorf("Error: Expected status 200 but got %d", resp.StatusCode)
		return "", fmt.Errorf("Error: Expected status 200 but got %d", resp.StatusCode)
	}
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(err)
		return "", err
	}
	//log.Debug("Raw API Response:", string(rawBody))
	return string(rawBody), nil
}

// Minimal diff parser for demonstration
func (hc *HttpClient) ParseDiff(diff string) []map[string]interface{} {
	files := []map[string]interface{}{}
	var currentFile map[string]interface{}
	var currentHunk map[string]interface{}
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			if currentFile != nil {
				files = append(files, currentFile)
			}
			currentFile = map[string]interface{}{"path": "", "hunks": []map[string]interface{}{}}
		} else if strings.HasPrefix(line, "+++ b/") {
			if currentFile != nil {
				currentFile["path"] = strings.TrimPrefix(line, "+++ b/")
			}
		} else if strings.HasPrefix(line, "@@") {
			if currentFile != nil {
				currentHunk = map[string]interface{}{"header": line, "lines": []string{}}
				hunks := currentFile["hunks"].([]map[string]interface{})
				currentFile["hunks"] = append(hunks, currentHunk)
			}
		} else if currentHunk != nil {
			lines := currentHunk["lines"].([]string)
			currentHunk["lines"] = append(lines, line)
		}
	}
	if currentFile != nil {
		files = append(files, currentFile)
	}
	//log.Debug("Diff Files:", files)
	return files
}

// Fetch and list comments for a specific pull request
func (hc *HttpClient) FetchPullRequestComments(prID int, workspace, repoSlug, username, appPassword string) ([]model.PullRequestComment, error) {
	commentsAPIURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%d/comments", workspace, repoSlug, prID)
	log.Debugf("Fetching comments from URL: %s\n", commentsAPIURL)

	allComments := []model.PullRequestComment{}
	nextURL := commentsAPIURL
	for nextURL != "" {
		req, err := http.NewRequest("GET", nextURL, nil)
		if err != nil {
			log.Fatal(err)
			return nil, err
		}
		req.SetBasicAuth(username, appPassword)

		resp, err := hc.http.Do(req)
		if err != nil {
			log.Error(err)
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			log.Errorf("Error: Expected status 200 but got %d", resp.StatusCode)
		}
		rawBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Error(err)
			return nil, err
		}
		//fmt.Println("Raw Response Body Comment:")
		//fmt.Println(string(rawBody))

		var result struct {
			Comments []model.PullRequestComment `json:"values"`
			Pagelen  int                        `json:"pagelen"`
			Size     int                        `json:"size"`
			Page     int                        `json:"page"`
			Next     string                     `json:"next"`
		}
		if err := json.Unmarshal(rawBody, &result); err != nil {
			log.Error(err)
			return nil, err
		}
		allComments = append(allComments, result.Comments...)
		nextURL = result.Next
	}
	return allComments, nil

}

// Push a comment to a specific pull request
func (hc *HttpClient) PushPullRequestComment(prID int, workspace, repoSlug, username, appPassword, commentText string) error {
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%d/comments", workspace, repoSlug, prID)
	log.Debugf("Posting comment to URL: %s", apiURL)

	payload := map[string]interface{}{
		"content": map[string]string{
			"raw": commentText,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Error(err)
		return err
	}

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(body)))
	if err != nil {
		log.Error(err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(username, appPassword)

	resp, err := hc.http.Do(req)
	if err != nil {
		log.Error(err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		rawBody, _ := io.ReadAll(resp.Body)
		log.Errorf("Failed to post comment. Status: %d, Body: %s", resp.StatusCode, string(rawBody))
		return fmt.Errorf("failed to post comment, status: %d", resp.StatusCode)
	}

	log.Debug("Comment posted successfully")
	return nil
}

// PushPullRequestInlineComment posts a comment on a specific file and destination line in the PR
func (hc *HttpClient) PushPullRequestInlineComment(prID int, workspace, repoSlug, username, appPassword, path string, line int, content string) error {
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%d/comments", workspace, repoSlug, prID)
	log.Debugf("Posting inline comment to URL: %s", apiURL)

	payload := map[string]interface{}{
		"content": map[string]string{
			"raw": content,
		},
		"inline": map[string]interface{}{
			"path": path,
			"to":   line, // destination line in the diff
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Error(err)
		return err
	}

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(body)))
	if err != nil {
		log.Error(err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(username, appPassword)

	resp, err := hc.http.Do(req)
	if err != nil {
		log.Error(err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		rawBody, _ := io.ReadAll(resp.Body)
		log.Errorf("Failed to post inline comment. Status: %d, Body: %s", resp.StatusCode, string(rawBody))
		return fmt.Errorf("failed to post inline comment, status: %d", resp.StatusCode)
	}

	log.Debug("Inline comment posted successfully")
	return nil
}
