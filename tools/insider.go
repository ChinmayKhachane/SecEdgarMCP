package tools

import (
	"context"
	"encoding/xml"
	"fmt"
	"math"
	"sec-edgar-mcp/edgar"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func RegisterInsiderTools(s *server.MCPServer, client *edgar.Client) {
	s.AddTool(
		mcp.NewTool("get_insider_transactions",
			mcp.WithDescription("Get insider trading transactions (Forms 3, 4, 5) for a company"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithNumber("days", mcp.Description("Number of days to look back (default: 90)")),
			mcp.WithNumber("limit", mcp.Description("Maximum results (default: 50)")),
		),
		WithTiming("get_insider_transactions", getInsiderTransactions(client)),
	)

	s.AddTool(
		mcp.NewTool("get_insider_summary",
			mcp.WithDescription("Get a summary of insider trading activity for a company including filing counts and unique insiders"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithNumber("days", mcp.Description("Number of days to look back (default: 180)")),
		),
		WithTiming("get_insider_summary", getInsiderSummary(client)),
	)

	s.AddTool(
		mcp.NewTool("get_form4_details",
			mcp.WithDescription("Get detailed Form 4 data including owner info, transactions, and holdings"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("accession_number", mcp.Required(), mcp.Description("Accession number of the Form 4 filing")),
		),
		WithTiming("get_form4_details", getForm4Details(client)),
	)

	s.AddTool(
		mcp.NewTool("analyze_form4_transactions",
			mcp.WithDescription("Analyze Form 4 transactions with detailed share counts, prices, and post-transaction ownership"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithNumber("days", mcp.Description("Number of days to look back (default: 90)")),
			mcp.WithNumber("limit", mcp.Description("Maximum results (default: 50)")),
		),
		WithTiming("analyze_form4_transactions", analyzeForm4Transactions(client)),
	)

	s.AddTool(
		mcp.NewTool("analyze_insider_sentiment",
			mcp.WithDescription("Analyze insider trading sentiment (buy/sell frequency and trends) over a period"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithNumber("months", mcp.Description("Number of months to analyze (default: 6)")),
		),
		WithTiming("analyze_insider_sentiment", analyzeInsiderSentiment(client)),
	)
}

func getInsiderTransactions(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		days := req.GetInt("days", 90)
		limit := req.GetInt("limit", 50)

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		// Get insider form filings (3, 4, 5).
		var allFilings []edgar.FilingInfo
		for _, formType := range []string{"3", "4", "5"} {
			filings, err := client.GetRecentFilings(cik, formType, days, limit)
			if err != nil {
				continue
			}
			allFilings = append(allFilings, filings...)
		}

		// Sort by filing date descending.
		sortFilingsByDate(allFilings)
		if len(allFilings) > limit {
			allFilings = allFilings[:limit]
		}

		transactions := make([]map[string]any, 0, len(allFilings))
		for _, f := range allFilings {
			transactions = append(transactions, map[string]any{
				"accession_number": f.AccessionNumber,
				"filing_date":     f.FilingDate,
				"form_type":       f.FormType,
				"company_name":    f.CompanyName,
				"sec_url":         f.SECURL,
			})
		}

		sub, _ := client.GetSubmissions(cik)
		name := ""
		if sub != nil {
			name = sub.Name
		}

		return jsonResult(map[string]any{
			"success":      true,
			"cik":          cik,
			"name":         name,
			"transactions": transactions,
			"count":        len(transactions),
			"form_types":   []string{"3", "4", "5"},
			"days_back":    days,
		})
	}
}

func getInsiderSummary(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		days := req.GetInt("days", 180)

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		form3, _ := client.GetRecentFilings(cik, "3", days, 200)
		form4, _ := client.GetRecentFilings(cik, "4", days, 200)
		form5, _ := client.GetRecentFilings(cik, "5", days, 200)

		total := len(form3) + len(form4) + len(form5)

		// Collect recent filings.
		var allFilings []edgar.FilingInfo
		allFilings = append(allFilings, form3...)
		allFilings = append(allFilings, form4...)
		allFilings = append(allFilings, form5...)
		sortFilingsByDate(allFilings)

		recentFilings := make([]map[string]any, 0)
		for i, f := range allFilings {
			if i >= 5 {
				break
			}
			recentFilings = append(recentFilings, map[string]any{
				"accession_number": f.AccessionNumber,
				"filing_date":     f.FilingDate,
				"form_type":       f.FormType,
				"sec_url":         f.SECURL,
			})
		}

		sub, _ := client.GetSubmissions(cik)
		name := ""
		if sub != nil {
			name = sub.Name
		}

		return jsonResult(map[string]any{
			"success":     true,
			"cik":         cik,
			"name":        name,
			"period_days": days,
			"summary": map[string]any{
				"total_filings":  total,
				"form_3_count":   len(form3),
				"form_4_count":   len(form4),
				"form_5_count":   len(form5),
				"recent_filings": recentFilings,
			},
		})
	}
}

// XML structures for parsing Form 4.
type ownershipDocument struct {
	XMLName        xml.Name       `xml:"ownershipDocument"`
	Issuer         xmlIssuer      `xml:"issuer"`
	ReportingOwner xmlOwner       `xml:"reportingOwner"`
	NonDerivTxns   []xmlNonDeriv  `xml:"nonDerivativeTable>nonDerivativeTransaction"`
	NonDerivHold   []xmlNonDerivH `xml:"nonDerivativeTable>nonDerivativeHolding"`
	DerivTxns      []xmlDeriv     `xml:"derivativeTable>derivativeTransaction"`
}

type xmlIssuer struct {
	CIK    string `xml:"issuerCik"`
	Name   string `xml:"issuerName"`
	Ticker string `xml:"issuerTradingSymbol"`
}

type xmlOwner struct {
	OwnerID struct {
		CIK  string `xml:"rptOwnerCik"`
		Name string `xml:"rptOwnerName"`
	} `xml:"reportingOwnerId"`
	OwnerRelationship struct {
		IsDirector    string `xml:"isDirector"`
		IsOfficer     string `xml:"isOfficer"`
		IsTenPct      string `xml:"isTenPercentOwner"`
		OfficerTitle  string `xml:"officerTitle"`
	} `xml:"reportingOwnerRelationship"`
}

type xmlNonDeriv struct {
	SecurityTitle struct {
		Value string `xml:"value"`
	} `xml:"securityTitle"`
	TransactionDate struct {
		Value string `xml:"value"`
	} `xml:"transactionDate"`
	TransactionCoding struct {
		Code string `xml:"transactionCode"`
	} `xml:"transactionCoding"`
	TransactionAmounts struct {
		Shares struct {
			Value float64 `xml:"value"`
		} `xml:"transactionShares"`
		PricePerShare struct {
			Value float64 `xml:"value"`
		} `xml:"transactionPricePerShare"`
		AcqDisp struct {
			Value string `xml:"value"`
		} `xml:"transactionAcquiredDisposedCode"`
	} `xml:"transactionAmounts"`
	PostTransaction struct {
		SharesOwned struct {
			Value float64 `xml:"value"`
		} `xml:"sharesOwnedFollowingTransaction"`
	} `xml:"postTransactionAmounts"`
	OwnershipNature struct {
		DirectOrIndirect struct {
			Value string `xml:"value"`
		} `xml:"directOrIndirectOwnership"`
	} `xml:"ownershipNature"`
}

type xmlNonDerivH struct {
	SecurityTitle struct {
		Value string `xml:"value"`
	} `xml:"securityTitle"`
	PostTransaction struct {
		SharesOwned struct {
			Value float64 `xml:"value"`
		} `xml:"sharesOwnedFollowingTransaction"`
	} `xml:"postTransactionAmounts"`
	OwnershipNature struct {
		DirectOrIndirect struct {
			Value string `xml:"value"`
		} `xml:"directOrIndirectOwnership"`
	} `xml:"ownershipNature"`
}

type xmlDeriv struct {
	SecurityTitle struct {
		Value string `xml:"value"`
	} `xml:"securityTitle"`
	TransactionDate struct {
		Value string `xml:"value"`
	} `xml:"transactionDate"`
	TransactionCoding struct {
		Code string `xml:"transactionCode"`
	} `xml:"transactionCoding"`
	TransactionAmounts struct {
		Shares struct {
			Value float64 `xml:"value"`
		} `xml:"transactionShares"`
		PricePerShare struct {
			Value float64 `xml:"value"`
		} `xml:"transactionPricePerShare"`
		AcqDisp struct {
			Value string `xml:"value"`
		} `xml:"transactionAcquiredDisposedCode"`
	} `xml:"transactionAmounts"`
}

func getForm4Details(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		accNum, _ := req.RequireString("accession_number")

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		content, _, err := client.GetFilingContent(cik, accNum)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get Form 4 content: %v", err)), nil
		}

		// Try to parse as XML.
		details, err := parseForm4XML(content)
		if err != nil {
			// Fall back to returning raw text info.
			return jsonResult(map[string]any{
				"success": true,
				"form4_details": map[string]any{
					"accession_number": accNum,
					"parse_note":       "Could not parse XML structure; raw content available via get_filing_content",
					"sec_url":          edgar.BuildSECURL(cik, accNum),
				},
			})
		}

		return jsonResult(map[string]any{
			"success":       true,
			"form4_details": details,
		})
	}
}

func parseForm4XML(content string) (map[string]any, error) {
	// Try to extract the XML portion from the filing.
	xmlContent := content
	if idx := strings.Index(content, "<ownershipDocument"); idx >= 0 {
		xmlContent = content[idx:]
		if end := strings.Index(xmlContent, "</ownershipDocument>"); end >= 0 {
			xmlContent = xmlContent[:end+len("</ownershipDocument>")]
		}
	} else {
		return nil, fmt.Errorf("no ownershipDocument found")
	}

	var doc ownershipDocument
	if err := xml.Unmarshal([]byte(xmlContent), &doc); err != nil {
		return nil, fmt.Errorf("XML parse error: %w", err)
	}

	// Build transactions list.
	transactions := make([]map[string]any, 0)
	for _, tx := range doc.NonDerivTxns {
		transactions = append(transactions, map[string]any{
			"security_title":   tx.SecurityTitle.Value,
			"transaction_date": tx.TransactionDate.Value,
			"transaction_code": tx.TransactionCoding.Code,
			"shares":           tx.TransactionAmounts.Shares.Value,
			"price_per_share":  tx.TransactionAmounts.PricePerShare.Value,
			"acquired_disposed": tx.TransactionAmounts.AcqDisp.Value,
			"shares_owned_after": tx.PostTransaction.SharesOwned.Value,
			"direct_or_indirect": tx.OwnershipNature.DirectOrIndirect.Value,
		})
	}

	for _, tx := range doc.DerivTxns {
		transactions = append(transactions, map[string]any{
			"security_title":    tx.SecurityTitle.Value,
			"transaction_date":  tx.TransactionDate.Value,
			"transaction_code":  tx.TransactionCoding.Code,
			"shares":            tx.TransactionAmounts.Shares.Value,
			"price_per_share":   tx.TransactionAmounts.PricePerShare.Value,
			"acquired_disposed": tx.TransactionAmounts.AcqDisp.Value,
			"type":              "derivative",
		})
	}

	// Build holdings list.
	holdings := make([]map[string]any, 0)
	for _, h := range doc.NonDerivHold {
		holdings = append(holdings, map[string]any{
			"security_title":     h.SecurityTitle.Value,
			"shares_owned":       h.PostTransaction.SharesOwned.Value,
			"direct_or_indirect": h.OwnershipNature.DirectOrIndirect.Value,
		})
	}

	isDirector := doc.ReportingOwner.OwnerRelationship.IsDirector == "1" || strings.EqualFold(doc.ReportingOwner.OwnerRelationship.IsDirector, "true")
	isOfficer := doc.ReportingOwner.OwnerRelationship.IsOfficer == "1" || strings.EqualFold(doc.ReportingOwner.OwnerRelationship.IsOfficer, "true")
	isTenPct := doc.ReportingOwner.OwnerRelationship.IsTenPct == "1" || strings.EqualFold(doc.ReportingOwner.OwnerRelationship.IsTenPct, "true")

	return map[string]any{
		"company": map[string]any{
			"cik":    doc.Issuer.CIK,
			"name":   doc.Issuer.Name,
			"ticker": doc.Issuer.Ticker,
		},
		"owner": map[string]any{
			"cik":                doc.ReportingOwner.OwnerID.CIK,
			"name":               doc.ReportingOwner.OwnerID.Name,
			"is_director":        isDirector,
			"is_officer":         isOfficer,
			"is_ten_pct_owner":   isTenPct,
			"officer_title":      doc.ReportingOwner.OwnerRelationship.OfficerTitle,
		},
		"transactions": transactions,
		"holdings":     holdings,
	}, nil
}

func analyzeForm4Transactions(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		days := req.GetInt("days", 90)
		limit := req.GetInt("limit", 50)

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		filings, err := client.GetRecentFilings(cik, "4", days, limit)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get Form 4 filings: %v", err)), nil
		}

		detailedTxns := make([]map[string]any, 0)
		for _, f := range filings {
			content, _, err := client.GetFilingContent(cik, f.AccessionNumber)
			if err != nil {
				continue
			}
			details, err := parseForm4XML(content)
			if err != nil {
				continue
			}
			details["filing_date"] = f.FilingDate
			details["accession_number"] = f.AccessionNumber
			details["sec_url"] = f.SECURL
			detailedTxns = append(detailedTxns, details)
		}

		sub, _ := client.GetSubmissions(cik)
		name := ""
		if sub != nil {
			name = sub.Name
		}

		return jsonResult(map[string]any{
			"success":               true,
			"cik":                   cik,
			"name":                  name,
			"detailed_transactions": detailedTxns,
			"count":                 len(detailedTxns),
			"days_back":             days,
		})
	}
}

func analyzeInsiderSentiment(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		months := req.GetInt("months", 6)

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		days := months * 30
		filings, err := client.GetRecentFilings(cik, "4", days, 200)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get Form 4 filings: %v", err)), nil
		}

		// Analyze buy/sell patterns.
		buyCount := 0
		sellCount := 0
		totalBuyValue := 0.0
		totalSellValue := 0.0
		insiders := make(map[string]bool)

		for _, f := range filings {
			content, _, err := client.GetFilingContent(cik, f.AccessionNumber)
			if err != nil {
				continue
			}
			details, err := parseForm4XML(content)
			if err != nil {
				continue
			}

			if owner, ok := details["owner"].(map[string]any); ok {
				if name, ok := owner["name"].(string); ok {
					insiders[name] = true
				}
			}

			if txns, ok := details["transactions"].([]map[string]any); ok {
				for _, tx := range txns {
					code, _ := tx["transaction_code"].(string)
					shares, _ := tx["shares"].(float64)
					price, _ := tx["price_per_share"].(float64)

					switch code {
					case "P": // Purchase
						buyCount++
						totalBuyValue += shares * price
					case "S": // Sale
						sellCount++
						totalSellValue += shares * price
					}
				}
			}
		}

		// Determine frequency and trend.
		filingCount := len(filings)
		frequency := "low"
		if filingCount > 20 {
			frequency = "high"
		} else if filingCount > 5 {
			frequency = "moderate"
		}

		sentiment := "neutral"
		if buyCount > sellCount*2 {
			sentiment = "bullish"
		} else if sellCount > buyCount*2 {
			sentiment = "bearish"
		} else if buyCount > sellCount {
			sentiment = "slightly_bullish"
		} else if sellCount > buyCount {
			sentiment = "slightly_bearish"
		}

		sub, _ := client.GetSubmissions(cik)
		name := ""
		if sub != nil {
			name = sub.Name
		}

		return jsonResult(map[string]any{
			"success":       true,
			"cik":           cik,
			"name":          name,
			"period_months": months,
			"analysis": map[string]any{
				"filing_count":     filingCount,
				"frequency":        frequency,
				"buy_count":        buyCount,
				"sell_count":       sellCount,
				"total_buy_value":  math.Round(totalBuyValue*100) / 100,
				"total_sell_value": math.Round(totalSellValue*100) / 100,
				"unique_insiders":  len(insiders),
				"sentiment":        sentiment,
			},
		})
	}
}

func sortFilingsByDate(filings []edgar.FilingInfo) {
	for i := 0; i < len(filings); i++ {
		for j := i + 1; j < len(filings); j++ {
			di, _ := time.Parse("2006-01-02", filings[i].FilingDate)
			dj, _ := time.Parse("2006-01-02", filings[j].FilingDate)
			if dj.After(di) {
				filings[i], filings[j] = filings[j], filings[i]
			}
		}
	}
}
