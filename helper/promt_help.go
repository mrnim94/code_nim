package helper

import (
	"code_nim/log"
	"code_nim/model"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func CreatePrompt(filePath string, hunkLines []string, pr *model.PullRequest) string {
	log.Debugf("Begin to Create Prompt for PR: %d", pr.ID)
	return fmt.Sprintf(`You are an expert code reviewer. Please follow these instructions carefully:

- Provide your feedback strictly in the following JSON format:
  {"reviews": [{"lineNumber": <diff_line_index>, "reviewComment": "<comment>"}]}

- Review the unified diff for file "%s" below. The lineNumber refers to the 1-based index of the displayed diff lines (including context and +/- lines). Do not use absolute file line numbers.
- Focus your comments on code quality, bugs, logic errors, security, performance, and best practices.
- Maybe Refactor the following code to improve readability, maintainability, and efficiency. Please ensure the logic remains unchanged.
- Use clear, concise GitHub Markdown in your comments.
- ONLY provide feedback if improvements are necessary; if the code is optimal, return an empty "reviews" array.
- IMPORTANT: Do NOT suggest adding comments to the code.
- Assess whether the changes align with the pull request's title and description.
- If the PR is too large, suggest breaking it down; if very small, ensure the change is meaningful.
- If the diff is overly extensive, explicitly mention that it's too large for effective review.

Examples of review comments:
- Good: {"lineNumber": 42, "reviewComment": "Potential nil pointer dereference. Please check if 'user' is nil before accessing its fields."}
- Good: {"lineNumber": 10, "reviewComment": "Consider using a constant for the retry interval to improve maintainability."}
- Good: {"lineNumber": 27, "reviewComment": "The replica count 2 for scaling up in the update-envs step is hardcoded. Consider making this a parameter (e.g., target_replicas) to provide more flexibility and reusability for the workflow, allowing different target replica counts without modifying the workflow definition."}
- Bad: {"lineNumber": 5, "reviewComment": "Add more comments to the code."} (Do NOT suggest this)
- Bad: {"lineNumber": 12, "reviewComment": "Looks fine."} (Be specific and actionable)

Pull Request Title: %s

Pull Request Description:
---
%s
---

Git Diff to Review:
---diff
%s
---
`, filePath, pr.Title, pr.Description, strings.Join(hunkLines, "\n"))

}

func GetAIResponseOfGemini(prompt string, geminiKey, geminiModel string) ([]model.ReviewComment, error) {
	// Gemini API endpoint (v1beta/models/gemini-2.0-flash-001:generateContent)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", geminiModel, geminiKey)
	payload := map[string]interface{}{
		"contents": []map[string]interface{}{{"parts": []map[string]string{{"text": prompt}}}},
		"generationConfig": map[string]interface{}{
			"maxOutputTokens": 8192,
			"temperature":     0.8,
			"topP":            0.95,
		},
	}
	b, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", strings.NewReader(string(b)))
	if err != nil {
		log.Error(err)
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Error(err)
		return nil, err
	}
	// Extract text
	var text string
	if c, ok := result["candidates"].([]interface{}); ok && len(c) > 0 {
		if content, ok := c[0].(map[string]interface{})["content"].(map[string]interface{}); ok {
			if parts, ok := content["parts"].([]interface{}); ok && len(parts) > 0 {
				text = parts[0].(map[string]interface{})["text"].(string)
			}
		}
	}
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
	}
	if strings.HasSuffix(text, "```") {
		text = strings.TrimSuffix(text, "```")
	}
	text = strings.TrimSpace(text)
	var respObj model.ReviewResponse
	if err := json.Unmarshal([]byte(text), &respObj); err != nil {
		log.Error(err)
		log.Error("Debug raw data form AI" + text)
		return nil, err
	}
	var comments []model.ReviewComment
	for _, r := range respObj.Reviews {
		comments = append(comments, model.ReviewComment{
			Body:     r.ReviewComment,
			Path:     "", // to be filled by caller
			Position: r.LineNumber,
		})
	}
	return comments, nil
}
