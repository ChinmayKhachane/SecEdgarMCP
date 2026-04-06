package tools

import (
	"context"
	"fmt"
	"math"
	"sec-edgar-mcp/edgar"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// conceptGroups defines groups of XBRL concepts that represent the same
// financial metric across different ASC standard migrations. Every key is a
// real XBRL concept name. When any concept in a group is requested, all
// members are tried and the one with the most recent filing date is returned.
var conceptGroups = [][]string{
	// ASC 606 Revenue Recognition (~2018)
	{"RevenueFromContractWithCustomerExcludingAssessedTax", "Revenues", "SalesRevenueNet", "SalesRevenueGoodsNet"},
	// Cost of revenue variants
	{"CostOfRevenue", "CostOfGoodsAndServicesSold"},
	// ASC 842 Leases (~2019)
	{"OperatingLeaseRightOfUseAsset", "OperatingLeaseLiability", "OperatingLeasesRentExpenseNet"},
	// ASC 326 / CECL Credit Losses (~2020)
	{"AccountsReceivableAllowanceForCreditLossExcludingAccruedInterest", "AllowanceForDoubtfulAccountsReceivableCurrent"},
	// Debt taxonomy — companies vary on which tag they use
	{"LongTermDebt", "LongTermDebtNoncurrent"},
}

// conceptGroupIndex maps each XBRL concept to its full group of alternatives.
// Built automatically from conceptGroups at init time.
var conceptGroupIndex = func() map[string][]string {
	m := make(map[string][]string)
	for _, group := range conceptGroups {
		for _, concept := range group {
			m[concept] = group
		}
	}
	return m
}()

// Standard XBRL concepts organized by statement type.
// Each entry is a key into conceptAlternatives, or a standalone concept name.
var incomeStatementConcepts = []string{
	"RevenueFromContractWithCustomerExcludingAssessedTax", "CostOfRevenue",
	"GrossProfit", "OperatingExpenses", "OperatingIncomeLoss",
	"NonoperatingIncomeExpense", "InterestExpense",
	"IncomeLossFromContinuingOperationsBeforeIncomeTaxesExtraordinaryItemsNoncontrollingInterest",
	"IncomeTaxExpenseBenefit", "NetIncomeLoss",
	"EarningsPerShareBasic", "EarningsPerShareDiluted",
}

var balanceSheetConcepts = []string{
	"Assets", "AssetsCurrent", "CashAndCashEquivalentsAtCarryingValue",
	"AccountsReceivableNetCurrent", "AccountsReceivableAllowanceForCreditLossExcludingAccruedInterest", "InventoryNet",
	"AssetsNoncurrent", "PropertyPlantAndEquipmentNet",
	"OperatingLeaseRightOfUseAsset",
	"Goodwill", "IntangibleAssetsNetExcludingGoodwill",
	"Liabilities", "LiabilitiesCurrent", "AccountsPayableCurrent",
	"LiabilitiesNoncurrent", "LongTermDebt",
	"StockholdersEquity", "CommonStockValue",
	"RetainedEarningsAccumulatedDeficit",
}

var cashFlowConcepts = []string{
	"NetCashProvidedByUsedInOperatingActivities",
	"NetCashProvidedByUsedInInvestingActivities",
	"NetCashProvidedByUsedInFinancingActivities",
	"CashAndCashEquivalentsPeriodIncreaseDecrease",
	"DepreciationDepletionAndAmortization",
	"PaymentsToAcquirePropertyPlantAndEquipment",
	"PaymentsOfDividends", "ProceedsFromIssuanceOfDebt", "RepaymentsOfDebt",
}

// defaultKeyMetrics maps a display name to one or more XBRL concept alternatives.
// When multiple alternatives exist, all are fetched and the one with the most
// recent data point is returned.
var defaultKeyMetrics = []string{
	"RevenueFromContractWithCustomerExcludingAssessedTax",
	"NetIncomeLoss",
	"Assets",
	"Liabilities",
	"StockholdersEquity",
	"EarningsPerShareBasic",
	"CommonStockSharesOutstanding",
	"CashAndCashEquivalentsAtCarryingValue",
}

func RegisterFinancialTools(s *server.MCPServer, client *edgar.Client) {
	s.AddTool(
		mcp.NewTool("get_financials",
			mcp.WithDescription("Extract income statement, balance sheet, and/or cash flow data from a company's latest 10-K or 10-Q filing"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("statement_type", mcp.Description("Type of statement: income, balance, cash, or all (default: all)")),
		),
		WithTiming("get_financials", getFinancials(client)),
	)

	s.AddTool(
		mcp.NewTool("get_key_metrics",
			mcp.WithDescription("Get key financial metrics (Revenue, Net Income, Assets, EPS, etc.) from XBRL company facts"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
		),
		WithTiming("get_key_metrics", getKeyMetrics(client)),
	)

	s.AddTool(
		mcp.NewTool("compare_periods",
			mcp.WithDescription("Compare a financial metric across fiscal years with growth calculations and CAGR"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("metric", mcp.Required(), mcp.Description("XBRL concept name (e.g., Revenues, NetIncomeLoss, Assets)")),
			mcp.WithNumber("start_year", mcp.Required(), mcp.Description("Start fiscal year")),
			mcp.WithNumber("end_year", mcp.Required(), mcp.Description("End fiscal year")),
		),
		WithTiming("compare_periods", comparePeriods(client)),
	)

	s.AddTool(
		mcp.NewTool("discover_company_metrics",
			mcp.WithDescription("Discover all available XBRL metrics for a company, optionally filtered by search term"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("search_term", mcp.Description("Optional search filter for metric names")),
		),
		WithTiming("discover_company_metrics", discoverCompanyMetrics(client)),
	)

	s.AddTool(
		mcp.NewTool("get_xbrl_concepts",
			mcp.WithDescription("Extract specific XBRL concept values from a company's filings"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("concepts", mcp.Description("Comma-separated list of XBRL concept names to extract")),
			mcp.WithString("form_type", mcp.Description("Form type to extract from: 10-K or 10-Q (default: 10-K)")),
		),
		WithTiming("get_xbrl_concepts", getXBRLConcepts(client)),
	)

	s.AddTool(
		mcp.NewTool("discover_xbrl_concepts",
			mcp.WithDescription("Discover available XBRL concepts in a company's filings (paginated)"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("form_type", mcp.Description("Form type: 10-K or 10-Q (default: 10-K)")),
			mcp.WithString("namespace_filter", mcp.Description("Optional namespace filter (e.g., us-gaap)")),
			mcp.WithNumber("offset", mcp.Description("Number of results to skip (default: 0)")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of results to return (default: 50, max: 200)")),
		),
		WithTiming("discover_xbrl_concepts", discoverXBRLConcepts(client)),
	)
}

func getFinancials(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		stmtType := req.GetString("statement_type", "all")

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		facts, err := client.GetCompanyFacts(cik)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get company facts: %v", err)), nil
		}

		statements := make(map[string]any)

		if stmtType == "all" || stmtType == "income" {
			statements["income_statement"] = extractConceptValues(facts, incomeStatementConcepts)
		}
		if stmtType == "all" || stmtType == "balance" {
			statements["balance_sheet"] = extractConceptValues(facts, balanceSheetConcepts)
		}
		if stmtType == "all" || stmtType == "cash" {
			statements["cash_flow"] = extractConceptValues(facts, cashFlowConcepts)
		}

		return jsonResult(map[string]any{
			"success":    true,
			"cik":        cik,
			"name":       facts.EntityName,
			"statements": statements,
		})
	}
}

// resolveConceptAlternatives returns all XBRL concepts in the same group as the
// given concept. If the concept belongs to a group (e.g. "Revenues" is grouped
// with "RevenueFromContractWithCustomerExcludingAssessedTax"), all members are
// returned. Otherwise the concept itself is returned as a single-element slice.
func resolveConceptAlternatives(concept string) []string {
	if group, ok := conceptGroupIndex[concept]; ok {
		return group
	}
	return []string{concept}
}

// bestAlternative fetches all alternative concepts and returns the data point
// with the most recent end date along with the concept name that matched.
func bestAlternative(facts *edgar.CompanyFactsResponse, namespace string, alternatives []string) (*edgar.FactDataPoint, string) {
	var best *edgar.FactDataPoint
	bestConcept := ""
	for _, alt := range alternatives {
		dp := edgar.GetLatestMetricValue(facts, namespace, alt)
		if dp != nil && (best == nil || dp.End > best.End) {
			best = dp
			bestConcept = alt
		}
	}
	return best, bestConcept
}

func extractConceptValues(facts *edgar.CompanyFactsResponse, concepts []string) map[string]any {
	result := make(map[string]any)
	for _, concept := range concepts {
		alts := resolveConceptAlternatives(concept)
		dp, matched := bestAlternative(facts, "us-gaap", alts)
		if dp != nil {
			result[concept] = map[string]any{
				"value":         dp.Val,
				"end_date":      dp.End,
				"form":          dp.Form,
				"fiscal_year":   dp.FY,
				"fiscal_period": dp.FP,
				"xbrl_concept":  matched,
			}
		}
	}
	return result
}

func getKeyMetrics(client *edgar.Client) server.ToolHandlerFunc {
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

		metrics := make(map[string]any)
		found := 0
		for _, concept := range defaultKeyMetrics {
			alts := resolveConceptAlternatives(concept)
			best, bestConcept := bestAlternative(facts, "us-gaap", alts)
			if best != nil {
				metrics[concept] = map[string]any{
					"value":         best.Val,
					"end_date":      best.End,
					"form":          best.Form,
					"fiscal_year":   best.FY,
					"fiscal_period": best.FP,
					"xbrl_concept":  bestConcept,
				}
				found++
			}
		}

		return jsonResult(map[string]any{
			"success":           true,
			"cik":               cik,
			"name":              facts.EntityName,
			"metrics":           metrics,
			"requested_metrics": defaultKeyMetrics,
			"found_metrics":     found,
		})
	}
}

func comparePeriods(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		metric, _ := req.RequireString("metric")
		startYear := req.GetInt("start_year", 2020)
		endYear := req.GetInt("end_year", 2024)

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		facts, err := client.GetCompanyFacts(cik)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get company facts: %v", err)), nil
		}

		// Resolve concept alternatives so revenue aliases etc. are all checked.
		ns, ok := facts.Facts["us-gaap"]
		if !ok {
			return errorResult("No us-gaap data available"), nil
		}

		alts := resolveConceptAlternatives(metric)

		// Collect annual (10-K) data points within the year range across all alternatives.
		type yearData struct {
			Year  int     `json:"fiscal_year"`
			Value float64 `json:"value"`
			End   string  `json:"end_date"`
			Form  string  `json:"form"`
		}
		yearMap := make(map[int]*yearData)

		for _, alt := range alts {
			fd, ok := ns[alt]
			if !ok {
				continue
			}
			for _, points := range fd.Units {
				for _, p := range points {
					if p.Form != "10-K" {
						continue
					}
					if p.FY >= startYear && p.FY <= endYear {
						if existing, ok := yearMap[p.FY]; !ok || p.End > existing.End {
							yearMap[p.FY] = &yearData{
								Year:  p.FY,
								Value: p.Val,
								End:   p.End,
								Form:  p.Form,
							}
						}
					}
				}
			}
		}

		if len(yearMap) == 0 {
			return errorResult(fmt.Sprintf("Metric %q (and alternatives %v) not found in 10-K filings for the given year range", metric, alts)), nil
		}

		periodData := make([]yearData, 0, len(yearMap))
		for _, yd := range yearMap {
			periodData = append(periodData, *yd)
		}
		sort.Slice(periodData, func(i, j int) bool {
			return periodData[i].Year < periodData[j].Year
		})

		// Calculate analysis.
		analysis := map[string]any{}
		if len(periodData) >= 2 {
			startVal := periodData[0].Value
			endVal := periodData[len(periodData)-1].Value
			if startVal != 0 {
				totalGrowth := (endVal - startVal) / math.Abs(startVal) * 100
				analysis["total_growth_percent"] = math.Round(totalGrowth*100) / 100
				analysis["start_value"] = startVal
				analysis["end_value"] = endVal

				numYears := float64(periodData[len(periodData)-1].Year - periodData[0].Year)
				if numYears > 0 && startVal > 0 && endVal > 0 {
					cagr := (math.Pow(endVal/startVal, 1.0/numYears) - 1) * 100
					analysis["cagr_percent"] = math.Round(cagr*100) / 100
				}
			}
		}

		return jsonResult(map[string]any{
			"success":     true,
			"cik":         cik,
			"name":        facts.EntityName,
			"metric":      metric,
			"period_data": periodData,
			"analysis":    analysis,
		})
	}
}

func discoverCompanyMetrics(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		searchTerm := req.GetString("search_term", "")

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		facts, err := client.GetCompanyFacts(cik)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get company facts: %v", err)), nil
		}

		metrics := make([]map[string]any, 0)

		for namespace, concepts := range facts.Facts {
			for name, fd := range concepts {
				if searchTerm != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(searchTerm)) {
					continue
				}

				unitNames := make([]string, 0, len(fd.Units))
				totalPoints := 0
				for unit, points := range fd.Units {
					unitNames = append(unitNames, unit)
					totalPoints += len(points)
				}

				metrics = append(metrics, map[string]any{
					"namespace":    namespace,
					"concept":      name,
					"label":        fd.Label,
					"description":  fd.Description,
					"units":        unitNames,
					"data_points":  totalPoints,
				})
			}
		}

		return jsonResult(map[string]any{
			"success":           true,
			"cik":               cik,
			"name":              facts.EntityName,
			"available_metrics": metrics,
			"count":             len(metrics),
		})
	}
}

func getXBRLConcepts(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		conceptsStr := req.GetString("concepts", "")
		formType := req.GetString("form_type", "10-K")

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		facts, err := client.GetCompanyFacts(cik)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get company facts: %v", err)), nil
		}

		// Determine which concepts to extract.
		var concepts []string
		if conceptsStr != "" {
			for _, c := range strings.Split(conceptsStr, ",") {
				c = strings.TrimSpace(c)
				if c != "" {
					concepts = append(concepts, c)
				}
			}
		} else {
			// Default: all major financial concepts.
			concepts = append(concepts, incomeStatementConcepts...)
			concepts = append(concepts, balanceSheetConcepts...)
			concepts = append(concepts, cashFlowConcepts...)
		}

		results := make(map[string]any)
		for _, concept := range concepts {
			alts := resolveConceptAlternatives(concept)
			var best *edgar.FactDataPoint
			bestConcept := ""
			for _, alt := range alts {
				dp := getLatestByForm(facts, "us-gaap", alt, formType)
				if dp != nil && (best == nil || dp.End > best.End) {
					best = dp
					bestConcept = alt
				}
			}
			if best != nil {
				results[concept] = map[string]any{
					"value":         best.Val,
					"end_date":      best.End,
					"form":          best.Form,
					"fiscal_year":   best.FY,
					"fiscal_period": best.FP,
					"xbrl_concept":  bestConcept,
				}
			}
		}

		return jsonResult(map[string]any{
			"success":  true,
			"cik":      cik,
			"name":     facts.EntityName,
			"concepts": results,
			"filing_reference": map[string]any{
				"form_type":   formType,
				"data_source": "XBRL Company Facts API",
			},
		})
	}
}

func discoverXBRLConcepts(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		formType := req.GetString("form_type", "10-K")
		nsFilter := req.GetString("namespace_filter", "")
		offset := req.GetInt("offset", 0)
		limit := req.GetInt("limit", 50)
		if limit > 200 {
			limit = 200
		}
		if limit < 1 {
			limit = 50
		}
		if offset < 0 {
			offset = 0
		}

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		facts, err := client.GetCompanyFacts(cik)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get company facts: %v", err)), nil
		}

		// Collect all matching concepts first, then paginate.
		type conceptEntry struct {
			Namespace   string
			Name        string
			Label       string
			Description string
		}
		var allConcepts []conceptEntry

		for namespace, concepts := range facts.Facts {
			if nsFilter != "" && !strings.EqualFold(namespace, nsFilter) {
				continue
			}
			for name, fd := range concepts {
				hasForm := false
				for _, points := range fd.Units {
					for _, p := range points {
						if strings.EqualFold(p.Form, formType) {
							hasForm = true
							break
						}
					}
					if hasForm {
						break
					}
				}
				if !hasForm {
					continue
				}
				allConcepts = append(allConcepts, conceptEntry{
					Namespace:   namespace,
					Name:        name,
					Label:       fd.Label,
					Description: fd.Description,
				})
			}
		}

		// Sort for stable pagination order.
		sort.Slice(allConcepts, func(i, j int) bool {
			if allConcepts[i].Namespace != allConcepts[j].Namespace {
				return allConcepts[i].Namespace < allConcepts[j].Namespace
			}
			return allConcepts[i].Name < allConcepts[j].Name
		})

		total := len(allConcepts)

		// Apply pagination.
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		page := allConcepts[offset:end]

		statements := make(map[string][]map[string]any)
		for _, c := range page {
			statements[c.Namespace] = append(statements[c.Namespace], map[string]any{
				"concept":     c.Name,
				"label":       c.Label,
				"description": c.Description,
			})
		}

		return jsonResult(map[string]any{
			"success":              true,
			"cik":                  cik,
			"name":                 facts.EntityName,
			"financial_statements": statements,
			"total_facts":          total,
			"form_type":            formType,
			"pagination": map[string]any{
				"offset":   offset,
				"limit":    limit,
				"returned": len(page),
				"has_more": end < total,
			},
		})
	}
}

// getLatestByForm returns the latest data point for a concept filtered by form type.
func getLatestByForm(facts *edgar.CompanyFactsResponse, namespace, concept, formType string) *edgar.FactDataPoint {
	ns, ok := facts.Facts[namespace]
	if !ok {
		return nil
	}
	fd, ok := ns[concept]
	if !ok {
		return nil
	}
	var best *edgar.FactDataPoint
	for _, points := range fd.Units {
		for i := range points {
			p := &points[i]
			if !strings.EqualFold(p.Form, formType) {
				continue
			}
			if best == nil || p.End > best.End {
				best = p
			}
		}
	}
	return best
}
