package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/nousresearch/hermes-go/llm"
	"github.com/nousresearch/hermes-go/security"
)

const (
	webSearchTimeout       = 10 * time.Second
	webSearchMaxResults    = 5
	webSearchMaxSnippetLen = 500
)

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func RegisterWebSearchTool(registry *Registry) error {
	return registry.Register(&Tool{
		Name:        "web_search",
		Description: "Search the web for current information. When the user asks to search, look up, find something, or asks a question about a topic they want you to look up (like 'search for X', 'what is Y', 'find information about Z'), extract the key search terms from their request and use this tool immediately. Do NOT ask the user for clarification - just call the tool with the extracted query. Supports searching for topics, people, places, events, definitions, news, and other factual information.",
		Schema: DefaultSchema("web_search", "Search the web for information", map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The search query. Use concise, specific keywords extracted from the user's request. Do not include conversational phrases.",
			},
		}, []string{"query"}),
		Handler: func(args map[string]interface{}) string {
			query, ok := args["query"].(string)
			if !ok {
				return llm.ToolResultJSON(false, nil, "query argument is required and must be a string")
			}

			if len(query) > 500 {
				return llm.ToolResultJSON(false, nil, "query too long (max 500 characters)")
			}

			query = strings.TrimSpace(query)
			if query == "" {
				return llm.ToolResultJSON(false, nil, "query cannot be empty")
			}

			results, err := performWebSearch(query)
			if err != nil {
				return llm.ToolResultJSON(false, nil, fmt.Sprintf("search error: %v", err))
			}

			if len(results) == 0 {
				return llm.ToolResultJSON(false, nil, "no results found")
			}

			return llm.ToolResultJSON(true, map[string]interface{}{
				"query":   query,
				"results": results,
				"count":   len(results),
			}, "")
		},
		Parallel: true,
	})
}

func performWebSearch(query string) ([]SearchResult, error) {
	results, err := searchDuckDuckGoInstantAnswer(query)
	if err == nil && len(results) > 0 {
		return results, nil
	}

	results, err = searchDuckDuckGoHTML(query)
	if err == nil && len(results) > 0 {
		return results, nil
	}

	results, err = searchDuckDuckGoLite(query)
	if err == nil && len(results) > 0 {
		return results, nil
	}

	return nil, fmt.Errorf("all search backends failed")
}

func searchDuckDuckGoInstantAnswer(query string) ([]SearchResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), webSearchTimeout)
	defer cancel()

	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: webSearchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("instant answer API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024))
	if err != nil {
		return nil, err
	}

	var iaResp ddgInstantAnswer
	if err := json.Unmarshal(body, &iaResp); err != nil {
		return nil, fmt.Errorf("parse instant answer response: %w", err)
	}

	var results []SearchResult

	if iaResp.AbstractText != "" && iaResp.AbstractURL != "" {
		results = append(results, SearchResult{
			Title:   iaResp.Heading,
			URL:     iaResp.AbstractURL,
			Snippet: iaResp.AbstractText,
		})
	}

	for _, rel := range iaResp.RelatedTopics {
		if len(results) >= webSearchMaxResults {
			break
		}
		if rel.FirstURL != "" && rel.Text != "" {
			results = append(results, SearchResult{
				Title:   rel.Text,
				URL:     rel.FirstURL,
				Snippet: rel.Text,
			})
		}
		if len(results) >= webSearchMaxResults {
			break
		}
		for _, sub := range rel.Topics {
			if len(results) >= webSearchMaxResults {
				break
			}
			if sub.FirstURL != "" && sub.Text != "" {
				results = append(results, SearchResult{
					Title:   sub.Text,
					URL:     sub.FirstURL,
					Snippet: sub.Text,
				})
			}
		}
	}

	return results, nil
}

func searchDuckDuckGoHTML(query string) ([]SearchResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), webSearchTimeout)
	defer cancel()

	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	client := &http.Client{
		Timeout: webSearchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	if err != nil {
		return nil, err
	}

	html := string(body)
	if strings.Contains(html, "anomaly-modal") || strings.Contains(html, "challenge") {
		return nil, fmt.Errorf("CAPTCHA challenge")
	}

	return parseDuckDuckGoResults(html)
}

func searchDuckDuckGoLite(query string) ([]SearchResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), webSearchTimeout)
	defer cancel()

	searchURL := fmt.Sprintf("https://lite.duckduckgo.com/lite/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: webSearchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lite search returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	if err != nil {
		return nil, err
	}

	return parseDuckDuckGoLiteResults(string(body))
}

func parseDuckDuckGoResults(html string) ([]SearchResult, error) {
	var results []SearchResult

	html = strings.ReplaceAll(html, "\n", " ")
	html = strings.ReplaceAll(html, "\r", " ")

	resultBlocks := splitResults(html)
	for _, block := range resultBlocks {
		if len(results) >= webSearchMaxResults {
			break
		}

		title := extractBetween(block, `<a rel="nofollow" class="result__a"`, `</a>`)
		title = extractText(title)

		rawURL := extractAttr(block, `<a rel="nofollow" class="result__a"`, `href="`)
		if rawURL == "" {
			rawURL = extractAttr(block, `<a class="result__a"`, `href="`)
		}
		finalURL := resolveDuckDuckGoURL(rawURL)

		snippet := extractBetween(block, `<a class="result__snippet"`, `</a>`)
		if snippet == "" {
			snippet = extractBetween(block, `<a class="result__snippet"`, `</div>`)
		}
		snippet = extractText(snippet)

		if title == "" || snippet == "" {
			continue
		}

		if len(snippet) > webSearchMaxSnippetLen {
			snippet = snippet[:webSearchMaxSnippetLen] + "..."
		}

		results = append(results, SearchResult{
			Title:   security.Truncate(title, 200),
			URL:     finalURL,
			Snippet: snippet,
		})
	}

	return results, nil
}

func parseDuckDuckGoLiteResults(html string) ([]SearchResult, error) {
	var results []SearchResult

	linkRe := regexp.MustCompile(`rel="nofollow" href="([^"]*)"`)
	titleRe := regexp.MustCompile(`class="result-link"[^>]*>([^<]*)</a>`)
	snippetRe := regexp.MustCompile(`class="result-snippet"[^>]*>([^<]*)</td>`)

	links := linkRe.FindAllStringSubmatch(html, -1)
	titles := titleRe.FindAllStringSubmatch(html, -1)
	snippets := snippetRe.FindAllStringSubmatch(html, -1)

	maxLen := len(links)
	if len(titles) < maxLen {
		maxLen = len(titles)
	}
	if len(snippets) < maxLen {
		maxLen = len(snippets)
	}

	for i := 0; i < maxLen && i < webSearchMaxResults; i++ {
		rawURL := resolveDuckDuckGoURL(links[i][1])
		title := strings.TrimSpace(titles[i][1])
		snippet := strings.TrimSpace(snippets[i][1])

		if title == "" || rawURL == "" {
			continue
		}

		results = append(results, SearchResult{
			Title:   security.Truncate(title, 200),
			URL:     rawURL,
			Snippet: security.Truncate(snippet, webSearchMaxSnippetLen),
		})
	}

	return results, nil
}

func resolveDuckDuckGoURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	if strings.HasPrefix(rawURL, "//duckduckgo.com/l/") || strings.Contains(rawURL, "uddg=") {
		parsed, err := url.Parse(rawURL)
		if err == nil {
			uddg := parsed.Query().Get("uddg")
			if uddg != "" {
				decoded, err := url.QueryUnescape(uddg)
				if err == nil {
					return decoded
				}
			}
		}
	}

	if strings.HasPrefix(rawURL, "//") {
		return "https:" + rawURL
	}

	return rawURL
}

func splitResults(html string) []string {
	var blocks []string
	searchClass := `class="result results_"`
	start := 0

	for {
		idx := strings.Index(html[start:], searchClass)
		if idx == -1 {
			break
		}

		blockStart := start + idx
		blockEnd := strings.Index(html[blockStart:], `<div class="result results_`)
		if blockEnd == -1 {
			blockEnd = len(html) - blockStart
		}

		blocks = append(blocks, html[blockStart:blockStart+blockEnd])
		start = blockStart + blockEnd
	}

	return blocks
}

func extractBetween(html, startMarker, endMarker string) string {
	startIdx := strings.Index(html, startMarker)
	if startIdx == -1 {
		return ""
	}

	content := html[startIdx:]
	endIdx := strings.Index(content, endMarker)
	if endIdx == -1 {
		return ""
	}

	return content[:endIdx+len(endMarker)]
}

func extractAttr(html, tag, attr string) string {
	tagIdx := strings.Index(html, tag)
	if tagIdx == -1 {
		return ""
	}

	attrIdx := strings.Index(html[tagIdx:], attr)
	if attrIdx == -1 {
		return ""
	}

	content := html[tagIdx+attrIdx+len(attr):]
	endIdx := strings.IndexAny(content, `" `)
	if endIdx == -1 {
		return ""
	}

	return content[:endIdx]
}

func extractText(html string) string {
	var result strings.Builder
	inTag := false
	inScript := false
	inStyle := false

	for i := 0; i < len(html); i++ {
		ch := html[i]

		if ch == '<' {
			inTag = true
			if i+7 < len(html) && strings.ToLower(html[i:i+7]) == "<script" {
				inScript = true
			}
			if i+6 < len(html) && strings.ToLower(html[i:i+6]) == "<style" {
				inStyle = true
			}
			continue
		}

		if ch == '>' {
			inTag = false
			if inScript && i >= 7 && strings.ToLower(html[i-7:i]) == "/script" {
				inScript = false
			}
			if inStyle && i >= 6 && strings.ToLower(html[i-6:i]) == "/style" {
				inStyle = false
			}
			continue
		}

		if inTag || inScript || inStyle {
			continue
		}

		if ch == '&' {
			endIdx := strings.Index(html[i:], ";")
			if endIdx != -1 && endIdx < 10 {
				entity := html[i : i+endIdx+1]
				switch entity {
				case "&amp;":
					result.WriteByte('&')
				case "&lt;":
					result.WriteByte('<')
				case "&gt;":
					result.WriteByte('>')
				case "&quot;":
					result.WriteByte('"')
				case "&#39;":
					result.WriteByte('\'')
				case "&nbsp;":
					result.WriteByte(' ')
				default:
					result.WriteString(entity)
				}
				i += endIdx
				continue
			}
		}

		result.WriteByte(ch)
	}

	text := result.String()
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\t", " ")

	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	return strings.TrimSpace(text)
}

type ddgInstantAnswer struct {
	Heading        string `json:"Heading"`
	AbstractText   string `json:"AbstractText"`
	AbstractURL    string `json:"AbstractURL"`
	RelatedTopics  []ddgRelatedTopic
	Disambiguation []ddgRelatedTopic `json:"Results"`
}

type ddgRelatedTopic struct {
	Text     string            `json:"Text"`
	FirstURL string            `json:"FirstURL"`
	Icon     map[string]string `json:"Icon"`
	Name     string            `json:"Name"`
	Topics   []ddgRelatedTopic `json:"Topics"`
}

type webSearchResponse struct {
	Results []SearchResult `json:"results"`
	Query   string         `json:"query"`
	Count   int            `json:"count"`
}

func (r webSearchResponse) String() string {
	data, _ := json.MarshalIndent(r, "", "  ")
	return string(data)
}
