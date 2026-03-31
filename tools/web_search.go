package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
		Description: "Search the web for current information. Use this tool when the user asks to search, look up, or find information about any topic, person, place, or event. Always extract the core search terms from the user's request.",
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
	ctx, cancel := context.WithTimeout(context.Background(), webSearchTimeout)
	defer cancel()

	escapedQuery := url.QueryEscape(query)
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", escapedQuery)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Hermes-Go/0.6.0; +https://github.com/nousresearch/hermes-go)")

	client := &http.Client{
		Timeout: webSearchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parseDuckDuckGoResults(string(body))
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

		url := extractAttr(block, `<a rel="nofollow" class="result__a"`, `href="`)

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
			URL:     url,
			Snippet: snippet,
		})
	}

	return results, nil
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

type webSearchResponse struct {
	Results []SearchResult `json:"results"`
	Query   string         `json:"query"`
	Count   int            `json:"count"`
}

func (r webSearchResponse) String() string {
	data, _ := json.MarshalIndent(r, "", "  ")
	return string(data)
}
