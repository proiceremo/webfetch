//go:build live

// Package webfetch live tests hit the real Cloudflare Browser Rendering API.
//
// Run with:    go test -tags=live ./...
// Skip with:   (default behaviour — these never run unless the tag is on)
//
// Requirements:
//   - CLOUDFLARE_ACCOUNT_ID  set in the environment
//   - CLOUDFLARE_API_TOKEN   set in the environment with `Browser Rendering - Edit`
//   - Outbound HTTPS reach to api.cloudflare.com
//
// Each test skips itself rather than failing when these prerequisites are
// missing so a partial environment (e.g. CI without secrets) still yields a
// clean PASS line for unit tests via the same `go test` invocation.
package webfetch

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// liveTestURL is the canonical IANA test domain. It serves stable HTML with
// an <h1> ("Example Domain") and a single anchor link to iana.org — perfect
// for verifying every endpoint's payload shape without depending on a third
// party that might rate-limit or restructure overnight.
const liveTestURL = "https://example.com/"

// newLiveClient skips the calling test when credentials or the env tag are
// missing. Returns a Client wired to a 60-second timeout, the same as the
// production default.
func newLiveClient(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("CLOUDFLARE_ACCOUNT_ID") == "" || os.Getenv("CLOUDFLARE_API_TOKEN") == "" {
		t.Skip("CLOUDFLARE_ACCOUNT_ID and CLOUDFLARE_API_TOKEN must be set for live webfetch tests")
	}
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// skipIfNetworkUnreachable converts opaque network errors into a skip with a
// useful message. Cloudflare API calls go to api.cloudflare.com; DNS or
// outbound TCP failures shouldn't fail-fail the test in environments without
// internet access (sandboxes, air-gapped CI).
func skipIfNetworkUnreachable(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		t.Skipf("DNS resolution failed (%v) — skipping live test in offline environment", err)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		// net/http wraps connection failures here; substring match is the
		// most reliable form across Go versions.
		msg := urlErr.Error()
		switch {
		case strings.Contains(msg, "no such host"),
			strings.Contains(msg, "connection refused"),
			strings.Contains(msg, "network is unreachable"),
			strings.Contains(msg, "i/o timeout"):
			t.Skipf("network unreachable (%v) — skipping live test", err)
		}
	}
}

func TestLive_FetchContent(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	resp, err := client.FetchContent(ctx, PageOptions{URL: liveTestURL})
	skipIfNetworkUnreachable(t, err)
	if err != nil {
		t.Fatalf("FetchContent: %v", err)
	}
	if resp == nil {
		t.Fatal("FetchContent returned nil response")
	}
	if resp.Content == "" {
		t.Fatal("FetchContent returned empty Content — the regression bug is back")
	}
	if !strings.Contains(strings.ToLower(resp.Content), "<html") {
		t.Fatalf("FetchContent body doesn't look like HTML; first 200 chars: %q", truncate(resp.Content, 200))
	}
	// example.com's <h1> always reads "Example Domain"; this catches the
	// case where the API succeeds but returns someone else's content (an
	// upstream proxy / cache misroute).
	if !strings.Contains(resp.Content, "Example Domain") {
		t.Fatalf("FetchContent body missing the canonical 'Example Domain' marker; first 200 chars: %q", truncate(resp.Content, 200))
	}
	// meta.title comes from the page's <title> tag.
	if resp.Title != "" && !strings.Contains(resp.Title, "Example") {
		t.Logf("WARNING: page title is unexpected: %q", resp.Title)
	}
}

func TestLive_FetchMarkdown(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	resp, err := client.FetchMarkdown(ctx, PageOptions{URL: liveTestURL})
	skipIfNetworkUnreachable(t, err)
	if err != nil {
		t.Fatalf("FetchMarkdown: %v", err)
	}
	if resp.Content == "" {
		t.Fatal("FetchMarkdown returned empty Content")
	}
	if !strings.Contains(resp.Content, "Example Domain") {
		t.Fatalf("FetchMarkdown body missing canonical marker; first 200 chars: %q", truncate(resp.Content, 200))
	}
}

func TestLive_FetchLinks(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	resp, err := client.FetchLinks(ctx, PageOptions{URL: liveTestURL})
	skipIfNetworkUnreachable(t, err)
	if err != nil {
		t.Fatalf("FetchLinks: %v", err)
	}
	if len(resp.Links) == 0 {
		t.Fatal("FetchLinks returned no links — the []LinkItem→[]string fix may have regressed")
	}
	// example.com contains a "More information" link to iana.org.
	foundIANA := false
	for _, link := range resp.Links {
		if strings.Contains(link, "iana.org") {
			foundIANA = true
			break
		}
	}
	if !foundIANA {
		t.Fatalf("Expected at least one link to iana.org from example.com; got %v", resp.Links)
	}
}

func TestLive_TakeScreenshot(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	resp, err := client.TakeScreenshot(ctx, PageOptions{URL: liveTestURL}, nil)
	skipIfNetworkUnreachable(t, err)
	if err != nil {
		t.Fatalf("TakeScreenshot: %v", err)
	}
	if len(resp.Data) < 1024 {
		t.Fatalf("TakeScreenshot returned suspiciously small image (%d bytes); content-type=%q", len(resp.Data), resp.ContentType)
	}
	// Sniff the magic bytes. Default screenshot is PNG.
	switch {
	case len(resp.Data) >= 8 && string(resp.Data[:8]) == "\x89PNG\r\n\x1a\n":
		// PNG — good
	case len(resp.Data) >= 3 && resp.Data[0] == 0xFF && resp.Data[1] == 0xD8 && resp.Data[2] == 0xFF:
		// JPEG — also fine
	default:
		t.Fatalf("Screenshot bytes don't have a known image magic header; first 16 bytes: %x", resp.Data[:min(16, len(resp.Data))])
	}
}

func TestLive_Scrape(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	resp, err := client.Scrape(ctx, PageOptions{URL: liveTestURL}, []ScrapeSelector{
		{Selector: "h1"},
		{Selector: "a"},
	})
	skipIfNetworkUnreachable(t, err)
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if len(resp.Result) == 0 {
		t.Fatal("Scrape returned empty Result")
	}
	// Find the h1 selector item and verify its text.
	var h1Text string
	var anchorHref string
	for _, item := range resp.Result {
		if item.Selector == "h1" && len(item.Results) > 0 {
			h1Text = item.Results[0].Text
		}
		if item.Selector == "a" && len(item.Results) > 0 {
			// AttributeMap regression test: the old map[string]string
			// shape never populated; the new []ScrapeAttribute shape
			// + AttributeMap helper should yield a working href.
			anchorHref = item.Results[0].AttributeMap()["href"]
		}
	}
	if h1Text != "Example Domain" {
		t.Fatalf("Expected h1 text 'Example Domain', got %q", h1Text)
	}
	if !strings.Contains(anchorHref, "iana.org") {
		t.Fatalf("Expected anchor href to contain 'iana.org', got %q", anchorHref)
	}
}

func TestLive_TakeSnapshot(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	resp, err := client.TakeSnapshot(ctx, PageOptions{URL: liveTestURL})
	skipIfNetworkUnreachable(t, err)
	if err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}
	if resp.Content == "" {
		t.Fatal("TakeSnapshot returned empty Content — result.content extraction may have regressed")
	}
	if resp.Screenshot == "" {
		t.Fatal("TakeSnapshot returned empty Screenshot — result.screenshot extraction may have regressed")
	}
	// Screenshot field is base64. Decode at least the first chunk to
	// catch the case where Cloudflare returned a non-base64 placeholder.
	chunk := resp.Screenshot
	if len(chunk) > 64 {
		chunk = chunk[:64]
	}
	if _, err := base64.StdEncoding.DecodeString(chunk); err != nil {
		t.Fatalf("Snapshot screenshot does not look like base64: %v (first 64 chars: %q)", err, chunk)
	}
}

// TestLive_SurfacesAPIError verifies the success=false → Go error path runs
// against the real API. A malformed request (empty URL) is the simplest way
// to provoke an error envelope without abusing rate limits.
func TestLive_SurfacesAPIError(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Empty URL + empty HTML — Cloudflare rejects this with a structured error.
	_, err := client.FetchContent(ctx, PageOptions{})
	skipIfNetworkUnreachable(t, err)
	if err == nil {
		t.Fatal("Expected an error from an empty request body, got nil")
	}
	// The error must mention either the envelope's code/message or the HTTP
	// status — anything more specific would tie the test to the current
	// Cloudflare wording.
	if !strings.Contains(err.Error(), "cloudflare") && !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("Error doesn't look like a structured API error: %v", err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
