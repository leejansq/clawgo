package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ============================================================================
// 股票/ETF 查询工具 - 使用 Yahoo Finance API
// ============================================================================

// StockTool 股票查询工具
type StockTool struct {
	httpClient *http.Client
}

// NewStockTool 创建股票查询工具
func NewStockTool() *StockTool {
	return &StockTool{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// GetQuote 获取股票/ETF 实时报价
func (t *StockTool) GetQuote(ctx context.Context, symbol string) (*QuoteResult, error) {
	symbol = strings.ToUpper(symbol)

	// Yahoo Finance v8 API
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d", symbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	var result yahooChartResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v, body: %s", err, string(body))
	}

	if len(result.Chart.Result) == 0 {
		return nil, fmt.Errorf("未找到股票 %s", symbol)
	}

	r := result.Chart.Result[0]
	meta := r.Meta
	quote := r.Indicators.Quote[0]

	currentPrice := meta.PreviousClose
	if len(r.Timestamp) > 0 {
		currentPrice = meta.RegularMarketPrice
	}

	return &QuoteResult{
		Symbol:         meta.Symbol,
		RegularPrice:   currentPrice,
		RegularChange: meta.RegularMarketChange,
		PercentChange:  meta.RegularMarketChangePercent,
		Open:           getFloat(quote.Open, 0),
		High:           getFloat(quote.High, 0),
		Low:            getFloat(quote.Low, 0),
		Volume:         int64(meta.RegularMarketVolume),
		MarketCap:      0,
		PE:             0,
		EPS:            0,
		YearHigh:       meta.FiftyTwoWeekHigh,
		YearLow:        meta.FiftyTwoWeekLow,
		Exchange:       meta.Exchange,
		Currency:       meta.Currency,
		Timestamp:      time.Now().Unix(),
	}, nil
}

// GetHistorical 获取历史K线数据
func (t *StockTool) GetHistorical(ctx context.Context, symbol string, period string, interval string) (*ChartResult, error) {
	symbol = strings.ToUpper(symbol)

	// 解析时间范围
	rangeStr := period
	if rangeStr == "" {
		rangeStr = "1mo"
	}

	// 解析K线间隔
	intervalStr := interval
	if intervalStr == "" {
		intervalStr = "1d"
	}

	// Yahoo Finance v8 API
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=%s&range=%s", symbol, intervalStr, rangeStr)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	var result yahooChartResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if len(result.Chart.Result) == 0 {
		return nil, fmt.Errorf("未找到股票 %s", symbol)
	}

	r := result.Chart.Result[0]
	quotes := r.Indicators.Quote[0]

	var bars []KLineBar
	for i := 0; i < len(r.Timestamp) && i < len(quotes.Close); i++ {
		if quotes.Close[i] == nil {
			continue
		}
		bars = append(bars, KLineBar{
			Timestamp: r.Timestamp[i],
			Open:      getFloat(quotes.Open, i),
			High:      getFloat(quotes.High, i),
			Low:       getFloat(quotes.Low, i),
			Close:     getFloat(quotes.Close, i),
			Volume:    getInt64(quotes.Volume, i),
		})
	}

	startTime := int64(0)
	endTime := int64(0)
	if len(r.Timestamp) > 0 {
		startTime = r.Timestamp[0]
		endTime = r.Timestamp[len(r.Timestamp)-1]
	}

	return &ChartResult{
		Symbol:    symbol,
		Period:    period,
		Interval:  interval,
		Bars:      bars,
		StartTime: startTime,
		EndTime:   endTime,
	}, nil
}

// Analyze 分析股票并给出投资建议
func (t *StockTool) Analyze(ctx context.Context, symbol string) (*AnalysisResult, error) {
	symbol = strings.ToUpper(symbol)

	// 获取实时报价
	quoteResult, err := t.GetQuote(ctx, symbol)
	if err != nil {
		return nil, err
	}

	// 获取历史数据 (6个月)
	chartResult, err := t.GetHistorical(ctx, symbol, "6mo", "1d")
	if err != nil {
		return nil, err
	}

	// 计算技术指标
	analysis := analyzeData(quoteResult, chartResult)

	return analysis, nil
}

// Search 搜索股票 (使用 Yahoo Finance)
func (t *StockTool) Search(ctx context.Context, query string) ([]SearchResult, error) {
	// Yahoo Finance 搜索 API
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v1/finance/search?q=%s&quotesCount=10&newsCount=0", query)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	var result yahooSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if len(result.Quotes) == 0 {
		return nil, fmt.Errorf("未找到相关股票")
	}

	var searchResults []SearchResult
	for _, q := range result.Quotes {
		if q.Symbol == "" {
			continue
		}
		resultType := "stock"
		if strings.Contains(q.QuoteType, "ETF") {
			resultType = "etf"
		}
		searchResults = append(searchResults, SearchResult{
			Symbol:   q.Symbol,
			Name:     q.ShortName,
			Exchange: q.Exchange,
			Type:     resultType,
		})
	}

	return searchResults, nil
}

// ============================================================================
// Yahoo Finance API 响应结构
// ============================================================================

type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol                   string  `json:"symbol"`
				RegularMarketPrice       float64 `json:"regularMarketPrice"`
				PreviousClose            float64 `json:"previousClose"`
				RegularMarketChange      float64 `json:"regularMarketChange"`
				RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
				RegularMarketVolume      int64   `json:"regularMarketVolume"`
				FiftyTwoWeekHigh        float64 `json:"fiftyTwoWeekHigh"`
				FiftyTwoWeekLow         float64 `json:"fiftyTwoWeekLow"`
				Exchange                 string  `json:"exchangeName"`
				Currency                string  `json:"currency"`
			} `json:"meta"`
			Timestamp []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*int64   `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
	} `json:"chart"`
}

type yahooSearchResponse struct {
	Quotes []struct {
		Symbol    string `json:"symbol"`
		ShortName string `json:"shortname"`
		Exchange  string `json:"exchange"`
		QuoteType string `json:"quoteType"`
	} `json:"quotes"`
}

// ============================================================================
// 数据结构
// ============================================================================

// QuoteResult 报价结果
type QuoteResult struct {
	Symbol         string  `json:"symbol"`
	RegularPrice   float64 `json:"regular_price"`
	RegularChange float64 `json:"regular_change"`
	PercentChange float64 `json:"percent_change"`
	Open          float64 `json:"open"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Volume        int64   `json:"volume"`
	MarketCap     int64   `json:"market_cap"`
	PE            float64 `json:"pe"`
	EPS           float64 `json:"eps"`
	YearHigh      float64 `json:"year_high"`
	YearLow       float64 `json:"year_low"`
	Exchange      string  `json:"exchange"`
	Currency      string  `json:"currency"`
	Timestamp     int64   `json:"timestamp"`
}

// ChartResult K线数据
type ChartResult struct {
	Symbol    string     `json:"symbol"`
	Period    string     `json:"period"`
	Interval  string     `json:"interval"`
	Bars      []KLineBar `json:"bars"`
	StartTime int64      `json:"start_time"`
	EndTime   int64      `json:"end_time"`
}

// KLineBar K线柱
type KLineBar struct {
	Timestamp int64   `json:"timestamp"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    int64   `json:"volume"`
}

// SearchResult 搜索结果
type SearchResult struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Exchange string `json:"exchange"`
	Type     string `json:"type"`
}

// AnalysisResult 分析结果
type AnalysisResult struct {
	Symbol            string               `json:"symbol"`
	CurrentPrice      float64              `json:"current_price"`
	Change            float64              `json:"change"`
	ChangePercent     float64              `json:"change_percent"`
	Analysis          string               `json:"analysis"`
	Recommendation   Recommendation      `json:"recommendation"`
	TechnicalIndicators TechnicalIndicators `json:"technical_indicators"`
	RiskLevel         string               `json:"risk_level"`
}

// Recommendation 投资建议
type Recommendation struct {
	Suitability string   `json:"suitability"`
	Rating      string   `json:"rating"`
	Reasons     []string `json:"reasons"`
	RiskLevel  string   `json:"risk_level"`
}

// TechnicalIndicators 技术指标
type TechnicalIndicators struct {
	MA5        float64 `json:"ma5"`
	MA10       float64 `json:"ma10"`
	MA20       float64 `json:"ma20"`
	MA60       float64 `json:"ma60"`
	RSI        float64 `json:"rsi"`
	MACD       string  `json:"macd"`
	Trend      string  `json:"trend"`
	Volatility string  `json:"volatility"`
}

// ============================================================================
// 辅助函数
// ============================================================================

func getFloat(arr []*float64, i int) float64 {
	if i >= len(arr) || arr[i] == nil {
		return 0
	}
	return *arr[i]
}

func getInt64(arr []*int64, i int) int64 {
	if i >= len(arr) || arr[i] == nil {
		return 0
	}
	return *arr[i]
}

// ============================================================================
// 技术分析
// ============================================================================

func analyzeData(q *QuoteResult, c *ChartResult) *AnalysisResult {
	result := &AnalysisResult{
		Symbol:        q.Symbol,
		CurrentPrice:  q.RegularPrice,
		Change:        q.RegularChange,
		ChangePercent: q.PercentChange,
	}

	// 计算技术指标
	tech := calculateTechnicalIndicators(c.Bars)
	result.TechnicalIndicators = tech

	// 生成分析
	analysis := generateAnalysis(q, tech)
	result.Analysis = analysis

	// 给出投资建议
	result.Recommendation = generateRecommendation(q, tech)

	// 风险等级
	result.RiskLevel = result.Recommendation.RiskLevel

	return result
}

func calculateTechnicalIndicators(bars []KLineBar) TechnicalIndicators {
	tech := TechnicalIndicators{}

	if len(bars) == 0 {
		return tech
	}

	// 计算移动平均线
	calcMA := func(n int) float64 {
		if len(bars) < n {
			return 0
		}
		sum := 0.0
		for i := len(bars) - n; i < len(bars); i++ {
			sum += bars[i].Close
		}
		return sum / float64(n)
	}

	tech.MA5 = calcMA(5)
	tech.MA10 = calcMA(10)
	tech.MA20 = calcMA(20)
	tech.MA60 = calcMA(60)

	// 判断趋势
	if tech.MA5 > tech.MA20 {
		tech.Trend = "上涨"
	} else if tech.MA5 < tech.MA20 {
		tech.Trend = "下跌"
	} else {
		tech.Trend = "震荡"
	}

	// 计算RSI
	tech.RSI = calculateRSI(bars)

	// 计算波动性
	if len(bars) >= 20 {
		sum := 0.0
		mean := 0.0
		for i := len(bars) - 20; i < len(bars); i++ {
			mean += bars[i].Close
		}
		mean /= 20
		for i := len(bars) - 20; i < len(bars); i++ {
			d := bars[i].Close - mean
			sum += d * d
		}
		variance := sum / 20
		volatility := variance / mean * 100

		if volatility > 5 {
			tech.Volatility = "高"
		} else if volatility > 2 {
			tech.Volatility = "中"
		} else {
			tech.Volatility = "低"
		}
	}

	return tech
}

func calculateRSI(bars []KLineBar) float64 {
	if len(bars) < 14 {
		return 50
	}

	var gains, losses float64
	for i := len(bars) - 14; i < len(bars)-1; i++ {
		change := bars[i+1].Close - bars[i].Close
		if change > 0 {
			gains += change
		} else {
			losses -= change
		}
	}

	avgGain := gains / 14
	avgLoss := losses / 14

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))
	return rsi
}

func generateAnalysis(q *QuoteResult, tech TechnicalIndicators) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📊 %s 当前分析\n\n", q.Symbol))

	// 基本面
	sb.WriteString("【基本面】\n")
	sb.WriteString(fmt.Sprintf("• 当前价格: $%.2f\n", q.RegularPrice))
	sb.WriteString(fmt.Sprintf("• 涨跌: $%.2f (%.2f%%)\n", q.RegularChange, q.PercentChange))
	sb.WriteString(fmt.Sprintf("• 52周范围: $%.2f - $%.2f\n", q.YearLow, q.YearHigh))
	if q.PE > 0 {
		sb.WriteString(fmt.Sprintf("• 市盈率(PE): %.2f\n", q.PE))
	}
	sb.WriteString(fmt.Sprintf("• 成交量: %d\n\n", q.Volume))

	// 技术面
	sb.WriteString("【技术面】\n")
	sb.WriteString(fmt.Sprintf("• 短期趋势: %s\n", tech.Trend))
	sb.WriteString(fmt.Sprintf("• 5日均线: $%.2f\n", tech.MA5))
	sb.WriteString(fmt.Sprintf("• 20日均线: $%.2f\n", tech.MA20))
	sb.WriteString(fmt.Sprintf("• RSI(14): %.2f\n", tech.RSI))
	sb.WriteString(fmt.Sprintf("• 波动性: %s\n\n", tech.Volatility))

	return sb.String()
}

func generateRecommendation(q *QuoteResult, tech TechnicalIndicators) Recommendation {
	rec := Recommendation{
		Reasons: []string{},
	}

	// 计算得分
	score := 0

	// 趋势得分
	if tech.Trend == "上涨" {
		score += 2
		rec.Reasons = append(rec.Reasons, "短期趋势向上")
	} else if tech.Trend == "下跌" {
		score -= 2
		rec.Reasons = append(rec.Reasons, "短期趋势向下")
	}

	// RSI得分
	if tech.RSI < 30 {
		score += 2
		rec.Reasons = append(rec.Reasons, "RSI显示超卖")
	} else if tech.RSI > 70 {
		score -= 1
		rec.Reasons = append(rec.Reasons, "RSI显示超买")
	}

	// 均线得分
	if q.RegularPrice > tech.MA20 {
		score += 1
		rec.Reasons = append(rec.Reasons, "价格高于20日均线")
	}

	// 涨跌幅得分
	if q.PercentChange > 5 {
		score -= 1
		rec.Reasons = append(rec.Reasons, "单日涨幅较大")
	} else if q.PercentChange < -5 {
		score += 1
		rec.Reasons = append(rec.Reasons, "单日跌幅较大，可能存在反弹机会")
	}

	// 波动性
	if tech.Volatility == "高" {
		rec.RiskLevel = "高"
		rec.Suitability = "适合高风险承受能力的投资者"
	} else if tech.Volatility == "中" {
		rec.RiskLevel = "中"
		rec.Suitability = "适合中等风险承受能力的投资者"
	} else {
		rec.RiskLevel = "低"
		rec.Suitability = "适合稳健型投资者"
	}

	// 评级
	if score >= 3 {
		rec.Rating = "强烈推荐"
	} else if score >= 1 {
		rec.Rating = "推荐"
	} else if score >= -1 {
		rec.Rating = "中性"
	} else if score >= -3 {
		rec.Rating = "不推荐"
	} else {
		rec.Rating = "观望"
	}

	// 添加风险提示
	if rec.RiskLevel == "高" {
		rec.Reasons = append(rec.Reasons, "⚠️ 波动性较高，请注意风险")
	}

	return rec
}
