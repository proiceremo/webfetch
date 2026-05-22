# webfetch

Cloudflare Browser Rendering API client for Go. Fetches web pages as rendered HTML, markdown, screenshots, or structured snapshots, and supports CSS-selector scraping.

---

## Table of Contents

- [Requirements](#requirements)
- [Installation](#installation)
- [Usage](#usage)
- [API Methods](#api-methods)
- [Page Options](#page-options)
- [Screenshot Options](#screenshot-options)
- [Scrape Selectors](#scrape-selectors)
- [Error Handling](#error-handling)
- [Examples](#examples)

---

## Requirements

- `CLOUDFLARE_ACCOUNT_ID`
- `CLOUDFLARE_API_TOKEN`

These are read from environment variables automatically.

---

## Installation

```bash
go get github.com/proagent/webfetch
```

---

## Usage

### Create Client

```go
import "github.com/proagent/webfetch"

client, err := webfetch.NewClient()
if err != nil {
    log.Fatal(err)
}
```

### Fetch as Markdown

```go
md, err := client.FetchMarkdown(ctx, webfetch.PageOptions{
    URL: "https://example.com",
})
fmt.Println(md)
```

### Fetch Rendered HTML

```go
html, err := client.FetchContent(ctx, webfetch.PageOptions{
    URL: "https://example.com",
})
```

### Take a Screenshot

```go
ss, err := client.TakeScreenshot(ctx,
    webfetch.PageOptions{URL: "https://example.com"},
    &webfetch.ScreenshotOptions{
        Width:  1280,
        Height: 720,
        Format: webfetch.ScreenshotFormatPNG,
    },
)
```

### Scrape with CSS Selectors

```go
scraped, err := client.Scrape(ctx,
    webfetch.PageOptions{URL: "https://example.com"},
    []webfetch.ScrapeSelector{
        {Selector: "h1"},
        {Selector: ".price", Attr: "data-value"},
        {Selector: "meta[name=description]", Attr: "content"},
    },
)
```

### Extract All Links

```go
links, err := client.FetchLinks(ctx, webfetch.PageOptions{
    URL: "https://example.com",
})
```

### Full Snapshot

```go
snapshot, err := client.TakeSnapshot(ctx, webfetch.PageOptions{
    URL: "https://example.com",
})
// snapshot.HTML + snapshot.Screenshot (base64 PNG)
```

---

## API Methods

| Method | Endpoint | Description | Returns |
|--------|----------|-------------|---------|
| `FetchContent` | `/content` | Rendered HTML | `string` |
| `FetchMarkdown` | `/markdown` | Markdown conversion | `string` |
| `TakeScreenshot` | `/screenshot` | PNG/JPEG screenshot | `[]byte` |
| `Scrape` | `/scrape` | CSS selector extraction | `[]ScrapeResult` |
| `FetchLinks` | `/links` | All anchor hrefs | `[]string` |
| `TakeSnapshot` | `/snapshot` | HTML + base64 screenshot | `Snapshot` |

---

## Page Options

```go
type PageOptions struct {
    URL             string `json:"url"`
    WaitForSelector string `json:"waitForSelector,omitempty"`
    WaitForTimeout  int    `json:"waitForTimeout,omitempty"`
    JavaScript      string `json:"javascript,omitempty"`
    UserAgent       string `json:"userAgent,omitempty"`
    Cookies         []Cookie `json:"cookies,omitempty"`
}
```

| Field | Type | Description |
|-------|------|-------------|
| `URL` | string | Target URL (required) |
| `WaitForSelector` | string | Wait for CSS selector to appear |
| `WaitForTimeout` | int | Max wait time in milliseconds |
| `JavaScript` | string | JS to execute before capture |
| `UserAgent` | string | Custom user agent |
| `Cookies` | []Cookie | Cookies to inject |

---

## Screenshot Options

```go
type ScreenshotOptions struct {
    Width   int                `json:"width"`
    Height  int                `json:"height"`
    Format  ScreenshotFormat   `json:"format"`
    FullPage bool              `json:"fullPage"`
}

type ScreenshotFormat string

const (
    ScreenshotFormatPNG  ScreenshotFormat = "png"
    ScreenshotFormatJPEG ScreenshotFormat = "jpeg"
)
```

---

## Scrape Selectors

```go
type ScrapeSelector struct {
    Selector string `json:"selector"`
    Attr     string `json:"attr,omitempty"`
}

type ScrapeResult struct {
    Selector string   `json:"selector"`
    Values   []string `json:"values"`
}
```

If `Attr` is empty, the inner text is extracted. If `Attr` is set, that attribute is extracted.

---

## Error Handling

All methods return `(Result, error)`. Errors include:

- `webfetch.ErrInvalidURL` — URL is empty or malformed
- `webfetch.ErrCloudflareError` — Cloudflare API returned an error
- `webfetch.ErrTimeout` — Request timed out
- `webfetch.ErrSelectorNotFound` — `WaitForSelector` did not match

### Retry Behavior

The client automatically retries on transient Cloudflare errors with exponential backoff (3 attempts by default).

---

## Examples

### Fetch Documentation as Markdown

```go
md, err := client.FetchMarkdown(ctx, webfetch.PageOptions{
    URL: "https://pkg.go.dev/net/http",
})
```

### Screenshot a Dashboard

```go
ss, err := client.TakeScreenshot(ctx,
    webfetch.PageOptions{
        URL: "https://dashboard.example.com",
        WaitForSelector: ".chart-loaded",
        WaitForTimeout: 10000,
    },
    &webfetch.ScreenshotOptions{
        Width:  1920,
        Height: 1080,
        Format: webfetch.ScreenshotFormatPNG,
    },
)
```

### Scrape Product Prices

```go
results, err := client.Scrape(ctx,
    webfetch.PageOptions{URL: "https://store.example.com/products"},
    []webfetch.ScrapeSelector{
        {Selector: ".product-name"},
        {Selector: ".product-price", Attr: "data-price"},
    },
)
```
