package webfetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client is a Cloudflare Browser Rendering API client.
type Client struct {
	accountID  string
	apiToken   string
	baseURL    string
	httpClient *http.Client
}

// ClientOption configures the Client.
type ClientOption func(*Client)

// WithBaseURL overrides the default Cloudflare API base URL.
func WithBaseURL(url string) ClientOption {
	return func(c *Client) { c.baseURL = url }
}

// WithHTTPClient uses a custom HTTP client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) { c.httpClient = hc }
}

// NewClient creates a new Cloudflare Browser Rendering client.
// It reads CLOUDFLARE_ACCOUNT_ID and CLOUDFLARE_API_TOKEN from environment variables.
func NewClient(opts ...ClientOption) (*Client, error) {
	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	apiToken := os.Getenv("CLOUDFLARE_API_TOKEN")

	if accountID == "" {
		return nil, fmt.Errorf("CLOUDFLARE_ACCOUNT_ID environment variable is required")
	}
	if apiToken == "" {
		return nil, fmt.Errorf("CLOUDFLARE_API_TOKEN environment variable is required")
	}

	c := &Client{
		accountID: accountID,
		apiToken:  apiToken,
		baseURL:   "https://api.cloudflare.com/client/v4",
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// IsConfigured returns true if the required environment variables are set.
func IsConfigured() bool {
	return os.Getenv("CLOUDFLARE_ACCOUNT_ID") != "" && os.Getenv("CLOUDFLARE_API_TOKEN") != ""
}

func (c *Client) endpoint(path string) string {
	return fmt.Sprintf("%s/accounts/%s/browser-rendering%s", c.baseURL, c.accountID, path)
}

func (c *Client) doJSON(ctx context.Context, path string, body any, result any) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(path), bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, path, formatErrorBody(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return nil
}

// formatErrorBody tries to extract a human-readable error message from a
// Cloudflare error response (which always carries an envelope). Falls back
// to the raw body when the response isn't recognisable JSON.
func formatErrorBody(body []byte) string {
	var env struct {
		Errors []APIError `json:"errors"`
	}
	if json.Unmarshal(body, &env) == nil && len(env.Errors) > 0 {
		parts := make([]string, 0, len(env.Errors))
		for _, e := range env.Errors {
			parts = append(parts, fmt.Sprintf("[%d] %s", e.Code, e.Message))
		}
		return joinErrors(parts)
	}
	return string(body)
}

func joinErrors(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "; "
		}
		out += p
	}
	return out
}

func (c *Client) doRaw(ctx context.Context, path string, body any) ([]byte, string, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(path), bytes.NewReader(jsonBody))
	if err != nil {
		return nil, "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, path, string(data))
	}

	return data, resp.Header.Get("Content-Type"), nil
}

// FetchContent fetches rendered HTML content from a URL.
// POST /browser-rendering/content
//
// Applies withSensibleDefaults so JavaScript-heavy pages (the vast majority
// of modern URLs) wait for content to actually render before returning.
// Callers that already specified GotoOptions keep their settings.
func (c *Client) FetchContent(ctx context.Context, pageOpts PageOptions) (*ContentResponse, error) {
	var result ContentResponse
	if err := c.doJSON(ctx, "/content", pageOpts.withSensibleDefaults(), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FetchMarkdown fetches a page and returns its content as markdown.
// POST /browser-rendering/markdown
func (c *Client) FetchMarkdown(ctx context.Context, pageOpts PageOptions) (*MarkdownResponse, error) {
	var result MarkdownResponse
	if err := c.doJSON(ctx, "/markdown", pageOpts.withSensibleDefaults(), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// TakeScreenshot takes a screenshot of a page.
// POST /browser-rendering/screenshot
func (c *Client) TakeScreenshot(ctx context.Context, pageOpts PageOptions, ssOpts *ScreenshotOptions) (*ScreenshotResponse, error) {
	body := struct {
		PageOptions
		*ScreenshotOptions `json:"screenshotOptions,omitempty"`
	}{
		PageOptions:       pageOpts.withSensibleDefaults(),
		ScreenshotOptions: ssOpts,
	}
	data, contentType, err := c.doRaw(ctx, "/screenshot", body)
	if err != nil {
		return nil, err
	}
	return &ScreenshotResponse{Data: data, ContentType: contentType}, nil
}

// Scrape scrapes elements from a page using CSS selectors.
// POST /browser-rendering/scrape
func (c *Client) Scrape(ctx context.Context, pageOpts PageOptions, selectors []ScrapeSelector) (*ScrapeResponse, error) {
	body := struct {
		PageOptions
		Elements []ScrapeSelector `json:"elements"`
	}{
		PageOptions: pageOpts.withSensibleDefaults(),
		Elements:    selectors,
	}
	var result ScrapeResponse
	if err := c.doJSON(ctx, "/scrape", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FetchLinks extracts all links from a page.
// POST /browser-rendering/links
func (c *Client) FetchLinks(ctx context.Context, pageOpts PageOptions) (*LinksResponse, error) {
	var result LinksResponse
	if err := c.doJSON(ctx, "/links", pageOpts.withSensibleDefaults(), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// TakeSnapshot returns both HTML content and a base64 screenshot.
// POST /browser-rendering/snapshot
func (c *Client) TakeSnapshot(ctx context.Context, pageOpts PageOptions) (*SnapshotResponse, error) {
	var result SnapshotResponse
	if err := c.doJSON(ctx, "/snapshot", pageOpts.withSensibleDefaults(), &result); err != nil {
		return nil, err
	}
	return &result, nil
}
