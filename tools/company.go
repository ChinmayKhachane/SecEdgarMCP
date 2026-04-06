package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sec-edgar-mcp/edgar"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var toolLogger *log.Logger

// InitLogger sets up file-based logging. Call from main before registering tools.
func InitLogger(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	toolLogger = log.New(f, "[mcp] ", log.Ldate|log.Ltime|log.Lmsgprefix)
	toolLogger.Println("logger initialized")
	return f, nil
}

// Logger returns the shared logger instance for use by other packages.
func Logger() *log.Logger {
	return toolLogger
}

// WithTiming wraps a tool handler and logs the total time it takes to execute.
func WithTiming(name string, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		result, err := handler(ctx, req)
		elapsed := time.Since(start)
		if toolLogger != nil {
			if err != nil {
				toolLogger.Printf("%-35s %s (error: %v)", name, elapsed, err)
			} else {
				toolLogger.Printf("%-35s %s", name, elapsed)
			}
		}
		return result, err
	}
}

func RegisterCompanyTools(s *server.MCPServer, client *edgar.Client) {
	s.AddTool(
		mcp.NewTool("get_cik_by_ticker",
			mcp.WithDescription("Convert a stock ticker symbol to SEC CIK number"),
			mcp.WithString("ticker", mcp.Required(), mcp.Description("Stock ticker symbol (e.g., AAPL, MSFT)")),
		),
		WithTiming("get_cik_by_ticker", getCIKByTicker(client)),
	)

	s.AddTool(
		mcp.NewTool("get_company_info",
			mcp.WithDescription("Get detailed company information from SEC EDGAR including name, CIK, SIC code, exchange, state, and fiscal year end"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
		),
		WithTiming("get_company_info", getCompanyInfo(client)),
	)

	s.AddTool(
		mcp.NewTool("search_companies",
			mcp.WithDescription("Search for companies by name in SEC EDGAR"),
			mcp.WithString("query", mcp.Required(), mcp.Description("Company name to search for")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of results (default: 10)")),
		),
		WithTiming("search_companies", searchCompanies(client)),
	)

	s.AddTool(
		mcp.NewTool("get_company_facts",
			mcp.WithDescription("Get key financial metrics from XBRL company facts including Assets, Liabilities, Revenue, Net Income, EPS, and more"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
		),
		WithTiming("get_company_facts", getCompanyFacts(client)),
	)
}

func getCIKByTicker(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ticker, _ := req.RequireString("ticker")

		cik, err := client.ResolveCIK(ticker)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve ticker: %v", err)), nil
		}

		return jsonResult(map[string]any{
			"success": true,
			"cik":     cik,
			"ticker":  ticker,
		})
	}
}

func getCompanyInfo(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")

		info, err := client.GetCompanyInfo(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get company info: %v", err)), nil
		}

		return jsonResult(map[string]any{
			"success": true,
			"company": info,
		})
	}
}

func searchCompanies(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, _ := req.RequireString("query")
		limit := req.GetInt("limit", 10)

		results, err := client.SearchCompanies(query, limit)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to search companies: %v", err)), nil
		}

		return jsonResult(map[string]any{
			"success":   true,
			"companies": results,
			"count":     len(results),
		})
	}
}

func getCompanyFacts(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		facts, err := client.GetCompanyFacts(cik)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get company facts: %v", err)), nil
		}

		metricKeys := []string{
			"Assets", "Liabilities", "StockholdersEquity",
			"RevenueFromContractWithCustomerExcludingAssessedTax",
			"NetIncomeLoss", "EarningsPerShareBasic", "EarningsPerShareDiluted",
			"CommonStockSharesOutstanding", "CashAndCashEquivalentsAtCarryingValue",
		}

		metrics := make(map[string]any)
		for _, key := range metricKeys {
			alts := resolveConceptAlternatives(key)
			dp, matched := bestAlternative(facts, "us-gaap", alts, "")
			if dp != nil {
				metrics[key] = map[string]any{
					"value":         dp.Val,
					"end_date":      dp.End,
					"form":          dp.Form,
					"fiscal_year":   dp.FY,
					"fiscal_period": dp.FP,
					"xbrl_concept":  matched,
				}
			}
		}

		return jsonResult(map[string]any{
			"success": true,
			"cik":     cik,
			"name":    facts.EntityName,
			"metrics": metrics,
		})
	}
}

// Shared helpers

// MarshalJSON is exported for use by main package.
func MarshalJSON(data map[string]any) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}

func jsonResult(data map[string]any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(b)), nil
}

func errorResult(msg string) *mcp.CallToolResult {
	b, _ := json.MarshalIndent(map[string]any{
		"success": false,
		"error":   msg,
	}, "", "  ")
	return mcp.NewToolResultError(string(b))
}
