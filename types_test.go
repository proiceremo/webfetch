package webfetch

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestContentResponseUnmarshalRealWireFormat is the regression test for the
// original bug: every endpoint returns an envelope with `success`, `errors`,
// `result`, and (sometimes) `meta`. The previous Go struct looked for
// top-level `content`, `title`, and `status` — fields that don't exist —
// so every response unmarshalled to all-zero values. This locks in the
// correct decoding.
func TestContentResponseUnmarshalRealWireFormat(t *testing.T) {
	wire := []byte(`{
		"success": true,
		"errors": [],
		"meta": { "status": 200, "title": "Example Domain" },
		"result": "<html><body>Hello</body></html>"
	}`)
	var resp ContentResponse
	if err := json.Unmarshal(wire, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Content != "<html><body>Hello</body></html>" {
		t.Fatalf("Content not populated; got %q", resp.Content)
	}
	if resp.Title != "Example Domain" {
		t.Fatalf("Title not pulled from meta; got %q", resp.Title)
	}
	if resp.Status != 200 {
		t.Fatalf("Status not pulled from meta; got %d", resp.Status)
	}
}

func TestContentResponseSurfacesAPIError(t *testing.T) {
	wire := []byte(`{
		"success": false,
		"errors": [{"code": 2001, "message": "Rate limit exceeded"}]
	}`)
	var resp ContentResponse
	err := json.Unmarshal(wire, &resp)
	if err == nil {
		t.Fatal("expected error for success=false; got nil")
	}
	if !strings.Contains(err.Error(), "Rate limit exceeded") {
		t.Fatalf("error must surface the API message; got %v", err)
	}
	if !strings.Contains(err.Error(), "2001") {
		t.Fatalf("error must surface the API code; got %v", err)
	}
}

func TestContentResponseHandlesNullResult(t *testing.T) {
	wire := []byte(`{"success": true, "result": null}`)
	var resp ContentResponse
	if err := json.Unmarshal(wire, &resp); err != nil {
		t.Fatalf("null result should not error; got %v", err)
	}
	if resp.Content != "" {
		t.Fatalf("expected empty content for null result; got %q", resp.Content)
	}
}

func TestMarkdownResponseUnmarshalRealWireFormat(t *testing.T) {
	wire := []byte(`{
		"success": true,
		"result": "# Example\n\nHello world."
	}`)
	var resp MarkdownResponse
	if err := json.Unmarshal(wire, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Content != "# Example\n\nHello world." {
		t.Fatalf("markdown content not populated; got %q", resp.Content)
	}
}

func TestSnapshotResponseUnmarshalRealWireFormat(t *testing.T) {
	wire := []byte(`{
		"success": true,
		"meta": { "status": 200, "title": "Example Domain" },
		"result": {
			"content": "<html>snapshot</html>",
			"screenshot": "base64data=="
		}
	}`)
	var resp SnapshotResponse
	if err := json.Unmarshal(wire, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Content != "<html>snapshot</html>" {
		t.Fatalf("snapshot content not pulled from nested result.content; got %q", resp.Content)
	}
	if resp.Screenshot != "base64data==" {
		t.Fatalf("snapshot screenshot not pulled from nested result.screenshot; got %q", resp.Screenshot)
	}
	if resp.Title != "Example Domain" || resp.Status != 200 {
		t.Fatalf("snapshot meta not pulled correctly; got title=%q status=%d", resp.Title, resp.Status)
	}
}

func TestLinksResponseUnmarshalAsStringArray(t *testing.T) {
	// Cloudflare returns result as `array of string`. The previous Go type
	// modelled it as `[]LinkItem` — an object array — which never decoded
	// anything, so Links was always empty.
	wire := []byte(`{
		"success": true,
		"result": ["https://example.com/", "https://example.com/page"]
	}`)
	var resp LinksResponse
	if err := json.Unmarshal(wire, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Links) != 2 {
		t.Fatalf("expected 2 links; got %d (%v)", len(resp.Links), resp.Links)
	}
	if resp.Links[0] != "https://example.com/" {
		t.Fatalf("first link wrong: %q", resp.Links[0])
	}
}

func TestScrapeResponseUnmarshalRealWireFormat(t *testing.T) {
	wire := []byte(`{
		"success": true,
		"result": [
			{
				"selector": "h1",
				"results": [
					{
						"text": "Example Domain",
						"html": "Example Domain",
						"top": 133.4,
						"left": 100,
						"width": 600,
						"height": 39,
						"attributes": []
					}
				]
			},
			{
				"selector": "a",
				"results": [
					{
						"text": "More information...",
						"html": "More information...",
						"attributes": [
							{"name": "href", "value": "https://www.iana.org/domains/example"}
						]
					}
				]
			}
		]
	}`)
	var resp ScrapeResponse
	if err := json.Unmarshal(wire, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Result) != 2 {
		t.Fatalf("expected 2 scrape items; got %d", len(resp.Result))
	}
	if resp.Result[0].Selector != "h1" {
		t.Fatalf("first selector wrong: %q", resp.Result[0].Selector)
	}
	if got := resp.Result[0].Results[0].Text; got != "Example Domain" {
		t.Fatalf("h1 text wrong: %q", got)
	}
	// Attributes is now correctly decoded as []ScrapeAttribute, not the
	// previous broken map[string]string.
	a := resp.Result[1].Results[0]
	if len(a.Attributes) != 1 || a.Attributes[0].Name != "href" {
		t.Fatalf("href attribute not decoded; got %#v", a.Attributes)
	}
	if got := a.AttributeMap()["href"]; got != "https://www.iana.org/domains/example" {
		t.Fatalf("AttributeMap helper broken; got %q", got)
	}
}

func TestWithSensibleDefaultsFillsGotoOptions(t *testing.T) {
	got := PageOptions{URL: "https://example.com"}.withSensibleDefaults()
	if got.GotoOptions == nil {
		t.Fatal("expected GotoOptions to be filled")
	}
	if got.GotoOptions.WaitUntil != "networkidle2" {
		t.Fatalf("WaitUntil should default to networkidle2; got %q", got.GotoOptions.WaitUntil)
	}
	if got.GotoOptions.Timeout <= 0 {
		t.Fatalf("Timeout should default to a positive value; got %d", got.GotoOptions.Timeout)
	}
	if !got.BestAttempt {
		t.Fatal("BestAttempt should be enabled by default")
	}
}

func TestWithSensibleDefaultsPreservesUserOverrides(t *testing.T) {
	in := PageOptions{
		URL: "https://example.com",
		GotoOptions: &GotoOptions{
			WaitUntil: "load",
			Timeout:   5000,
		},
	}
	got := in.withSensibleDefaults()
	if got.GotoOptions.WaitUntil != "load" {
		t.Fatalf("user WaitUntil should win; got %q", got.GotoOptions.WaitUntil)
	}
	if got.GotoOptions.Timeout != 5000 {
		t.Fatalf("user Timeout should win; got %d", got.GotoOptions.Timeout)
	}
}
