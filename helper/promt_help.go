package helper

import (
	"code_nim/log"
	"code_nim/model"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Helper functions for min/max operations
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func CreatePrompt(filePath string, hunkLines []string, pr *model.PullRequest) string {
	log.Debugf("Begin to Create Prompt for PR: %d", pr.ID)
	return fmt.Sprintf(`You are an expert code reviewer. Please follow these instructions carefully:

- Provide your feedback strictly in the following JSON format:
  {"reviews": [{"lineNumber": <diff_line_index>, "lineText": "<exact line snippet>", "reviewComment": "<comment>"}]}

- Review the unified diff for file "%s" below. The lineNumber refers to the 1-based index of the displayed diff lines (including context and +/- lines). Do not use absolute file line numbers. Also include the exact line text (lineText) you are referring to from the diff to help anchor placement.
- Your reviewComment must be actionable like CodeRabbit. Use this structure:
  [<Severity: nit|minor|medium|major|critical>] [<Category: bug|performance|security|style|readability|maintainability>]
  <Short title in one sentence>
  Why:
    - <1-2 bullets on reasoning/risks>
  How (step-by-step):
    - <Precise steps to change code>
  Suggested change (Before/After):
    ~~~go
    // Before
    <minimal relevant snippet>
    ~~~
    ~~~go
    // After
    <minimal relevant snippet with improvement>
    ~~~
  Notes (optional):
    - <edge cases, tests, follow-ups>

- Focus your comments on code quality, bugs, logic errors, security, performance, and best practices.
- Maybe Refactor the following code to improve readability, maintainability, and efficiency. Please ensure the logic remains unchanged.
- Use clear, concise GitHub Markdown in your comments.
- ONLY provide feedback if improvements are necessary; if the code is optimal, return an empty "reviews" array.
- IMPORTANT: Do NOT suggest adding comments to the code.
- Assess whether the changes align with the pull request's title and description.
- If the PR is too large, suggest breaking it down; if very small, ensure the change is meaningful.
- If the diff is overly extensive, explicitly mention that it's too large for effective review.

Examples of review comments:
- {"lineNumber": 61, "lineText": "+ func matchesEngineID(deploymentName string, engineID string) bool {", "reviewComment": "[minor] [readability] Boundary-safe engine ID matching\nWhy:\n  - strings.Contains(name, suffix- may match unintended names (e.g., dlp vs adlp).\nHow (step-by-step):\n  - Ensure an ID matches only at word/hyphen boundary or end-of-name.\nSuggested change (Before/After):\n~~~go\n// Before\nreturn strings.Contains(deploymentName, engineID+\"-\") || strings.HasSuffix(deploymentName, engineID)\n~~~\n~~~go\n// After\nre := regexp.MustCompile((^|-)" + "" + "" + " + regexp.QuoteMeta(engineID) + "$")\nreturn re.MatchString(deploymentName)\n~~~\nNotes:\n  - Precompile the regex outside loops for performance; or implement non-regex boundary checks."}

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
		log.Errorf("Failed to make request to Gemini API: %v", err)
		return nil, err
	}
	defer resp.Body.Close()
	
	// Check HTTP status code first
	if resp.StatusCode != 200 {
		var errorResult model.GeminiErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResult); err != nil {
			log.Errorf("Failed to decode error response from Gemini API (status %d): %v", resp.StatusCode, err)
			return nil, fmt.Errorf("gemini API returned status %d", resp.StatusCode)
		}
		
		// Handle specific error types using the structured response
		code := errorResult.Error.Code
		message := errorResult.Error.Message
		status := errorResult.Error.Status
		
		switch code {
		case 429:
			log.Errorf("Gemini API rate limit exceeded: %s", message)
			log.Error("Please check your API quota and billing details")
			log.Error("For more information: https://ai.google.dev/gemini-api/docs/rate-limits")
			return nil, fmt.Errorf("gemini API rate limit exceeded: %s", message)
		case 401:
			log.Errorf("Gemini API authentication failed: %s", message)
			log.Error("Please check your API key")
			return nil, fmt.Errorf("gemini API authentication failed: %s", message)
		case 403:
			log.Errorf("Gemini API access forbidden: %s", message)
			log.Error("Please check your API permissions and billing")
			return nil, fmt.Errorf("gemini API access forbidden: %s", message)
		case 400:
			log.Errorf("Gemini API bad request: %s", message)
			log.Error("Please check your request parameters and model name")
			return nil, fmt.Errorf("gemini API bad request: %s", message)
		default:
			log.Errorf("Gemini API error (code %d, status %s): %s", code, status, message)
			return nil, fmt.Errorf("gemini API error: %s", message)
		}
	}
	
	// Parse successful response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Errorf("Failed to decode successful response from Gemini API: %v", err)
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
	
	// Add validation and better error handling for JSON parsing
	if text == "" {
		log.Error("Received empty response from AI")
		return []model.ReviewComment{}, nil // Return empty slice instead of error
	}
	
	// Check if the response looks like JSON
	if !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		log.Errorf("AI response doesn't appear to be JSON. First 100 chars: %s", 
			text[:min(100, len(text))])
		return []model.ReviewComment{}, nil // Return empty slice instead of error
	}
	
	// Log the full response for debugging when JSON parsing fails
	log.Debugf("Attempting to parse AI response JSON (length: %d)", len(text))
	
	var respObj model.ReviewResponse
	if err := json.Unmarshal([]byte(text), &respObj); err != nil {
		log.Errorf("Failed to parse JSON from AI response: %v", err)
		log.Errorf("Raw AI response (first 500 chars): %s", text[:min(500, len(text))])
		log.Errorf("Raw AI response (last 200 chars): %s", text[max(0, len(text)-200):])
		
		// Try to check if JSON is just incomplete by looking for common patterns
		if strings.Contains(text, `"reviews"`) && !strings.HasSuffix(text, "}") {
			log.Error("AI response appears to be incomplete JSON (missing closing brace)")
		}
		
		// Return empty slice instead of error to allow processing to continue
		return []model.ReviewComment{}, nil
	}
	var comments []model.ReviewComment
	for _, r := range respObj.Reviews {
		anchor := strings.TrimSpace(r.LineText)
		comments = append(comments, model.ReviewComment{
			Body:     r.ReviewComment,
			Path:     "", // to be filled by caller
			Position: r.LineNumber,
			Anchor:   anchor,
		})
	}
	return comments, nil
}
