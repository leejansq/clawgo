/*
 * Web Search Tool - 使用 DuckDuckGo HTML 解析实现搜索
 */

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// SearchRequest 搜索请求
type SearchRequest struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

// SearchResult 搜索结果项
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Source  string `json:"source"`
}

// SearchResponse 搜索响应
type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Total   int            `json:"total"`
}

// WebSearchTool Web搜索工具
type WebSearchTool struct {
	httpClient *http.Client
}

const defaultMaxResults = 5

// NewWebSearchTool 创建 Web 搜索工具
func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Info 返回工具信息
func (t *WebSearchTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "web_search",
		Desc: "Search the internet for information, news, and data. Use this to find relevant facts, statistics, case studies, and latest developments for your video script.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:        schema.String,
				Desc:        "The search query in Chinese or English",
				Required:    true,
			},
			"max_results": {
				Type: schema.Integer,
				Desc: "Maximum number of results to return, default 5",
			},
		}),
	}, nil
}

// InvokableRun 执行搜索
func (t *WebSearchTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var req SearchRequest
	if err := json.Unmarshal([]byte(argumentsInJSON), &req); err != nil {
		return "", fmt.Errorf("failed to unmarshal arguments: %w", err)
	}

	if req.MaxResults <= 0 {
		req.MaxResults = defaultMaxResults
	}

	results, err := t.search(ctx, req)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	resp := SearchResponse{
		Results: results,
		Total:   len(results),
	}

	data, _ := json.Marshal(resp)
	return string(data), nil
}

// Run 同步执行搜索（兼容旧接口）
func (t *WebSearchTool) Run(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	if req.MaxResults <= 0 {
		req.MaxResults = defaultMaxResults
	}
	results, err := t.search(ctx, *req)
	if err != nil {
		return nil, err
	}
	return &SearchResponse{Results: results, Total: len(results)}, nil
}

func (t *WebSearchTool) search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	// 使用 DuckDuckGo HTML 搜索
	return t.duckduckgoSearch(ctx, req)
}

func (t *WebSearchTool) duckduckgoSearch(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	// 编码搜索查询
	encodedQuery := url.QueryEscape(req.Query)
	searchURL := fmt.Sprintf("https://duckduckgo.com/html/?q=%s&kl=zh-cn", encodedQuery)

	// 创建 HTTP 请求
	httpReq, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	httpReq.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	// 发送请求
	resp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 解析 HTML
	return t.parseDuckDuckGoHTML(body, req.MaxResults)
}

func (t *WebSearchTool) parseDuckDuckGoHTML(html []byte, maxResults int) ([]SearchResult, error) {
	var results []SearchResult

	// 匹配搜索结果的正则表达式
	// DuckDuckGo HTML 页面结构：<a class="result__a" href="...">Title</a> 和 <a class="result__snippet" href="...">Snippet</a>
	titlePattern := regexp.MustCompile(`<a class="result__a"[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	snippetPattern := regexp.MustCompile(`<a class="result__snippet"[^>]*>(.*?)</a>`)

	// 提取标题和URL
	titleMatches := titlePattern.FindAllSubmatch(html, -1)
	snippetMatches := snippetPattern.FindAllSubmatch(html, -1)

	// 清理 HTML 标签
	cleanHTML := func(s string) string {
		// 移除所有 HTML 标签
		s = regexp.MustCompile(`<[^>]*>`).ReplaceAllString(s, "")
		// 解码 HTML 实体
		s = strings.ReplaceAll(s, "&amp;", "&")
		s = strings.ReplaceAll(s, "&lt;", "<")
		s = strings.ReplaceAll(s, "&gt;", ">")
		s = strings.ReplaceAll(s, "&quot;", "\"")
		s = strings.ReplaceAll(s, "&#39;", "'")
		s = strings.ReplaceAll(s, "&nbsp;", " ")
		// 清理多余空格
		s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
		return strings.TrimSpace(s)
	}

	// 构建结果
	resultMap := make(map[int]*SearchResult)
	for i, match := range titleMatches {
		if i >= maxResults {
			break
		}
		url := string(match[1])
		title := cleanHTML(string(match[2]))
		if title == "" || url == "" {
			continue
		}
		resultMap[i] = &SearchResult{
			Title: title,
			URL:   url,
		}
	}

	// 填充 snippet
	for i, match := range snippetMatches {
		if i >= maxResults {
			break
		}
		snippet := cleanHTML(string(match[1]))
		if snippet == "" {
			continue
		}
		if r, ok := resultMap[i]; ok {
			r.Snippet = snippet
		}
	}

	// 收集结果
	for _, r := range resultMap {
		if r.Title != "" {
			results = append(results, *r)
		}
	}

	// 如果正则匹配失败，尝试备用解析方法
	if len(results) == 0 {
		results = t.fallbackParse(html, maxResults)
	}

	return results, nil
}

func (t *WebSearchTool) fallbackParse(html []byte, maxResults int) []SearchResult {
	var results []SearchResult

	// 简单的备用解析：查找 <h2> 和 <p> 标签
	h2Pattern := regexp.MustCompile(`<h2[^>]*>(.*?)</h2>`)
	pPattern := regexp.MustCompile(`<p>(.*?)</p>`)

	h2Matches := h2Pattern.FindAllSubmatch(html, -1)
	pMatches := pPattern.FindAllSubmatch(html, -1)

	cleanText := func(s string) string {
		s = regexp.MustCompile(`<[^>]*>`).ReplaceAllString(s, "")
		s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
		return strings.TrimSpace(s)
	}

	for i := 0; i < len(h2Matches) && len(results) < maxResults; i++ {
		title := cleanText(string(h2Matches[i][1]))
		if title != "" && len(title) > 5 {
			snippet := ""
			if i < len(pMatches) {
				snippet = cleanText(string(pMatches[i][1]))
			}
			results = append(results, SearchResult{
				Title:   title,
				Snippet: snippet,
			})
		}
	}

	return results
}

// BuildSearchResults 从原始文本构建结构化结果
func BuildSearchResults(query string, htmlContent string) ([]SearchResult, error) {
	tool := NewWebSearchTool()
	clean := regexp.MustCompile(`\s+`).ReplaceAllString(htmlContent, " ")
	if len(clean) > 5000 {
		clean = clean[:5000]
	}
	return tool.parseDuckDuckGoHTML([]byte(clean), 5)
}

// FormatResultsForPrompt 将搜索结果格式化为 prompt 友好的文本
func FormatResultsForPrompt(results []SearchResult) string {
	if len(results) == 0 {
		return "未找到相关搜索结果"
	}

	var buf bytes.Buffer
	buf.WriteString("搜索结果：\n\n")

	for i, r := range results {
		buf.WriteString(fmt.Sprintf("[%d] %s\n", i+1, r.Title))
		buf.WriteString(fmt.Sprintf("   来源: %s\n", r.URL))
		if r.Snippet != "" {
			buf.WriteString(fmt.Sprintf("   摘要: %s\n", r.Snippet))
		}
		buf.WriteString("\n")
	}

	return buf.String()
}
