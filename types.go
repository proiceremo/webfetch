// Package webfetch provides a Go client for the Cloudflare Browser Rendering API.
// It supports content fetching (HTML, markdown), screenshots, scraping, and more.
package webfetch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// GotoOptions controls page navigation behavior.
type GotoOptions struct {
	WaitUntil string `json:"waitUntil,omitempty"` // "load", "domcontentloaded", "networkidle0", "networkidle2"
	Timeout   int    `json:"timeout,omitempty"`   // milliseconds
	Referer   string `json:"referer,omitempty"`
}

// ViewportOptions sets the browser viewport dimensions.
type ViewportOptions struct {
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

// Cookie represents a browser cookie for the request.
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
}

// AddScriptTag injects a script into the page.
type AddScriptTag struct {
	URL     string `json:"url,omitempty"`
	Content string `json:"content,omitempty"`
}

// AddStyleTag injects a stylesheet into the page.
type AddStyleTag struct {
	URL     string `json:"url,omitempty"`
	Content string `json:"content,omitempty"`
}

// PageOptions contains the common request body fields shared across endpoints.
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

// withSensibleDefaults returns a copy of PageOptions with safe defaults applied
// for content-extraction calls against JavaScript-heavy pages (most modern
// sites). The Cloudflare default `waitUntil` is `domcontentloaded`, which
// fires before SPA frameworks finish rendering — so the API returns an empty
// or partial page in the common case. Setting `networkidle2` with a generous
// timeout, combined with bestAttempt, dramatically improves the hit rate for
// real-world URLs without affecting users who set their own GotoOptions.
func (p PageOptions) withSensibleDefaults() PageOptions {
	if p.GotoOptions == nil {
		p.GotoOptions = &GotoOptions{
			WaitUntil: "networkidle2",
			Timeout:   45000,
		}
	} else {
		if p.GotoOptions.WaitUntil == "" {
			p.GotoOptions.WaitUntil = "networkidle2"
		}
		if p.GotoOptions.Timeout <= 0 {
			p.GotoOptions.Timeout = 45000
		}
	}
	// Best-attempt lets the call still return partial content when the
	// network idleness condition times out — much better than failing
	// outright on the chunk of the web that never reaches networkidle.
	p.BestAttempt = true
	return p
}

// AuthCredentials for HTTP authentication.
type AuthCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// WaitForSelector waits for a CSS selector to appear before proceeding.
type WaitForSelector struct {
	Selector string `json:"selector"`
	Visible  bool   `json:"visible,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
	Timeout  int    `json:"timeout,omitempty"`
}

// ScrapeSelector defines what to extract when scraping.
type ScrapeSelector struct {
	Selector  string `json:"selector"`
	Type      string `json:"type,omitempty"`      // "text", "html", "attribute"
	Attribute string `json:"attribute,omitempty"` // required when type is "attribute"
}

// ScreenshotOptions controls screenshot output.
type ScreenshotOptions struct {
	Type           string    `json:"type,omitempty"`    // "png" or "jpeg"
	Quality        int       `json:"quality,omitempty"` // 0-100, jpeg only
	FullPage       bool      `json:"fullPage,omitempty"`
	Clip           *ClipRect `json:"clip,omitempty"`
	OmitBackground bool      `json:"omitBackground,omitempty"`
}

// ClipRect specifies the area to capture.
type ClipRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// --- Response types -------------------------------------------------------
//
// Every Cloudflare Browser Rendering endpoint wraps its payload in the same
// envelope:
//
//   { "success": bool, "errors": [...], "result": <endpoint-specific>, "meta": {...} }
//
// `result` is endpoint-specific — a string for /content and /markdown, an
// object for /snapshot, an array of strings for /links, an array of scrape
// items for /scrape. `meta` (when present) carries the page-level title and
// HTTP status the headless browser observed. Earlier versions of this file
// modelled every response as a flat struct mapping top-level `content`,
// `title`, `status` — but those fields never existed at the top level, so
// the structs unmarshalled silently to all-zero values for every single
// call. That bug was the root cause of the long-standing "webfetch returns
// empty content" symptom.
//
// We now implement UnmarshalJSON on each response type so the public field
// names stay readable (Content, Title, Status, Links, …) while the wire
// payload is decoded from the correct envelope path.

// APIError mirrors one entry from the Cloudflare `errors` array.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// apiEnvelope is the wire shape every Browser Rendering endpoint returns.
// success / errors are universal; result is endpoint-specific and decoded
// by the caller via the json.RawMessage; meta is present on the endpoints
// that surface page-level orientation (status, title).
type apiEnvelope struct {
	Success bool            `json:"success"`
	Errors  []APIError      `json:"errors,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Meta    *struct {
		Status int    `json:"status,omitempty"`
		Title  string `json:"title,omitempty"`
	} `json:"meta,omitempty"`
}

// requireSuccess turns the envelope's `success: false + errors[]` shape into
// a Go error so callers don't silently get an empty payload. Cloudflare
// returns HTTP 200 even for endpoint-level failures (rate limit, invalid
// options, content-signal denials), so the success flag is the only
// reliable signal.
func (e *apiEnvelope) requireSuccess() error {
	if e.Success {
		return nil
	}
	if len(e.Errors) == 0 {
		return fmt.Errorf("cloudflare browser-rendering: success=false with no error details")
	}
	msgs := make([]string, 0, len(e.Errors))
	for _, err := range e.Errors {
		msgs = append(msgs, fmt.Sprintf("[%d] %s", err.Code, err.Message))
	}
	return fmt.Errorf("cloudflare browser-rendering: %s", strings.Join(msgs, "; "))
}

// resultIsNull reports whether the envelope's result is missing or the JSON
// literal `null`. Both cases mean "no payload" and should NOT be unmarshalled.
func (e *apiEnvelope) resultIsNull() bool {
	trimmed := bytes.TrimSpace(e.Result)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

// ContentResponse is the response from /browser-rendering/content.
// The HTML payload lives in `result` (string); the page title and observed
// HTTP status come from the envelope's `meta` block.
type ContentResponse struct {
	Content string `json:"content"`
	Title   string `json:"title,omitempty"`
	Status  int    `json:"status,omitempty"`
}

func (r *ContentResponse) UnmarshalJSON(data []byte) error {
	var env apiEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	if err := env.requireSuccess(); err != nil {
		return err
	}
	if !env.resultIsNull() {
		if err := json.Unmarshal(env.Result, &r.Content); err != nil {
			return fmt.Errorf("decode /content result: %w", err)
		}
	}
	if env.Meta != nil {
		r.Title = env.Meta.Title
		r.Status = env.Meta.Status
	}
	return nil
}

// MarkdownResponse is the response from /browser-rendering/markdown.
// Same envelope shape as ContentResponse — markdown payload lives in
// `result` and the page metadata in `meta`.
type MarkdownResponse struct {
	Content string `json:"content"`
	Title   string `json:"title,omitempty"`
	Status  int    `json:"status,omitempty"`
}

func (r *MarkdownResponse) UnmarshalJSON(data []byte) error {
	var env apiEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	if err := env.requireSuccess(); err != nil {
		return err
	}
	if !env.resultIsNull() {
		if err := json.Unmarshal(env.Result, &r.Content); err != nil {
			return fmt.Errorf("decode /markdown result: %w", err)
		}
	}
	if env.Meta != nil {
		r.Title = env.Meta.Title
		r.Status = env.Meta.Status
	}
	return nil
}

// ScreenshotResponse contains binary screenshot data. The screenshot
// endpoint returns raw image bytes (not the JSON envelope), so this type
// is populated directly by the client and does NOT need a custom decoder.
type ScreenshotResponse struct {
	Data        []byte `json:"-"`
	ContentType string `json:"-"`
}

// ScrapeResponse is the response from /browser-rendering/scrape.
//
// Wire shape: `result` is an array of objects with `selector` and `results`,
// where `results` itself is an array of per-element matches. Each match
// carries text/html plus attributes — and `attributes` is an array of
// {name, value} objects, NOT the map[string]string the older type modelled.
type ScrapeResponse struct {
	Result []ScrapeItem `json:"result"`
}

func (r *ScrapeResponse) UnmarshalJSON(data []byte) error {
	var env apiEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	if err := env.requireSuccess(); err != nil {
		return err
	}
	if env.resultIsNull() {
		return nil
	}
	return json.Unmarshal(env.Result, &r.Result)
}

// ScrapeItem is a single (selector, matches) tuple as returned by /scrape.
type ScrapeItem struct {
	Selector string          `json:"selector"`
	Results  []ScrapeElement `json:"results"`
}

// ScrapeElement is one DOM element that matched the selector. The
// attribute list is keyed by `name`/`value` in the wire format, so we
// model it the same way and expose AttributeMap as a convenience.
type ScrapeElement struct {
	Text       string             `json:"text,omitempty"`
	HTML       string             `json:"html,omitempty"`
	Attributes []ScrapeAttribute  `json:"attributes,omitempty"`
	Height     float64            `json:"height,omitempty"`
	Width      float64            `json:"width,omitempty"`
	Top        float64            `json:"top,omitempty"`
	Left       float64            `json:"left,omitempty"`
}

// ScrapeAttribute is a single HTML attribute as { name, value }.
type ScrapeAttribute struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// AttributeMap collapses Attributes into a map for callers that prefer
// keyed lookup. Later entries with the same name win.
func (e ScrapeElement) AttributeMap() map[string]string {
	if len(e.Attributes) == 0 {
		return nil
	}
	out := make(map[string]string, len(e.Attributes))
	for _, a := range e.Attributes {
		out[a.Name] = a.Value
	}
	return out
}

// LinksResponse is the response from /browser-rendering/links.
//
// Wire shape: `result` is an array of bare URL strings — not an array of
// `{href, text}` objects as the previous Go type modelled. That mismatch
// meant Links was always an empty slice in practice.
type LinksResponse struct {
	Links []string `json:"links"`
}

func (r *LinksResponse) UnmarshalJSON(data []byte) error {
	var env apiEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	if err := env.requireSuccess(); err != nil {
		return err
	}
	if env.resultIsNull() {
		return nil
	}
	return json.Unmarshal(env.Result, &r.Links)
}

// SnapshotResponse is the response from /browser-rendering/snapshot.
//
// Wire shape: `result` is an object with `content` (HTML string) and
// `screenshot` (base64-encoded image). Page metadata is again under `meta`.
type SnapshotResponse struct {
	Content    string `json:"content"`
	Screenshot string `json:"screenshot"` // base64
	Title      string `json:"title,omitempty"`
	Status     int    `json:"status,omitempty"`
}

func (r *SnapshotResponse) UnmarshalJSON(data []byte) error {
	var env apiEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	if err := env.requireSuccess(); err != nil {
		return err
	}
	if !env.resultIsNull() {
		var inner struct {
			Content    string `json:"content"`
			Screenshot string `json:"screenshot"`
		}
		if err := json.Unmarshal(env.Result, &inner); err != nil {
			return fmt.Errorf("decode /snapshot result: %w", err)
		}
		r.Content = inner.Content
		r.Screenshot = inner.Screenshot
	}
	if env.Meta != nil {
		r.Title = env.Meta.Title
		r.Status = env.Meta.Status
	}
	return nil
}
