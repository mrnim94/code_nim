package bitbucket_impl

import (
	"code_nim/helper/atlassian"
	"net/http"
)

type HttpClient struct {
	http *http.Client
}

// New returns a production client.
// You can swap it for a mock in tests.
func New(httpClient *http.Client) atlassian.Bitbucket {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &HttpClient{http: httpClient}
}
