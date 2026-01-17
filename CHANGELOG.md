# Changelog

## 0.15.0

### Added
- Review only new commits after the last bot review, using commit-aware tracking.
- LGTM pause: a non-bot "LGTM" comment stops both summary and inline reviews on a PR.
- Delta-diff fallback: if commit-to-commit diff is empty, fall back to full PR diff.
- Bot marker (`<!-- auto-review-bot -->`) to reliably detect bot comments when accounts are shared.
- Detailed inline-review diagnostics to explain why zero inline comments were posted.

### Changed
- Inline review suppression now relies on explicit LGTM only, not reviewer content.
- Summary comments include a hidden marker with the latest reviewed commit hash.

