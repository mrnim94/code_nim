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
	return fmt.Sprintf(`You are an expert code reviewer. Produce concise, actionable feedback that is specific, not generic.

STRICT OUTPUT FORMAT
- Return ONLY valid JSON: {"reviews": [{"lineNumber": <int>, "reviewComment": "<markdown>"}]}
- No extra text before/after JSON. Do not wrap JSON in code fences.

SCOPE AND STYLE
- Review ONLY the diff for file "%s" in the context of the PR title and description.
- Focus on correctness, security, performance, readability, maintainability, and edge cases.
- If no meaningful improvement exists, return {"reviews": []}.
- Do not suggest adding comments to code. Avoid generic praise.

REVIEW ITEM REQUIREMENTS
Each review object's "reviewComment" must include:
1) [Severity: Low|Medium|High] One-line summary
2) Evidence: quote the exact changed line(s) or a tiny snippet from the diff (use inline code markers)
3) Why: concrete risk/impact (bug, security risk, performance, maintainability)
4) Fix: a specific change (a minimal diff-like snippet or concise replacement)
5) Optional: brief reference (standard/best-practice) if truly helpful

ADDITIONAL RULES
- Choose "lineNumber" as the most relevant changed line for the issue.
- Only include reviews that contain BOTH Evidence and Fix; otherwise omit the review.
- Keep each review under ~120 words and prefer minimal snippets.
- If the diff is too large to review effectively, return a single High severity item asking to split it logically.

EXAMPLES (ILLUSTRATIVE)
- Good: {"lineNumber": 42, "reviewComment": "[High] Possible nil dereference\n\nEvidence: access to user.Name after user := find(...) with no nil-check.\n\nWhy: may panic at runtime when find returns nil.\n\nFix: add guard:\nif user == nil { return errUserNotFound }\nname := user.Name"}
- Good: {"lineNumber": 27, "reviewComment": "[Medium] Hardcoded replica count\n\nEvidence: replicas: 2 in deploy step.\n\nWhy: inflexible across environments.\n\nFix: parameterize via target_replicas input and default per env."}
- Bad: {"lineNumber": 5, "reviewComment": "Looks fine"}
- Bad: {"lineNumber": 12, "reviewComment": "Add comments for readability"}

PR TITLE: %s

PR DESCRIPTION:
---
%s
---

GIT DIFF TO REVIEW:
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
