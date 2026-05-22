# webfetch

A Cloudflare Browser Rendering API client for Go. It fetches dynamic web pages as fully rendered HTML, converts them to clean Markdown, captures viewport screenshots, scrapes DOM elements using CSS selectors, extracts anchor links, or takes structured snapshots.

---

## ⚠️ Critical Requirements

To use this library, you **must** provide the following environment variables:

```bash
export CLOUDFLARE_ACCOUNT_ID="your-cloudflare-account-id"
export CLOUDFLARE_API_TOKEN="your-cloudflare-api-token"
```

*   **Cloudflare Account ID**: Found on your Cloudflare Dashboard URL or Overview page.
*   **Cloudflare API Token**: Needs the `Browser Rendering - Edit` permission (or `Account.Browser Rendering` permission).

These variables are automatically loaded by the client on initialization.

---

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [API Methods](#api-methods)
- [Configuration Types](#configuration-types)
  - [PageOptions](#pageoptions)
  - [ScreenshotOptions](#screenshotoptions)
  - [ScrapeSelector](#scrapeselector)
- [Response Types](#response-types)
- [Examples](#examples)

---

## Installation

Initialize your Go module and fetch the package:

```bash
go get github.com/proiceremo/webfetch
```

---

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/proiceremo/webfetch"
)

func main() {
	ctx := context.Background()

	// 1. Initialize the client (automatically reads env vars)
	client, err := webfetch.NewClient()
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// 2. Fetch rendered markdown
	resp, err := client.FetchMarkdown(ctx, webfetch.PageOptions{
		URL: "https://example.com",
	})
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Title: %s\nStatus: %d\n\nMarkdown:\n%s\n", resp.Title, resp.Status, resp.Content)
}
```

---

## API Methods

All methods belong to the `*webfetch.Client` struct and accept a `context.Context` and a `webfetch.PageOptions`.

### 1. `FetchContent`
Fetches the fully rendered HTML of a page after running client-side JavaScript.
```go
resp, err := client.FetchContent(ctx, webfetch.PageOptions{URL: "https://example.com"})
// Returns (*webfetch.ContentResponse, error)
// Access HTML with resp.Content
```

### 2. `FetchMarkdown`
Fetches a page, renders it, and returns the contents formatted as clean Markdown. Excellent for feeding web pages to LLMs.
```go
resp, err := client.FetchMarkdown(ctx, webfetch.PageOptions{URL: "https://example.com"})
// Returns (*webfetch.MarkdownResponse, error)
// Access Markdown with resp.Content
```

### 3. `TakeScreenshot`
Captures a screenshot of the page.
```go
resp, err := client.TakeScreenshot(ctx, 
	webfetch.PageOptions{
		URL: "https://example.com",
		Viewport: &webfetch.ViewportOptions{Width: 1280, Height: 720},
	},
	&webfetch.ScreenshotOptions{
		Type: "png", // "png" or "jpeg"
	},
)
// Returns (*webfetch.ScreenshotResponse, error)
// Access raw image bytes with resp.Data and MIME type with resp.ContentType
```

### 4. `Scrape`
Scrapes targeted elements from the page using CSS selectors.
```go
selectors := []webfetch.ScrapeSelector{
	{Selector: "h1", Type: "text"},
	{Selector: ".price-tag", Type: "attribute", Attribute: "data-price"},
}
resp, err := client.Scrape(ctx, webfetch.PageOptions{URL: "https://example.com"}, selectors)
// Returns (*webfetch.ScrapeResponse, error)
```

### 5. `FetchLinks`
Extracts all anchor (`<a href="...">`) links from the page.
```go
resp, err := client.FetchLinks(ctx, webfetch.PageOptions{URL: "https://example.com"})
// Returns (*webfetch.LinksResponse, error)
// Access list of URLs with resp.Links (slice of strings)
```

### 6. `TakeSnapshot`
A combined call returning both the fully rendered HTML and a base64-encoded PNG screenshot of the page.
```go
resp, err := client.TakeSnapshot(ctx, webfetch.PageOptions{URL: "https://example.com"})
// Returns (*webfetch.SnapshotResponse, error)
// HTML: resp.Content
// Base64 PNG Screenshot: resp.Screenshot
```

---

## Configuration Types

### PageOptions
Controls navigation, viewport size, and behavior on page load.

```go
type PageOptions struct {
	URL                  string            `json:"url,omitempty"`
	HTML                 string            `json:"html,omitempty"`
	ActionTimeout        int               `json:"actionTimeout,omitempty"`
	AddScriptTag         []AddScriptTag    `json:"addScriptTag,omitempty"`
	AddStyleTag          []AddStyleTag     `json:"addStyleTag,omitempty"`
	AllowRequestPattern  []string          `json:"allowRequestPattern,omitempty"`
	AllowResourceTypes   []string          `json:"allowResourceTypes,omitempty"`
	Authenticate         *AuthCredentials  `json:"authenticate,omitempty"`
	BestAttempt          bool              `json:"bestAttempt,omitempty"`
	BlockRequestPattern  []string          `json:"blockRequestPattern,omitempty"`
	Cookies              []Cookie          `json:"cookies,omitempty"`
	EmulateMediaType     string            `json:"emulateMediaType,omitempty"`
	GotoOptions          *GotoOptions      `json:"gotoOptions,omitempty"`
	RejectResourceTypes  []string          `json:"rejectResourceTypes,omitempty"`
	SetExtraHTTPHeaders  map[string]string `json:"setExtraHTTPHeaders,omitempty"`
	SetJavaScriptEnabled *bool             `json:"setJavaScriptEnabled,omitempty"`
	UserAgent            string            `json:"userAgent,omitempty"`
	Viewport             *ViewportOptions  `json:"viewport,omitempty"`
	WaitForSelector      *WaitForSelector  `json:"waitForSelector,omitempty"`
	WaitForTimeout       int               `json:"waitForTimeout,omitempty"`
}
```

*   **Sensible Defaults**: When calling `FetchContent`, `FetchMarkdown`, `TakeScreenshot`, `Scrape`, `FetchLinks`, or `TakeSnapshot`, if `GotoOptions` is nil or incomplete, safe defaults (e.g. `WaitUntil: "networkidle2"`, `Timeout: 45000ms`, `BestAttempt: true`) are automatically applied. This prevents returning blank/partial content on heavy SPA websites.

### ScreenshotOptions
Controls image format, quality, and screenshot area.

```go
type ScreenshotOptions struct {
	Type           string    `json:"type,omitempty"`    // "png" or "jpeg"
	Quality        int       `json:"quality,omitempty"` // 0-100, jpeg only
	FullPage       bool      `json:"fullPage,omitempty"`
	Clip           *ClipRect `json:"clip,omitempty"`
	OmitBackground bool      `json:"omitBackground,omitempty"`
}
```

### ScrapeSelector
Defines CSS selectors for the `Scrape` API.

```go
type ScrapeSelector struct {
	Selector  string `json:"selector"`
	Type      string `json:"type,omitempty"`      // "text", "html", or "attribute"
	Attribute string `json:"attribute,omitempty"` // required when Type is "attribute"
}
```

---

## Response Types

### ContentResponse / MarkdownResponse
```go
type ContentResponse struct {
	Content string `json:"content"`
	Title   string `json:"title,omitempty"`
	Status  int    `json:"status,omitempty"`
}
```

### ScreenshotResponse
```go
type ScreenshotResponse struct {
	Data        []byte // Raw screenshot bytes
	ContentType string // Content type header (e.g. "image/png")
}
```

### ScrapeResponse
```go
type ScrapeResponse struct {
	Result []ScrapeItem `json:"result"`
}

type ScrapeItem struct {
	Selector string          `json:"selector"`
	Results  []ScrapeElement `json:"results"`
}

type ScrapeElement struct {
	Text       string             `json:"text,omitempty"`
	HTML       string             `json:"html,omitempty"`
	Attributes []ScrapeAttribute  `json:"attributes,omitempty"`
	Height     float64            `json:"height,omitempty"`
	Width      float64            `json:"width,omitempty"`
	Top        float64            `json:"top,omitempty"`
	Left       float64            `json:"left,omitempty"`
}

type ScrapeAttribute struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// AttributeMap collapses Attributes into a map[string]string for easy lookup.
func (e ScrapeElement) AttributeMap() map[string]string
```

### LinksResponse
```go
type LinksResponse struct {
	Links []string `json:"links"` // List of anchor URLs
}
```

### SnapshotResponse
```go
type SnapshotResponse struct {
	Content    string `json:"content"`              // HTML source
	Screenshot string `json:"screenshot"`           // Base64-encoded PNG screenshot
	Title      string `json:"title,omitempty"`
	Status     int    `json:"status,omitempty"`
}
```

---

## Examples

### 1. Wait for selector before capturing a screenshot
```go
resp, err := client.TakeScreenshot(ctx,
	webfetch.PageOptions{
		URL: "https://news.ycombinator.com",
		WaitForSelector: &webfetch.WaitForSelector{
			Selector: ".hnname",
			Timeout:  10000, // 10 seconds
		},
		Viewport: &webfetch.ViewportOptions{Width: 1024, Height: 768},
	},
	&webfetch.ScreenshotOptions{
		Type: "png",
	},
)
if err == nil {
	err = os.WriteFile("hn.png", resp.Data, 0644)
}
```

### 2. Scraping products and extracting attributes
```go
selectors := []webfetch.ScrapeSelector{
	{Selector: "a.product-card", Type: "attribute", Attribute: "href"},
	{Selector: ".product-title", Type: "text"},
}

resp, err := client.Scrape(ctx, webfetch.PageOptions{URL: "https://example.com/shop"}, selectors)
if err != nil {
	log.Fatal(err)
}

for _, item := range resp.Result {
	fmt.Printf("Selector: %s matches found: %d\n", item.Selector, len(item.Results))
	for _, el := range item.Results {
		if item.Selector == "a.product-card" {
			fmt.Printf(" - URL: %s\n", el.AttributeMap()["href"])
		} else {
			fmt.Printf(" - Text: %s\n", el.Text)
		}
	}
}
```
