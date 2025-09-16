# code-nim

An automated Pull Request reviewer for Bitbucket using Google's Gemini. It periodically scans configured repositories, fetches open PRs, generates structured AI review comments from diffs, and posts them back to Bitbucket. Built with Go, Echo, Logrus, and gocron.

### Features
- **Bitbucket PR scanning**: Lists open PRs for configured repos.
- **Diff parsing**: Parses PR diffs per file and hunk.
- **AI reviews with Gemini**: Crafts a review prompt and parses JSON responses into actionable comments.
- **Idempotent commenting**: Skips PRs where configured users have already commented.
- **Author filtering**: Ignore PRs from specific authors (useful for skipping senior developers or specific team members).
- **Inline comments**: Posts AI-generated comments directly on specific lines in the code diff.
- **Command filtering**: Automatically filters out AI responses that look like shell commands or code execution.
- **Scheduling**: Runs via cron specs using gocron.
- **Structured logging**: Rotating file logs with detailed PR processing information.

### Requirements
- Go 1.23+
- Bitbucket account with an App Password that can read PRs and post comments
- Google Generative Language API key (Gemini)

### Quick start
1. Clone the repo and install dependencies:
```bash
git clone <this-repo>
cd code_nim
go mod download
```
2. Configure your jobs in `config_file/review-config.yaml` (see Configuration below).
3. Build and run:
```bash
go run .
# or
go build -o code-nim
./code-nim
```
The service starts an Echo server on port `1994` and launches schedulers in the background.

### Configuration
Edit `config_file/review-config.yaml` to define one or more review tasks under `autoReviewPR`.

```yaml
autoReviewPR:
  - processName: my-reviews
    cron: "*/5 * * * *" # every 5 minutes
    gitProvider: bitbucket
    workspace: <your-workspace>
    repoSlug: <your-repo>
    displayNames:
      - <Your Name>
      - <Bot Display Name>
    username: <bitbucket-username>
    appPassword: <bitbucket-app-password>
    geminiKey: <google-generative-language-api-key>
    ignorePullRequestOf:
      displayNames:
        - Senior Developer Name
        - Lead Architect Name
```

**Configuration Fields:**
- **processName**: Identifier for the scheduled job.
- **cron**: Cron expression in UTC for how often to scan and review.
- **workspace**: Bitbucket workspace.
- **repoSlug**: Repository slug.
- **displayNames**: Display names that count as "already reviewed" and prevent duplicate AI comments.
- **username/appPassword**: Used for Bitbucket Basic Auth. App Password must allow reading PRs and posting comments.
- **geminiKey**: API key for Gemini models.
- **ignorePullRequestOf.displayNames**: *(Optional)* List of author display names whose PRs should be completely ignored and not reviewed by AI.

**⚠️ SECURITY WARNING**: Never commit real credentials to version control! Use environment variables or a secrets management system in production.

Notes:
- The service checks recent comments on a PR; if the configured `username` or any name in `displayNames` has a non-empty comment, it will skip posting AI comments.
- By default it uses model `gemini-2.5-flash` in `helper.GetAIResponseOfGemini`.

### How it works
1. **Initialization**: `main.go` initializes logging, sets timezone to `Asia/Ho_Chi_Minh`, builds the Bitbucket client, registers the scheduler via `handler.AutoReviewPRHandler`, and starts Echo on `:1994`.

2. **Scheduled Review Process**: `handler/autoReviewPR_handler.go` loads the YAML config, sets up cron jobs, and for each scheduled run:
   - Fetches all open PRs from the configured repository
   - Logs detailed information about each PR being processed
   - **Author Filtering**: Checks if PR author is in the `ignorePullRequestOf.displayNames` list and skips if found
   - **Duplicate Prevention**: Checks existing comments to avoid re-reviewing PRs where the bot has already commented
   - Fetches PR diff and parses it into structured data
   - Generates AI review comments using Gemini
   - **Command Filtering**: Filters out AI responses that look like shell commands
   - Posts inline comments directly on the relevant code lines

3. **Components**:
   - `helper/atlassian/bitbucket_impl`: Bitbucket HTTP client methods for PRs, diffs, comments, and diff parsing
   - `helper/promt_help.go`: Builds the Gemini prompt and parses model responses into `model.ReviewComment` items
   - `log/`: Configures Logrus with file rotation to `./log_files/` on Windows and `../../log_files/` elsewhere

4. **Logging**: The application provides detailed logs showing:
   - Number of PRs fetched
   - Each PR being processed (ID, title, author)
   - Author ignore list checking
   - Whether PRs are skipped or reviewed
   - AI comment generation and posting results

### Environment
The app sets some defaults on startup:
- `APP_NAME=code-nim`
- `LOG_LEVEL`: If not running in Kubernetes (no `KUBERNETES_SERVICE_HOST`), defaults to `DEBUG`; otherwise read from env.
- `TZ=Asia/Ho_Chi_Minh`

You can override `LOG_LEVEL` with one of: `DEBUG`, `INFO`, `WARN`, `ERROR`, `OFF`.

### Logging
Rotating logs are written under `log_files/`:
- `log_files/info/code-nim_YYYYMMDD_info.log`
- `log_files/error/code-nim_YYYYMMDD_error.log`

**Log Format Examples:**
```
INFO: Fetched 3 pull requests for review
INFO: Processing PR #1909: 'SREP-1158/template singapore region' by Phong Le
DEBUG: Checking if PR author 'Phong Le' matches ignore list entry 'Senior Dev'
INFO: Will ignore PR #1909 by Phong Le (matches ignore list)
INFO: Skipping PR #1909 (author is in ignore list)
INFO: Starting review process for PR #1898 by Minh Ha Phan Bao
```

**Log Levels:**
- `DEBUG`: Detailed author checking, API response sizes, diff parsing details
- `INFO`: PR processing status, ignore decisions, review actions
- `ERROR`: API failures, parsing errors, comment posting failures

### Best Practices & Usage Tips

**Author Ignore Lists:**
- Add senior developers or team leads to `ignorePullRequestOf.displayNames` to avoid reviewing their PRs
- Use exact display names as they appear in Bitbucket (case-sensitive)
- Consider ignoring automated PRs from bots or dependency update tools

**Scheduling:**
- Start with longer intervals (e.g., `*/10 * * * *`) to avoid rate limiting
- Monitor API usage and adjust frequency based on your team's PR volume
- Use different schedules for different repositories based on their activity

**Review Quality:**
- The AI works best on focused, single-purpose PRs
- Large PRs with many files may generate too many comments
- Consider repository-specific prompts for different codebases

**Monitoring:**
- Check logs regularly for API failures or rate limiting
- Monitor comment quality and adjust prompts if needed
- Use `DEBUG` level logging initially to understand the flow

### Troubleshooting

**Common Issues:**
1. **No comments posted**: Check that the Bitbucket App Password has comment permissions
2. **Rate limiting**: Reduce cron frequency or check Bitbucket API limits
3. **Gemini API errors**: Verify API key and check Google Cloud quotas
4. **PRs not ignored**: Ensure display names in ignore list match exactly (case-sensitive)

**Debug Steps:**
1. Set `LOG_LEVEL=DEBUG` to see detailed processing
2. Check `log_files/error/` for API failures
3. Verify configuration with a simple cron like `*/5 * * * *`
4. Test with a small repository first

### Security and secrets
- Do NOT commit real credentials in `config_file/review-config.yaml`. Store secrets in environment variables or a secrets manager, then template the YAML at deploy time.
- Use a least-privilege Bitbucket App Password.
- Restrict outbound network egress if possible to Bitbucket and Google APIs.

### Running in Docker (optional)
Create a minimal Dockerfile if desired. Example:
```Dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /app
COPY . .
RUN go build -o /code-nim

FROM alpine:3.20
WORKDIR /
COPY --from=build /code-nim /code-nim
COPY config_file /config_file
ENV LOG_LEVEL=INFO TZ=Asia/Ho_Chi_Minh
EXPOSE 1994
ENTRYPOINT ["/code-nim"]
```
And Command

```
docker run -d --name code-nim -e LOG_LEVEL=INFO -e TZ=Asia/Ho_Chi_Minh -v /home/docker/code_nim/review-config.yaml:/go/src/code_nim/config_file/review-config.yaml --restart unless-stopped mrnim94/code_nim:latest
```
### License
Apache License 2.0. See `LICENSE`.


