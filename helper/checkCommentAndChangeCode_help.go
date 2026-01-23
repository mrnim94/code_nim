package helper

import (
	"strconv"
	"strings"
)

// LooksLikeCommand checks if the comment body looks like a shell command or user command
func LooksLikeCommand(body string) bool {
	// Add more sophisticated checks as needed
	commandIndicators := []string{"$", "#!/bin/", "sudo ", "rm ", "ls ", "cd ", "echo ", "cat ", "touch ", "mkdir ", "curl ", "wget ", "python ", "go run", "npm ", "yarn ", "git ", "exit", "shutdown", "reboot"}
	for _, indicator := range commandIndicators {
		if len(body) >= len(indicator) && body[:len(indicator)] == indicator {
			return true
		}
	}
	return false
}

// DiffLineMapping holds both source (from) and destination (to) line numbers for a diff line
type DiffLineMapping struct {
	FromLine int // Line number in the old/source file (-1 for added lines)
	ToLine   int // Line number in the new/destination file (-1 for deleted lines)
}

// BuildDiffSnippetAndLineMap flattens hunks for the AI prompt and builds a mapping
// from snippet index (1-based in AI output) to both source and destination file lines.
// For lines not present on destination (deleted '-' lines), ToLine is -1.
// For lines not present on source (added '+' lines), FromLine is -1.
func BuildDiffSnippetAndLineMap(hunks []map[string]interface{}) ([]string, []DiffLineMapping) {
	var snippet []string
	var lineMap []DiffLineMapping
	for _, h := range hunks {
		header, _ := h["header"].(string)
		lines, _ := h["lines"].([]string)
		// Parse header like: @@ -a,b +c,d @@
		// Extract a (start line on source) and c (start line on destination)
		srcStart := 0
		destStart := 0

		// Parse source start line from "-a,b" part
		if parts := strings.Split(header, "-"); len(parts) > 1 {
			left := parts[1]
			if idx := strings.IndexAny(left, " ,+@"); idx >= 0 {
				left = left[:idx]
			}
			if v, err := strconv.Atoi(strings.TrimSpace(left)); err == nil {
				srcStart = v
			}
		}

		// Parse destination start line from "+c,d" part
		if parts := strings.Split(header, "+"); len(parts) > 1 {
			right := parts[1]
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

		srcLine := srcStart
		destLine := destStart
		for _, ln := range lines {
			snippet = append(snippet, ln)
			if strings.HasPrefix(ln, "+") {
				// Added line: exists only in destination
				lineMap = append(lineMap, DiffLineMapping{FromLine: -1, ToLine: destLine})
				destLine++
			} else if strings.HasPrefix(ln, "-") {
				// Deleted line: exists only in source
				lineMap = append(lineMap, DiffLineMapping{FromLine: srcLine, ToLine: -1})
				srcLine++
			} else {
				// Context line: exists in both source and destination
				lineMap = append(lineMap, DiffLineMapping{FromLine: srcLine, ToLine: destLine})
				srcLine++
				destLine++
			}
		}
	}
	return snippet, lineMap
}

// FormatSummaryBody enforces newlines around headers and bullets for PR summary
func FormatSummaryBody(body string) string {
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

// FormatReviewBody enforces proper markdown formatting with paragraph breaks for better rendering
func FormatReviewBody(body string) string {
	if body == "" {
		return body
	}

	// List of headings that should start on new paragraphs
	headings := []string{
		"Why:",
		"How (step-by-step):",
		"Suggested change (Before/After):",
		"Prompt for AI Agents (optional):",
	}

	formatted := body

	// Use double newlines for proper markdown paragraph breaks
	for _, heading := range headings {
		// Replace " Heading:" with proper paragraph break
		spacedHeading := " " + heading
		properHeading := "\n\n" + heading
		formatted = strings.ReplaceAll(formatted, spacedHeading, properHeading)

		// Handle cases where heading appears without preceding space
		formatted = strings.ReplaceAll(formatted, heading, properHeading)
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

// NearestMatchingLineIndex finds the nearest index in diffLines whose content contains the anchor.
// It searches first at the hinted index, then walks outward.
func NearestMatchingLineIndex(diffLines []string, anchor string, hintIdx int) int {
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
