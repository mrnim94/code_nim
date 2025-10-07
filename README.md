# code-nim

An enterprise-ready automated Pull Request reviewer for Bitbucket using Google's Gemini AI. It periodically scans configured repositories, fetches open PRs, generates structured AI review comments from diffs, and posts them back to Bitbucket with advanced concurrency protection and duplicate prevention. Built with Go, Echo, Logrus, and gocron.

## 🚀 Features

### Core Functionality
- **🔍 Bitbucket PR scanning**: Lists open PRs for configured repositories with pagination support
- **📝 Advanced diff parsing**: Parses PR diffs per file and hunk with accurate line mapping
- **🤖 AI reviews with Gemini**: Crafts structured review prompts and parses JSON responses into actionable comments
- **🔒 Intelligent duplicate prevention**: Skips PRs where configured users have already commented (inline or general)
- **👥 Author filtering**: Ignore PRs from specific authors (useful for skipping senior developers or specific team members)
- **📌 Precise inline comments**: Posts AI-generated comments directly on specific lines in the code diff with anchor-based positioning

### Advanced Features
- **⚡ Concurrency protection**: Mutex locking and gocron SingletonMode prevent race conditions and duplicate reviews
- **🛡️ Rate limiting protection**: Built-in delays between API calls to prevent rate limit errors
- **🎯 Smart comment formatting**: Automatic formatting of review sections (Why, How, Suggested changes, Notes)
- **🚫 Command filtering**: Automatically filters out AI responses that look like shell commands or code execution
- **📊 Performance monitoring**: Execution time tracking and detailed performance logging
- **🔧 Configurable AI models**: Support for different Gemini models per repository (gemini-1.5-flash, gemini-2.5-flash, etc.)
- **🗂️ Multi-repository support**: Configure multiple repositories with different settings in a single instance

### Error Handling & Reliability
- **🚨 Comprehensive error handling**: Detailed Gemini API error handling with specific error codes and guidance
- **🔄 Graceful degradation**: Continues processing even if individual files fail to generate comments
- **📋 Robust JSON parsing**: Better handling of malformed or incomplete AI responses
- **📈 Detailed logging**: Rotating file logs with structured information for debugging and monitoring
- **⏱️ Timeout handling**: Proper handling of long-running AI API calls

## 📋 Requirements

- Go 1.23+
- Bitbucket account with an App Password that can read PRs and post comments
- Google Generative Language API key (Gemini)

## 🚀 Quick Start

1. **Clone and setup**:
```bash
git clone <this-repo>
cd code_nim
go mod download
```

2. **Configure your jobs** in `config_file/review-config.yaml` (see Configuration below)

3. **Build and run**:
```bash
go run .
# or
go build -o code-nim
./code-nim
```

The service starts an Echo server on port `1994` and launches schedulers in the background with concurrency protection.

## ⚙️ Configuration

Edit `config_file/review-config.yaml` to define one or more review tasks under `autoReviewPR`:

```yaml
autoReviewPR:
  - processName: main-repo-reviews
    cron: "*/10 * * * *" # Every 10 minutes
    gitProvider: bitbucket
    workspace: <your-workspace>
    repoSlug: <your-repo>
    displayNames:
      - <Your Name>
      - <Bot Display Name>
    username: <bitbucket-username>
    appPassword: <bitbucket-app-password>
    geminiKey: <google-generative-language-api-key>
    geminiModel: gemini-1.5-flash  # Optional: specify model per repo
    ignorePullRequestOf:
      displayNames:
        - Senior Developer Name
        - Lead Architect Name
        
  - processName: second-repo-reviews
    cron: "*/15 * * * *" # Every 15 minutes
    gitProvider: bitbucket
    workspace: <your-workspace>
    repoSlug: <your-second-repo>
    displayNames:
      - <Your Name>
    username: <bitbucket-username>
    appPassword: <bitbucket-app-password>
    geminiKey: <google-generative-language-api-key>
    geminiModel: gemini-2.5-flash  # Different model for this repo
    ignorePullRequestOf:
      displayNames:
        - Bot User
```

### Configuration Fields

| Field | Description | Required |
|-------|-------------|----------|
| `processName` | Identifier for the scheduled job | ✅ |
| `cron` | Cron expression in UTC for how often to scan and review | ✅ |
| `workspace` | Bitbucket workspace | ✅ |
| `repoSlug` | Repository slug | ✅ |
| `displayNames` | Display names that count as "already reviewed" | ✅ |
| `username/appPassword` | Bitbucket Basic Auth credentials | ✅ |
| `geminiKey` | API key for Gemini models | ✅ |
| `geminiModel` | Specific Gemini model (defaults to `gemini-2.5-flash`) | ❌ |
| `ignorePullRequestOf.displayNames` | Authors whose PRs should be ignored | ❌ |

### Available Gemini Models
- `gemini-1.5-flash` - Fast, cost-effective
- `gemini-1.5-pro` - Higher quality, slower
- `gemini-2.5-flash` - Latest fast model (default)

**⚠️ SECURITY WARNING**: Never commit real credentials to version control! Use environment variables or a secrets management system in production.

## 🔄 How It Works

### 1. Initialization
`main.go` initializes:
- Logging with rotation to `log_files/`
- Timezone set to `Asia/Ho_Chi_Minh`  
- Bitbucket HTTP client with proper authentication
- Scheduler with concurrency protection
- Echo server on port `:1994`

### 2. Scheduled Review Process
`handler/autoReviewPR_handler.go` orchestrates the review workflow:

#### **Concurrency Protection**
- 🔒 **Mutex locking**: Prevents multiple review processes from running simultaneously
- ⚡ **gocron SingletonMode**: Additional scheduler-level protection against overlapping jobs
- ⏱️ **Execution timing**: Monitors and logs how long each review cycle takes

#### **PR Processing Pipeline**
1. **Fetch PRs**: Retrieves all open pull requests from configured repository
2. **Author filtering**: Checks if PR author is in ignore list and skips if found
3. **Duplicate detection**: Scans existing comments to see if user already reviewed (inline or general comments)
4. **Rate limiting**: Adds delays between PR processing and API calls
5. **Diff analysis**: Fetches and parses PR diffs with accurate line mapping
6. **AI review generation**: Calls Gemini with structured prompts per file
7. **Comment filtering**: Removes command-like responses and empty comments  
8. **Inline posting**: Posts formatted comments to specific diff lines
9. **Error recovery**: Gracefully handles failures and continues processing

#### **AI Review Generation**
- **Structured prompts**: Creates detailed prompts with PR context, file paths, and diffs
- **CodeRabbit-style reviews**: Generates comments with severity levels, categories, and structured feedback
- **Multiple formats**: Supports various code languages and file types
- **Context-aware**: Considers PR title, description, and overall change patterns

### 3. Architecture Components

#### **Core Modules**
- `handler/autoReviewPR_handler.go`: Main orchestration and concurrency control
- `helper/atlassian/bitbucket_impl/`: Bitbucket API client with comprehensive error handling
- `helper/promt_help.go`: AI prompt engineering and response parsing
- `model/`: Data structures for PRs, comments, and AI responses
- `log/`: Structured logging with file rotation

#### **Error Handling**
- **Gemini API errors**: Handles rate limits (429), auth failures (401), billing issues (403)
- **Bitbucket API errors**: Manages authentication and API limit issues  
- **JSON parsing**: Robust handling of malformed AI responses
- **Network failures**: Retry logic and graceful degradation

## 📊 Logging & Monitoring

### Log Levels & Locations
Rotating logs are written under `log_files/`:
- `log_files/info/code-nim_YYYYMMDD_info.log` - General operations
- `log_files/error/code-nim_YYYYMMDD_error.log` - Errors and failures

### Environment Variables
The app sets these defaults on startup:
- `APP_NAME=code-nim`
- `LOG_LEVEL`: `DEBUG` (if not in Kubernetes), otherwise read from env
- `TZ=Asia/Ho_Chi_Minh`

Override `LOG_LEVEL` with: `DEBUG`, `INFO`, `WARN`, `ERROR`, `OFF`

### Log Examples

**Successful Processing:**
```
INFO: Review PR Handler for workspace/repo (acquired lock)
INFO: Fetched 3 pull requests for review  
INFO: Processing PR #1953: 'Add oesis engine configuration' by Developer Name
DEBUG: Found existing inline comment at app-engines.yaml:126
INFO: Skipping PR #1953 - user has already reviewed (commented)
INFO: Review PR Handler completed for workspace/repo in 2.5s
```

**Error Handling:**
```
ERROR: Gemini API rate limit exceeded: Quota exceeded for requests per minute
ERROR: Please check your API quota and billing details
INFO: Skipping remaining files for PR #1945 due to rate limit
```

**Concurrency Protection:**
```
INFO: Skipping review execution - another review process is already running for workspace/repo
```

## 🛠️ Best Practices & Usage Tips

### **Scheduling Strategy**
- **Start conservative**: Begin with longer intervals (`*/15 * * * *`) to avoid rate limits
- **Monitor usage**: Check both Bitbucket and Gemini API usage patterns
- **Repository-specific timing**: High-activity repos may need longer intervals
- **Avoid peak hours**: Schedule during low-activity periods for better performance

### **Author Management**
- **Ignore senior developers**: Add team leads to `ignorePullRequestOf` to avoid reviewing their PRs
- **Case sensitivity**: Use exact display names as they appear in Bitbucket
- **Bot exclusion**: Ignore automated PRs from dependency update tools or CI/CD systems
- **Dynamic lists**: Consider different ignore lists for different repositories

### **Review Quality Optimization**
- **Focused PRs work best**: AI provides better reviews on single-purpose, smaller PRs
- **Large PR handling**: The system detects overly large PRs and suggests breaking them down
- **Model selection**: Use `gemini-1.5-flash` for speed, `gemini-2.5-flash` for balanced performance
- **Repository-specific models**: Configure different models based on codebase complexity

### **Performance Tuning**
- **Rate limit prevention**: Built-in delays prevent API throttling
- **Concurrent execution**: Mutex protection ensures no resource conflicts
- **Memory management**: Processes PRs sequentially to control memory usage
- **Log management**: Rotate logs regularly to prevent disk space issues

## 🐛 Troubleshooting

### **Common Issues**

| Issue | Symptoms | Solution |
|-------|----------|----------|
| **No comments posted** | Reviews run but no Bitbucket comments appear | Check App Password has comment permissions |
| **Rate limiting** | `429` errors in logs | Increase cron intervals, check API quotas |
| **Authentication failures** | `401/403` errors | Verify Bitbucket credentials and Gemini API key |
| **PRs not ignored** | Reviews posted on ignored authors | Ensure display names match exactly (case-sensitive) |
| **Duplicate comments** | Same comment posted multiple times | Check for concurrency issues, restart service |
| **Large diffs ignored** | No comments on big PRs | Expected behavior - AI skips overly large changes |

### **Debug Steps**

1. **Enable debug logging**:
   ```bash
   export LOG_LEVEL=DEBUG
   ./code-nim
   ```

2. **Check error logs**:
   ```bash
   tail -f log_files/error/code-nim_$(date +%Y%m%d)_error.log
   ```

3. **Verify configuration**:
   ```bash
   # Test with simple cron schedule
   cron: "*/5 * * * *"
   ```

4. **Monitor API usage**:
   - Check Bitbucket API rate limits
   - Verify Gemini API quotas in Google Cloud Console

### **Performance Issues**
- **Slow AI responses**: Consider using `gemini-1.5-flash` instead of `gemini-1.5-pro`
- **Memory usage**: Restart service periodically for long-running instances
- **Disk space**: Monitor `log_files/` directory size and rotate logs

## 🔐 Security & Deployment

### **Credentials Management**
- **Environment variables**: Store secrets in env vars, not config files
- **Secrets management**: Use HashiCorp Vault, AWS Secrets Manager, or Kubernetes secrets
- **Least privilege**: Create Bitbucket App Passwords with minimal required permissions
- **API key rotation**: Regularly rotate Gemini API keys and Bitbucket credentials

### **Network Security**
- **Egress filtering**: Restrict outbound access to Bitbucket and Google APIs only
- **TLS verification**: Ensure all API communications use HTTPS
- **Firewall rules**: Limit inbound access to Echo server port (1994)

### **Docker Deployment**

```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /app
COPY . .
RUN go build -o /code-nim

FROM alpine:3.20
WORKDIR /
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /code-nim /code-nim
COPY config_file /config_file
ENV LOG_LEVEL=INFO TZ=Asia/Ho_Chi_Minh
EXPOSE 1994
ENTRYPOINT ["/code-nim"]
```

**Run command:**
```bash
docker run -d --name code-nim \
  -e LOG_LEVEL=INFO \
  -e TZ=Asia/Ho_Chi_Minh \
  -v /host/path/review-config.yaml:/config_file/review-config.yaml \
  --restart unless-stopped \
  mrnim94/code_nim:latest
```

### **Production Considerations**
- **Health checks**: Monitor the Echo server endpoint for service health
- **Log aggregation**: Use ELK stack or similar for centralized log management  
- **Metrics**: Consider adding Prometheus metrics for monitoring
- **Backup**: Regular backup of configuration files and logs
- **Scaling**: Run multiple instances with different repository configurations

## 📄 License

Apache License 2.0. See `LICENSE`.

---

**Built with ❤️ for better code reviews**