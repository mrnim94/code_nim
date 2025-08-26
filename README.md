# code-nim

An automated Pull Request reviewer for Bitbucket using Google's Gemini. It periodically scans configured repositories, fetches open PRs, generates structured AI review comments from diffs, and posts them back to Bitbucket. Built with Go, Echo, Logrus, and gocron.

### Features
- **Bitbucket PR scanning**: Lists open PRs for configured repos.
- **Diff parsing**: Parses PR diffs per file and hunk.
- **AI reviews with Gemini**: Crafts a review prompt and parses JSON responses into actionable comments.
- **Idempotent commenting**: Skips PRs where configured users have already commented.
- **Scheduling**: Runs via cron specs using gocron.
- **Structured logging**: Rotating file logs for info and error levels.

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
```

- **processName**: Identifier for the scheduled job.
- **cron**: Cron expression in UTC for how often to scan and review.
- **workspace**: Bitbucket workspace.
- **repoSlug**: Repository slug.
- **displayNames**: Display names that count as "already reviewed" and prevent duplicate AI comments.
- **username/appPassword**: Used for Bitbucket Basic Auth. App Password must allow reading PRs and posting comments.
- **geminiKey**: API key for Gemini models.

Notes:
- The service checks recent comments on a PR; if the configured `username` or any name in `displayNames` has a non-empty comment, it will skip posting AI comments.
- By default it uses model `gemini-2.5-flash` in `helper.GetAIResponseOfGemini`.

### How it works
- `main.go` initializes logging, sets timezone to `Asia/Ho_Chi_Minh`, builds the Bitbucket client, registers the scheduler via `handler.AutoReviewPRHandler`, and starts Echo on `:1994`.
- `handler/autoReviewPR_handler.go` loads the YAML config, sets up cron jobs, fetches PRs, parses diffs, generates AI review comments, filters out command-like text, and posts comments.
- `helper/atlassian/bitbucket_impl` contains Bitbucket HTTP client methods for PRs, diffs, comments, and a minimal diff parser.
- `helper/promt_help.go` builds the Gemini prompt and parses the model response into `model.ReviewComment` items.
- `log/` configures Logrus with file rotation to `./log_files/` on Windows and `../../log_files/` elsewhere.

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

### License
Apache License 2.0. See `LICENSE`.
