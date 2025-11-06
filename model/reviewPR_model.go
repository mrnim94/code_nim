package model

type Task struct {
	AutoReviewPRs []AutoReviewPR `yaml:"autoReviewPR"`
}

type AutoReviewPR struct {
	ProcessName  string   `yaml:"processName"`
	Cron         string   `yaml:"cron"`
	GitProvider  string   `yaml:"gitProvider"`
	Workspace    string   `yaml:"workspace"`
	RepoSlug     string   `yaml:"repoSlug"`
	DisplayNames []string `yaml:"displayNames"`
	Username     string   `yaml:"username"`
	AppPassword  string   `yaml:"appPassword"`
	GeminiKey    string   `yaml:"geminiKey"`
	GeminiModel  string   `yaml:"geminiModel,omitempty"`
	// Generic AI configuration (optional). If aiProvider=="self", these are used.
	AIProvider          string `yaml:"aiProvider,omitempty"`     // "gemini" (default) or "self"
	AIModel             string `yaml:"aiModel,omitempty"`        // Preferred model name; falls back to GeminiModel
	AIKey               string `yaml:"aiKey,omitempty"`          // Generic API key; falls back to GeminiKey
	SelfAPIBaseURL      string `yaml:"selfApiBaseUrl,omitempty"` // e.g., http://192.168.101.27:1994
	IgnorePullRequestOf struct {
		DisplayNames []string `yaml:"displayNames"`
	} `yaml:"ignorePullRequestOf"`
}
