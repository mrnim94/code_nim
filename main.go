package main

import (
	"code_nim/handler"
	"code_nim/helper/atlassian/bitbucket_impl"
	"code_nim/log"
	"github.com/labstack/echo/v4"
	"os"
)

func init() {
	os.Setenv("APP_NAME", "code-nim")
	logger := log.InitLogger(false)
	// Check if KUBERNETES_SERVICE_HOST is set
	if _, exists := os.LookupEnv("KUBERNETES_SERVICE_HOST"); !exists {
		// If not in Kubernetes, set LOG_LEVEL to DEBUG
		os.Setenv("LOG_LEVEL", "DEBUG")
	}
	logger.SetLevel(log.GetLogLevel("LOG_LEVEL"))
	os.Setenv("TZ", "Asia/Ho_Chi_Minh")
}

func main() {
	bitbucket := bitbucket_impl.New(nil)

	autoReviewPRHandler := handler.AutoReviewPRHandler{
		Bitbucket: bitbucket,
	}

	e := echo.New()
	autoReviewPRHandler.HandlerAutoReviewPR()
	e.Logger.Fatal(e.Start(":1994"))
}
