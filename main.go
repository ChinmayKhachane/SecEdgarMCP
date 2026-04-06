package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sec-edgar-mcp/edgar"
	"sec-edgar-mcp/tools"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	userAgent := os.Getenv("SEC_EDGAR_USER_AGENT")
	if userAgent == "" {
		log.Fatal("SEC_EDGAR_USER_AGENT environment variable is required (format: \"FirstName LastName (email@example.com)\")")
	}

	logPath := os.Getenv("SEC_EDGAR_LOG_FILE")
	if logPath == "" {
		logPath = "sec-edgar-mcp.log"
	}
	logFile, err := tools.InitLogger(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open log file %s: %v\n", logPath, err)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	client := edgar.NewClient(userAgent)

	s := server.NewMCPServer(
		"SEC EDGAR MCP",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Register all tool groups.
	tools.RegisterCompanyTools(s, client)
	tools.RegisterFilingTools(s, client)
	tools.RegisterFinancialTools(s, client)
	tools.RegisterInsiderTools(s, client)
	registerUtilityTools(s)

	// Run with stdio transport.
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func registerUtilityTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_recommended_tools",
			mcp.WithDescription("Get recommended tools and usage tips for analyzing specific SEC form types"),
			mcp.WithString("form_type", mcp.Required(), mcp.Description("SEC form type (e.g., 10-K, 10-Q, 8-K, 4, DEF 14A)")),
		),
		tools.WithTiming("get_recommended_tools", getRecommendedTools()),
	)
}

var formRecommendations = map[string]map[string]any{
	"10-K": {
		"description": "Annual Report - comprehensive yearly financial disclosure",
		"recommended_tools": []string{
			"get_financials - Extract income statement, balance sheet, and cash flow",
			"get_filing_sections - Extract specific sections like Risk Factors, MD&A, Business",
			"get_key_metrics - Quick overview of key financial metrics",
			"get_xbrl_concepts - Extract specific XBRL data points",
			"compare_periods - Compare metrics across fiscal years",
		},
		"tips": []string{
			"Use get_filing_sections to read specific parts without loading the entire document",
			"Use get_financials with statement_type='income' for just the income statement",
			"Use compare_periods for year-over-year trend analysis",
		},
	},
	"10-Q": {
		"description": "Quarterly Report - interim financial disclosure",
		"recommended_tools": []string{
			"get_financials - Extract quarterly financial statements",
			"get_filing_sections - Extract specific sections",
			"get_key_metrics - Quick overview of financial metrics",
		},
		"tips": []string{
			"Compare with the same quarter from the prior year for meaningful analysis",
			"Quarterly data may not include all sections found in 10-K",
		},
	},
	"8-K": {
		"description": "Current Report - material events and corporate changes",
		"recommended_tools": []string{
			"analyze_8k - Automatically detect item codes and material events",
			"get_filing_content - Read the full filing text",
		},
		"tips": []string{
			"Item codes indicate the type of event (e.g., 2.02 = Results of Operations)",
			"Check for press releases attached as exhibits",
		},
	},
	"4": {
		"description": "Statement of Changes in Beneficial Ownership - insider trading",
		"recommended_tools": []string{
			"get_form4_details - Detailed transaction and ownership data",
			"get_insider_transactions - List of insider filings",
			"analyze_form4_transactions - Aggregate transaction analysis",
			"analyze_insider_sentiment - Buy/sell sentiment over time",
		},
		"tips": []string{
			"Transaction code 'P' = purchase, 'S' = sale, 'A' = award/grant",
			"Look at post-transaction holdings to see total ownership",
			"Use analyze_insider_sentiment for a high-level view of insider activity",
		},
	},
	"DEF 14A": {
		"description": "Proxy Statement - executive compensation, governance, shareholder votes",
		"recommended_tools": []string{
			"get_filing_content - Read the proxy statement",
			"get_filing_sections - Extract specific sections",
		},
		"tips": []string{
			"Contains executive compensation tables and director information",
			"Look for Say-on-Pay vote results and shareholder proposals",
		},
	},
}

func getRecommendedTools() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		formType, _ := req.RequireString("form_type")

		recs, ok := formRecommendations[formType]
		if !ok {
			recs = map[string]any{
				"description": fmt.Sprintf("Form %s", formType),
				"recommended_tools": []string{
					"get_recent_filings - Find filings of this type",
					"get_filing_content - Read the filing content",
					"get_company_info - Get company details",
				},
				"tips": []string{
					"Use get_recent_filings with form_type filter to find specific filings",
					"Use get_filing_content with pagination for large documents",
				},
			}
		}

		b, _ := tools.MarshalJSON(map[string]any{
			"success":         true,
			"form_type":       formType,
			"recommendations": recs,
		})
		return mcp.NewToolResultText(string(b)), nil
	}
}
