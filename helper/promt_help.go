package helper

import (
	"code_nim/log"
	"code_nim/model"
	"encoding/json"
	"fmt"
	"io"
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

// escapeControlCharsInJSONString escapes invalid control characters inside JSON string literals.
// It only transforms characters that appear inside quoted strings.
func escapeControlCharsInJSONString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)
	inString := false
	backslashes := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' {
			backslashes++
			b.WriteByte(ch)
			continue
		}
		if ch == '"' {
			if backslashes%2 == 0 {
				inString = !inString
			}
			backslashes = 0
			b.WriteByte(ch)
			continue
		}
		if backslashes > 0 {
			backslashes = 0
		}
		if inString {
			switch ch {
			case '\t':
				b.WriteString("\\t")
				continue
			case '\r':
				b.WriteString("\\r")
				continue
			case '\n':
				b.WriteString("\\n")
				continue
			default:
				if ch < 0x20 { // other control chars
					b.WriteString("\\u00")
					hex := fmt.Sprintf("%02x", ch)
					b.WriteString(hex)
					continue
				}
			}
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func CreatePrompt(filePath string, hunkLines []string, pr *model.PullRequest) string {
	log.Debugf("Begin to Create Prompt for PR: %d", pr.ID)
	return fmt.Sprintf(`You are an expert code reviewer. Please follow these instructions carefully:

- Provide your feedback strictly in the following JSON format:
  {"reviews": [{"lineNumber": <diff_line_index>, "lineText": "<exact line snippet>", "reviewComment": "<comment>"}]}

- Review the unified diff for file "%s" below. The lineNumber refers to the 1-based index of the displayed diff lines (including context and +/- lines). Do not use absolute file line numbers. Also include the exact line text (lineText) you are referring to from the diff to help anchor placement.
- Your reviewComment must be actionable like CodeRabbit. Use this structure:
  [<Type: Potential issue|Refactor|Nitpick>] [<Severity: Critical|Major|Minor|Trivial|Info>]
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
  Prompt for AI Agents (optional):
    - Provide a concise, copy/paste prompt that another AI agent can execute.
    - Include file path(s), exact area, and the requested change in imperative form.
    - Keep it actionable and scoped to the current review comment.

- Focus your comments on code quality, bugs, logic errors, security, performance, and best practices.
- SECURITY: Strongly prioritize detection of secrets or easy-to-reverse encodings (e.g., base64) committed to the repo.
  - Treat as [Potential issue] [Major] or [Critical] depending on leak severity.
  - GLOBAL heuristics (apply to ALL languages and file types, not only Kubernetes):
    - Suspicious key names (case-insensitive): password|passwd|pwd|secret|token|api[_-]?key|client[_-]?secret|private[_-]?key|access[_-]?key|accessKeyId|secretAccessKey|ssh[_-]?key|jwt|bearer|webhook|credential|METRICS_AUTH_.*.
    - PEM/SSH material: lines containing "-----BEGIN (RSA|EC|OPENSSH|PRIVATE|CERTIFICATE) KEY-----".
    - Provider patterns (examples): AWS AKIA/ASIA keys, GitHub tokens (ghp_|gho_|github_pat_), Stripe (sk_live_/pk_live_), GCP API keys (AIza...), Slack (xox[baprs]-...), and similar well-known formats.
    - JWTs: three base64url segments separated by '.' with plausible length.
    - Base64-like strings: ^[A-Za-z0-9_\\-+/]{14,}={0,2}$ that decode to mostly printable ASCII (usernames, hostnames, JSON, URLs).
    - Long hex secrets: 32/40/64 hex chars that look like keys or HMACs.
    - Connection strings/DSNs embedding credentials: scheme://user:pass@host:port/..., database URLs, SMTP creds.
    - .env or pipeline/config YAML/JSON with literal secrets or tokens.
    - Filenames/paths indicating credentials: secret(s), credential(s), token, key, .env, config/secrets, etc.
    - Avoid false positives: allow clear placeholders like <PASSWORD>, example, dummy, test-only, or obviously encrypted blobs (age, PGP, SOPS content).
  - Kubernetes-specific (when applicable):
    - Manifests with kind: Secret under "data:" or "stringData:"; caution that base64 is not encryption.
  - What to recommend (language-agnostic):
    - Never commit real secrets to VCS; rotate any leaked credential immediately.
    - Move secrets to a secret manager (Vault, AWS/GCP/Azure secrets, Doppler, 1Password) or use sealed/encrypted-at-rest mechanisms (SOPS/SealedSecrets).
    - Inject secrets at runtime (env vars, mounted files) via CI/CD or platform-level secret injection; keep placeholders in code/config.
    - Replace hard-coded credentials with references to secret variables; add pre-commit/CI scanners and update .gitignore to avoid committing generated secret files.
    - Where relevant (Kubernetes), prefer External Secrets or Secrets Store CSI; if storing manifests, store only encrypted material.
  - Your review comment should include concrete Before/After examples in the file’s native format (code, YAML, JSON, .env) illustrating a safe pattern.
- Maybe Refactor the following code to improve readability, maintainability, and efficiency. Please ensure the logic remains unchanged.
- Use clear, concise GitHub Markdown in your comments.
- ONLY provide feedback if improvements are necessary; if the code is optimal, return an empty "reviews" array.
- IMPORTANT: Do NOT suggest adding comments to the code.
- IMPORTANT: Do NOT suggest adding a trailing newline at the end of file. This is a trivial formatting issue handled by editors/linters and does not need review feedback.
- Assess whether the changes align with the pull request's title and description.
- If the PR is too large, suggest breaking it down; if very small, ensure the change is meaningful.
- If the diff is overly extensive, explicitly mention that it's too large for effective review.

Examples of review comments:
- {"lineNumber": 61, "lineText": "+ func matchesEngineID(deploymentName string, engineID string) bool {", "reviewComment": "[Refactor] [Minor] Boundary-safe engine ID matching\nWhy:\n  - strings.Contains(name, suffix- may match unintended names (e.g., dlp vs adlp).\nHow (step-by-step):\n  - Ensure an ID matches only at word/hyphen boundary or end-of-name.\nSuggested change (Before/After):\n~~~go\n// Before\nreturn strings.Contains(deploymentName, engineID+\"-\") || strings.HasSuffix(deploymentName, engineID)\n~~~\n~~~go\n// After\nre := regexp.MustCompile((^|-)" + "" + "" + " + regexp.QuoteMeta(engineID) + "$")\nreturn re.MatchString(deploymentName)\n~~~\nPrompt for AI Agents:\n  - In the file containing matchesEngineID, replace the strings.Contains/HasSuffix logic with a boundary-safe check (regex or equivalent), and keep behavior identical for existing callers."}
- {"lineNumber": 7, "lineText": "+   METRICS_AUTH_PASSWORD: MTIzNDU2", "reviewComment": "[Potential issue] [Critical] Base64-encoded credential committed to repo\n+Why:\n+  - Base64 is reversible and provides no secrecy; anyone can decode the value.\n+  - Committing real secrets risks unauthorized access if reused elsewhere.\n+How (step-by-step):\n+  - Rotate this credential immediately.\n+  - Replace the literal value with a reference to a secret manager variable injected at runtime.\n+  - Add CI scanning to block future secret commits.\n+Suggested change (Before/After):\n+~~~yaml\n+# Before\ndata:\n  METRICS_AUTH_PASSWORD: MTIzNDU2\n+~~~\n+~~~yaml\n+# After (generic example)\n# Use runtime-injected env or a reference to your secret manager\nenv:\n  - name: METRICS_AUTH_PASSWORD\n    valueFrom:\n      secretKeyRef:\n        name: metrics-auth\n        key: password\n+~~~\n+Prompt for AI Agents:\n+  - In the YAML file where METRICS_AUTH_PASSWORD is set, replace the literal with a secret reference and ensure runtime injection; remove the base64 value."}

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

// CreateSummaryPrompt builds a prompt that asks the AI to summarize the PR in
// a CodeRabbit-like style with grouped bullets.
func CreateSummaryPrompt(pr *model.PullRequest, diff string) string {
	log.Debugf("Create Summary Prompt for PR: %d", pr.ID)
	return fmt.Sprintf(`You are an expert code reviewer.

Produce a PR overview in Markdown that contains EXACTLY these sections, in this order:

## Summary
Output a grouped bullet list matching CodeRabbit style. Use EXACTLY this Markdown structure with blank lines between sections.

REQUIRED FORMAT:

**New Features**

- <Item text starting with a verb, ending with period.>
- <Item text starting with a verb, ending with period.>

**Bug Fixes**

- <Item text starting with a verb, ending with period.>

**Documentation**
- <Item text starting with a verb, ending with period.>

**Refactor**

- <Item text starting with a verb, ending with period.>

**Performance**

- <Item text starting with a verb, ending with period.>

**Tests**

- <Item text starting with a verb, ending with period.>

**Chores**

- <Item text starting with a verb, ending with period.>

Critical rules:
- Section headers are standalone bold lines (no list bullet): "**Section Name**".
- Each item is a single-level hyphen bullet with EXACTLY one hyphen and one space: "- Item text" (no icons like ○, •, ✔, or emojis).
- Keep a blank line before and after each section header for visual separation.
- 2-6 items per populated section.
- Each item ≤ 140 chars; start with verb, end with period.
- Omit empty sections completely.

## Walkthrough
A short paragraph (3-6 sentences) explaining the overall intent of the change and major areas touched.

## Changes
A compact table with two columns: Cohort / File(s) | Change Summary. Group related files by directory or purpose. Keep each summary to one short sentence.

## Sequence Flow
Provide a concise, plain Markdown numbered list that describes the most important end-to-end steps affected by this PR.

Plain output rules:
- Use an ordered list (1., 2., 3., ...). One step per line.
- Format each step as: Actor -> Target: short action/result (≤ 80 chars).
- Keep between 6 and 12 steps. Prefer high-signal actions; avoid noise.
- If a step is conditional, prefix briefly in parentheses, e.g.: (if samba-server) delete/update smb-storage-class.


Style rules:
- Be concise and high-signal; avoid repetition.
- Use verbs at the start of bullets and summaries.
- No shell commands.
- Output must be valid Markdown.

Pull Request Title: %s

Pull Request Description:
---
%s
---

Unified Git Diff:
---diff
%s
---
`, pr.Title, pr.Description, diff)
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
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimSuffix(text, "```")
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

// getGeminiText returns the raw text response from Gemini for a given prompt.
func getGeminiText(prompt string, geminiKey, geminiModel string) (string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", geminiModel, geminiKey)
	payload := map[string]interface{}{
		"contents": []map[string]interface{}{{"parts": []map[string]string{{"text": prompt}}}},
		"generationConfig": map[string]interface{}{
			"maxOutputTokens": 2048,
			"temperature":     0.4,
			"topP":            0.95,
		},
	}
	b, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", strings.NewReader(string(b)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var errorResult model.GeminiErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&errorResult)
		return "", fmt.Errorf("gemini status %d: %s", resp.StatusCode, errorResult.Error.Message)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	var text string
	if c, ok := result["candidates"].([]interface{}); ok && len(c) > 0 {
		if content, ok := c[0].(map[string]interface{})["content"].(map[string]interface{}); ok {
			if parts, ok := content["parts"].([]interface{}); ok && len(parts) > 0 {
				if t, ok := parts[0].(map[string]interface{})["text"].(string); ok {
					text = t
				}
			}
		}
	}
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```markdown")
	text = strings.TrimPrefix(text, "```md")
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimSuffix(text, "```")
	return strings.TrimSpace(text), nil
}

// GetAISummary returns a Markdown summary text via the configured provider.
func GetAISummary(prompt string, cfg *model.AutoReviewPR) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.AIProvider))
	modelName := strings.TrimSpace(cfg.AIModel)
	if modelName == "" {
		modelName = strings.TrimSpace(cfg.GeminiModel)
	}
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}
	log.Debugf("Getting AI summary for provider: %s and model %s", provider, modelName)

	switch provider {
	case "self":
		base := strings.TrimSpace(cfg.SelfAPIBaseURL)
		if base == "" {
			return "", fmt.Errorf("selfApiBaseUrl is required when aiProvider=self")
		}
		// Call self API directly to get text (avoid JSON-review path/logging)
		base = strings.TrimRight(base, "/")
		url := fmt.Sprintf("%s/v1beta/models/%s", base, modelName)
		b, _ := json.Marshal(map[string]interface{}{
			"contents": []map[string]interface{}{{"parts": []map[string]string{{"text": prompt}}}},
		})
		log.Debugf("Calling self API for summary at: %s", url)
		resp, err := http.Post(url, "application/json", strings.NewReader(string(b)))
		if err != nil {
			log.Errorf("Self API HTTP error: %v", err)
			return "", err
		}
		defer resp.Body.Close()
		rawBody, _ := io.ReadAll(resp.Body)
		log.Debugf("Self API raw response (first 500 chars): %s", string(rawBody)[:min(500, len(rawBody))])

		// Try JSON path first
		var obj map[string]interface{}
		if json.Unmarshal(rawBody, &obj) == nil {
			var text string
			if c, ok := obj["candidates"].([]interface{}); ok && len(c) > 0 {
				if content, ok := c[0].(map[string]interface{})["content"].(map[string]interface{}); ok {
					if parts, ok := content["parts"].([]interface{}); ok && len(parts) > 0 {
						if t, ok := parts[0].(map[string]interface{})["text"].(string); ok {
							text = t
							log.Debugf("Extracted text from candidates path, length: %d", len(text))
						}
					}
				}
			}
			if text == "" {
				if t, ok := obj["text"].(string); ok {
					text = t
					log.Debugf("Extracted text from root 'text' field, length: %d", len(text))
				}
			}
			if text == "" {
				log.Warnf("Could not extract text from JSON response structure")
			}
			text = strings.TrimSpace(text)
			text = strings.TrimPrefix(text, "```markdown")
			text = strings.TrimPrefix(text, "```md")
			text = strings.TrimPrefix(text, "```json")
			text = strings.TrimSuffix(text, "```")
			finalText := strings.TrimSpace(text)
			log.Debugf("Returning summary text, final length: %d", len(finalText))
			return finalText, nil
		}
		// Fallback: treat body as text
		log.Debugf("JSON unmarshal failed, treating response as plain text")
		t := strings.TrimSpace(string(rawBody))
		t = strings.TrimPrefix(t, "```markdown")
		t = strings.TrimPrefix(t, "```md")
		t = strings.TrimPrefix(t, "```json")
		t = strings.TrimSuffix(t, "```")
		finalText := strings.TrimSpace(t)
		log.Debugf("Returning plain text summary, final length: %d", len(finalText))
		return finalText, nil
	default:
		// Gemini
		return getGeminiText(prompt, strings.TrimSpace(cfg.GeminiKey), modelName)
	}
}

// GetAIResponse routes to the configured AI provider. Defaults to Gemini.
func GetAIResponse(prompt string, cfg *model.AutoReviewPR) ([]model.ReviewComment, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.AIProvider))
	// Resolve model and key (generic first, then Gemini-specific, then default model)
	modelName := strings.TrimSpace(cfg.AIModel)
	if modelName == "" {
		modelName = strings.TrimSpace(cfg.GeminiModel)
	}
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}

	apiKey := strings.TrimSpace(cfg.AIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(cfg.GeminiKey)
	}

	switch provider {
	case "self":
		base := strings.TrimSpace(cfg.SelfAPIBaseURL)
		if base == "" {
			log.Error("AI provider is 'self' but selfApiBaseUrl is empty")
			return nil, fmt.Errorf("selfApiBaseUrl is required when aiProvider=self")
		}
		log.Debugf("Using AI provider=self, base=%s, model=%s", base, modelName)
		return getAIResponseOfSelf(prompt, base, modelName)
	default:
		// Gemini
		log.Debugf("Using AI provider=gemini, model=%s", modelName)
		return GetAIResponseOfGemini(prompt, apiKey, modelName)
	}
}

// getAIResponseOfSelf calls a self-hosted AI API that mimics Gemini's content API.
// Expected endpoint form: {base}/v1beta/models/{model}
func getAIResponseOfSelf(prompt string, baseURL, modelName string) ([]model.ReviewComment, error) {
	base := strings.TrimRight(baseURL, "/")
	url := fmt.Sprintf("%s/v1beta/models/%s", base, modelName)

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{{"parts": []map[string]string{{"text": prompt}}}},
		// Keep generationConfig for compatibility; self API may ignore extra fields
		"generationConfig": map[string]interface{}{
			"maxOutputTokens": 8192,
			"temperature":     0.8,
			"topP":            0.95,
		},
	}
	b, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", strings.NewReader(string(b)))
	if err != nil {
		log.Errorf("Failed to call self AI API: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	rawBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Failed to read self AI API response body: %v", readErr)
		return nil, readErr
	}
	if resp.StatusCode != 200 {
		log.Errorf("Self AI API returned status %d, raw body (first 500 chars): %s", resp.StatusCode, string(rawBody)[:min(500, len(rawBody))])
		return nil, fmt.Errorf("self AI API returned status %d", resp.StatusCode)
	}

	// Parse successful response (try to follow Gemini schema, but be tolerant)
	log.Debugf("Self AI raw body (first 500 chars): %s", string(rawBody)[:min(500, len(rawBody))])
	var result map[string]interface{}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		// If body is not JSON object, treat as plain text carrier
		log.Debugf("Self AI response not a JSON object; will attempt plain text extraction")
		result = map[string]interface{}{}
	}

	var text string
	// Preferred: Gemini-like schema
	if c, ok := result["candidates"].([]interface{}); ok && len(c) > 0 {
		if content, ok := c[0].(map[string]interface{})["content"].(map[string]interface{}); ok {
			if parts, ok := content["parts"].([]interface{}); ok && len(parts) > 0 {
				if t, ok := parts[0].(map[string]interface{})["text"].(string); ok {
					text = t
				}
			}
		}
	}
	// Fallback: some self APIs may just return {"text": "..."}
	if text == "" {
		if t, ok := result["text"].(string); ok {
			text = t
		}
	}
	// Fallback: treat whole body as text
	if text == "" {
		text = strings.TrimSpace(string(rawBody))
	}
	text = strings.TrimSpace(text)
	// If response includes a preamble and fenced JSON, extract fenced JSON
	if strings.Contains(text, "```json") {
		start := strings.Index(text, "```json")
		rest := text[start+len("```json"):]
		if end := strings.Index(rest, "```"); end >= 0 {
			text = rest[:end]
		} else {
			text = rest
		}
	}
	// Remove any lingering fences and trim
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	// If still has preamble, try to slice from first '{' to last '}'
	if !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		if idx := strings.Index(text, "{"); idx >= 0 {
			candidate := text[idx:]
			if end := strings.LastIndex(candidate, "}"); end >= 0 {
				text = candidate[:end+1]
			}
		}
	}

	if text == "" {
		log.Error("Self AI API returned empty text response")
		return []model.ReviewComment{}, nil
	}
	if !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		log.Errorf("Self AI API response is not JSON (first 200 chars): %s", text[:min(200, len(text))])
		log.Debugf("Self AI extracted text (first 500 chars): %s", text[:min(500, len(text))])
		return []model.ReviewComment{}, nil
	}

	// Sanitize control characters inside JSON string literals (e.g., literal tabs)
	sanitized := escapeControlCharsInJSONString(text)
	log.Debugf("Parsing self AI API response JSON (length: %d)", len(sanitized))

	var respObj model.ReviewResponse
	if err := json.Unmarshal([]byte(sanitized), &respObj); err != nil {
		log.Errorf("Failed to parse JSON from self AI API: %v", err)
		log.Errorf("Raw AI response (first 500 chars): %s", text[:min(500, len(text))])
		return []model.ReviewComment{}, nil
	}
	var comments []model.ReviewComment
	for _, r := range respObj.Reviews {
		anchor := strings.TrimSpace(r.LineText)
		comments = append(comments, model.ReviewComment{
			Body:     r.ReviewComment,
			Path:     "",
			Position: r.LineNumber,
			Anchor:   anchor,
		})
	}
	return comments, nil
}
