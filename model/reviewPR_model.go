package model

type Task struct {
	AutoReviewPRs []AutoReviewPR `yaml:"autoReviewPR"`
}

type AutoReviewPR struct {
	ProcessName         string   `yaml:"processName"`
	Cron                string   `yaml:"cron"`
	GitProvider         string   `yaml:"gitProvider"`
	Workspace           string   `yaml:"workspace"`
	RepoSlug            string   `yaml:"repoSlug"`
	DisplayNames        []string `yaml:"displayNames"`
	Username            string   `yaml:"username"`
	AppPassword         string   `yaml:"appPassword"`
	GeminiKey           string   `yaml:"geminiKey"`
	IgnorePullRequestOf struct {
		DisplayNames []string `yaml:"displayNames"`
	} `yaml:"ignorePullRequestOf"`
}
