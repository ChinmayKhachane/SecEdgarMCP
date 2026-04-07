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
	// Cash change — old vs new tag
	{"CashCashEquivalentsRestrictedCashAndRestrictedCashEquivalentsPeriodIncreaseDecreaseIncludingExchangeRateEffect", "CashAndCashEquivalentsPeriodIncreaseDecrease"},
	// Debt repayment variants
	{"RepaymentsOfDebtAndCapitalLeaseObligations", "RepaymentsOfDebt"},
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
	"CashCashEquivalentsRestrictedCashAndRestrictedCashEquivalentsPeriodIncreaseDecreaseIncludingExchangeRateEffect",
	"DepreciationDepletionAndAmortization",
	"PaymentsToAcquirePropertyPlantAndEquipment",
	"PaymentsOfDividends", "ProceedsFromIssuanceOfDebt", "RepaymentsOfDebtAndCapitalLeaseObligations",
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
			mcp.WithString("period", mcp.Description("Period type: annual (10-K only), quarterly (10-Q only), or latest (most recent of either, default). Applies to income statement and cash flow only; balance sheet always returns the latest snapshot.")),
		),
		WithTiming("get_financials", getFinancials(client)),
	)

	s.AddTool(
		mcp.NewTool("get_key_metrics",
			mcp.WithDescription("Get key financial metrics (Revenue, Net Income, Assets, EPS, etc.) from XBRL company facts"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("period", mcp.Description("Period type: annual (10-K only), quarterly (10-Q only), or latest (most recent of either, default)")),
		),
		WithTiming("get_key_metrics", getKeyMetrics(client)),
	)

	s.AddTool(
		mcp.NewTool("compare_periods",
			mcp.WithDescription("Compare a financial metric across periods with growth analysis. Supports annual YoY, same-quarter YoY, and sequential quarter comparisons."),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("metric", mcp.Required(), mcp.Description("XBRL concept name (e.g., Revenues, NetIncomeLoss, Assets)")),
			mcp.WithNumber("start_year", mcp.Required(), mcp.Description("Start fiscal year")),
			mcp.WithNumber("end_year", mcp.Required(), mcp.Description("End fiscal year")),
			mcp.WithString("period", mcp.Description("Comparison mode: 'annual' (default) for YoY 10-K, 'quarterly' for same-quarter YoY across years, 'sequential' for consecutive quarters within year range")),
			mcp.WithString("quarter", mcp.Description("Quarter to compare when period=quarterly (Q1, Q2, Q3, Q4). Required for quarterly mode.")),
		),
		WithTiming("compare_periods", comparePeriods(client)),
	)

	s.AddTool(
		mcp.NewTool("get_quarterly_distribution",
			mcp.WithDescription("Show how income statement metrics distribute across quarters for one or more fiscal years. Shows each quarter's value and share of the annual total. Defaults to all income statement metrics if none specified."),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("years", mcp.Required(), mcp.Description("Comma-separated fiscal years (e.g. '2023,2024,2025')")),
			mcp.WithString("metrics", mcp.Description("Comma-separated XBRL concept names (e.g. 'Revenues,NetIncomeLoss'). Defaults to all income statement metrics.")),
		),
		WithTiming("get_quarterly_distribution", getQuarterlyDistribution(client)),
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
		period := req.GetString("period", "latest")

		var formFilter string
		switch period {
		case "annual":
			formFilter = "10-K"
		case "quarterly":
			formFilter = "10-Q"
		default:
			formFilter = ""
			period = "latest"
		}

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
			statements["income_statement"] = extractConceptValues(facts, incomeStatementConcepts, formFilter)
		}
		if stmtType == "all" || stmtType == "balance" {
			// Balance sheet is a point-in-time snapshot; always return the latest.
			statements["balance_sheet"] = extractConceptValues(facts, balanceSheetConcepts, "")
		}
		if stmtType == "all" || stmtType == "cash" {
			statements["cash_flow"] = extractConceptValues(facts, cashFlowConcepts, formFilter)
		}

		return jsonResult(map[string]any{
			"success":    true,
			"cik":        cik,
			"name":       facts.EntityName,
			"period":     period,
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
// formFilter is passed to GetLatestMetricValue ("", "10-K", or "10-Q").
func bestAlternative(facts *edgar.CompanyFactsResponse, namespace string, alternatives []string, formFilter string) (*edgar.FactDataPoint, string) {
	var best *edgar.FactDataPoint
	bestConcept := ""
	for _, alt := range alternatives {
		dp := edgar.GetLatestMetricValue(facts, namespace, alt, formFilter)
		if dp != nil && (best == nil || dp.End > best.End) {
			best = dp
			bestConcept = alt
		}
	}
	return best, bestConcept
}

func extractConceptValues(facts *edgar.CompanyFactsResponse, concepts []string, formFilter string) map[string]any {
	result := make(map[string]any)
	for _, concept := range concepts {
		alts := resolveConceptAlternatives(concept)
		dp, matched := bestAlternative(facts, "us-gaap", alts, formFilter)
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
		period := req.GetString("period", "latest")

		var formFilter string
		switch period {
		case "annual":
			formFilter = "10-K"
		case "quarterly":
			formFilter = "10-Q"
		default:
			formFilter = ""
			period = "latest"
		}

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
			best, bestConcept := bestAlternative(facts, "us-gaap", alts, formFilter)
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
			"period":            period,
			"metrics":           metrics,
			"requested_metrics": defaultKeyMetrics,
			"found_metrics":     found,
		})
	}
}

// periodDataPoint represents a single data point in a period comparison.
type periodDataPoint struct {
	FiscalYear   int     `json:"fiscal_year"`
	FiscalPeriod string  `json:"fiscal_period"`
	Value        float64 `json:"value"`
	EndDate      string  `json:"end_date"`
	Form         string  `json:"form"`
}

// periodKey uniquely identifies a period for deduplication.
type periodKey struct {
	Year   int
	Period string
}

// collectMetricData gathers XBRL data points for a metric (with concept resolution)
// filtered by form type, year range, and optional fiscal period.
func collectMetricData(ns map[string]edgar.FactData, metric string, formTypes []string, startYear, endYear int, fiscalPeriod string, quarterlyOnly ...bool) []periodDataPoint {
	filterQuarterly := len(quarterlyOnly) > 0 && quarterlyOnly[0]

	alts := resolveConceptAlternatives(metric)
	formSet := make(map[string]bool, len(formTypes))
	for _, f := range formTypes {
		formSet[f] = true
	}

	best := make(map[periodKey]*periodDataPoint)

	for _, alt := range alts {
		fd, ok := ns[alt]
		if !ok {
			continue
		}
		for _, points := range fd.Units {
			for _, p := range points {
				if !formSet[p.Form] {
					continue
				}
				if p.FY < startYear || p.FY > endYear {
					continue
				}
				if fiscalPeriod != "" && p.FP != fiscalPeriod {
					continue
				}
				if filterQuarterly && p.Start != "" && p.End != "" {
					start, err1 := edgar.ParseDate(p.Start)
					end, err2 := edgar.ParseDate(p.End)
					if err1 == nil && err2 == nil && int(end.Sub(start).Hours()/24) > 95 {
						continue
					}
				}
				key := periodKey{Year: p.FY, Period: p.FP}
				if existing, ok := best[key]; !ok || p.End > existing.EndDate {
					best[key] = &periodDataPoint{
						FiscalYear:   p.FY,
						FiscalPeriod: p.FP,
						Value:        p.Val,
						EndDate:      p.End,
						Form:         p.Form,
					}
				}
			}
		}
	}

	result := make([]periodDataPoint, 0, len(best))
	for _, dp := range best {
		result = append(result, *dp)
	}
	return result
}

// sortPeriodData sorts data points chronologically (by year, then quarter).
func sortPeriodData(data []periodDataPoint) {
	quarterOrder := map[string]int{"Q1": 1, "Q2": 2, "Q3": 3, "Q4": 4, "FY": 5}
	sort.Slice(data, func(i, j int) bool {
		if data[i].FiscalYear != data[j].FiscalYear {
			return data[i].FiscalYear < data[j].FiscalYear
		}
		return quarterOrder[data[i].FiscalPeriod] < quarterOrder[data[j].FiscalPeriod]
	})
}

// computeGrowthAnalysis calculates period-over-period growth percentages,
// total growth, and CAGR (for annual data) from sorted period data.
func computeGrowthAnalysis(data []periodDataPoint) map[string]any {
	analysis := map[string]any{}
	if len(data) < 2 {
		return analysis
	}

	// Period-over-period growth for each consecutive pair.
	periodGrowth := make([]map[string]any, 0, len(data)-1)
	for i := 1; i < len(data); i++ {
		prev := data[i-1]
		curr := data[i]
		entry := map[string]any{
			"from_period": fmt.Sprintf("%s %d", prev.FiscalPeriod, prev.FiscalYear),
			"to_period":   fmt.Sprintf("%s %d", curr.FiscalPeriod, curr.FiscalYear),
			"from_value":  prev.Value,
			"to_value":    curr.Value,
		}
		if prev.Value != 0 {
			pct := (curr.Value - prev.Value) / math.Abs(prev.Value) * 100
			entry["change_percent"] = math.Round(pct*100) / 100
			entry["change_absolute"] = curr.Value - prev.Value
		}
		periodGrowth = append(periodGrowth, entry)
	}
	analysis["period_growth"] = periodGrowth

	// Overall growth from first to last.
	startVal := data[0].Value
	endVal := data[len(data)-1].Value
	if startVal != 0 {
		totalGrowth := (endVal - startVal) / math.Abs(startVal) * 100
		analysis["total_growth_percent"] = math.Round(totalGrowth*100) / 100
	}
	analysis["start_value"] = startVal
	analysis["end_value"] = endVal

	// CAGR only makes sense for annual or same-quarter-across-years data.
	numYears := float64(data[len(data)-1].FiscalYear - data[0].FiscalYear)
	if numYears > 0 && startVal > 0 && endVal > 0 {
		cagr := (math.Pow(endVal/startVal, 1.0/numYears) - 1) * 100
		analysis["cagr_percent"] = math.Round(cagr*100) / 100
	}

	return analysis
}

// deriveQuarterlyData collects standalone quarterly data (≤95 days) and derives
// Q4 values from annual totals when not reported standalone (Q4 = FY - Q1 - Q2 - Q3).
// If filterQuarter is set (e.g. "Q4"), only that quarter is returned.
// If filterQuarter is empty, all quarters are returned.
func deriveQuarterlyData(ns map[string]edgar.FactData, metric string, startYear, endYear int, filterQuarter string) []periodDataPoint {
	// Get standalone quarterly data.
	qData := collectMetricData(ns, metric, []string{"10-Q", "10-K"}, startYear, endYear, "", true)
	// Get annual totals.
	fyData := collectMetricData(ns, metric, []string{"10-K"}, startYear, endYear, "FY")

	// Index quarterly data by year → quarter.
	byYear := make(map[int]map[string]*periodDataPoint)
	for i := range qData {
		dp := &qData[i]
		if dp.FiscalPeriod != "Q1" && dp.FiscalPeriod != "Q2" && dp.FiscalPeriod != "Q3" && dp.FiscalPeriod != "Q4" {
			continue
		}
		if byYear[dp.FiscalYear] == nil {
			byYear[dp.FiscalYear] = make(map[string]*periodDataPoint)
		}
		byYear[dp.FiscalYear][dp.FiscalPeriod] = dp
	}

	// Derive Q4 where missing.
	for _, fy := range fyData {
		qMap := byYear[fy.FiscalYear]
		if qMap == nil {
			continue
		}
		if _, hasQ4 := qMap["Q4"]; hasQ4 {
			continue
		}
		q1, hasQ1 := qMap["Q1"]
		q2, hasQ2 := qMap["Q2"]
		q3, hasQ3 := qMap["Q3"]
		if hasQ1 && hasQ2 && hasQ3 {
			qMap["Q4"] = &periodDataPoint{
				FiscalYear:   fy.FiscalYear,
				FiscalPeriod: "Q4",
				Value:        fy.Value - q1.Value - q2.Value - q3.Value,
				EndDate:      "derived",
				Form:         "derived",
			}
		}
	}

	// Collect results.
	var result []periodDataPoint
	for _, qMap := range byYear {
		for _, dp := range qMap {
			if filterQuarter != "" && dp.FiscalPeriod != filterQuarter {
				continue
			}
			result = append(result, *dp)
		}
	}
	return result
}

func comparePeriods(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		metric, _ := req.RequireString("metric")
		startYear := req.GetInt("start_year", 2020)
		endYear := req.GetInt("end_year", 2024)
		period := req.GetString("period", "annual")
		quarter := strings.ToUpper(req.GetString("quarter", ""))

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		facts, err := client.GetCompanyFacts(cik)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get company facts: %v", err)), nil
		}

		ns, ok := facts.Facts["us-gaap"]
		if !ok {
			return errorResult("No us-gaap data available"), nil
		}

		var data []periodDataPoint
		var mode string

		switch period {
		case "quarterly":
			// Same quarter compared YoY (e.g. Q4 2022 vs Q4 2023 vs Q4 2024).
			if quarter == "" {
				return errorResult("'quarter' parameter is required for quarterly mode (Q1, Q2, Q3, or Q4)"), nil
			}
			if quarter != "Q1" && quarter != "Q2" && quarter != "Q3" && quarter != "Q4" {
				return errorResult(fmt.Sprintf("Invalid quarter %q — must be Q1, Q2, Q3, or Q4", quarter)), nil
			}

			if quarter == "Q4" {
				// Q4 is often not reported standalone — derive from annual - (Q1+Q2+Q3).
				data = deriveQuarterlyData(ns, metric, startYear, endYear, "Q4")
			} else {
				data = collectMetricData(ns, metric, []string{"10-Q", "10-K"}, startYear, endYear, quarter, true)
			}
			mode = fmt.Sprintf("quarterly_yoy_%s", quarter)

		case "sequential":
			// Consecutive quarters within the year range (Q1→Q2→Q3→Q4→Q1...).
			data = deriveQuarterlyData(ns, metric, startYear, endYear, "")
			mode = "sequential_quarters"

		default: // "annual"
			data = collectMetricData(ns, metric, []string{"10-K"}, startYear, endYear, "FY")
			mode = "annual_yoy"
		}

		if len(data) == 0 {
			alts := resolveConceptAlternatives(metric)
			return errorResult(fmt.Sprintf("Metric %q (and alternatives %v) not found for the given parameters", metric, alts)), nil
		}

		sortPeriodData(data)
		analysis := computeGrowthAnalysis(data)

		return jsonResult(map[string]any{
			"success":     true,
			"cik":         cik,
			"name":        facts.EntityName,
			"metric":      metric,
			"mode":        mode,
			"period_data": data,
			"analysis":    analysis,
		})
	}
}

func getQuarterlyDistribution(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		yearsStr, _ := req.RequireString("years")
		metricsStr := req.GetString("metrics", "")

		// Parse years.
		var years []int
		for _, s := range strings.Split(yearsStr, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			var y int
			if _, err := fmt.Sscanf(s, "%d", &y); err == nil {
				years = append(years, y)
			}
		}
		if len(years) == 0 {
			return errorResult("No valid years provided. Use comma-separated years like '2023,2024,2025'"), nil
		}
		sort.Ints(years)

		// Parse metrics — default to revenue.
		var metrics []string
		if metricsStr != "" {
			for _, s := range strings.Split(metricsStr, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					metrics = append(metrics, s)
				}
			}
		} else {
			metrics = []string{"RevenueFromContractWithCustomerExcludingAssessedTax"}
		}

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		facts, err := client.GetCompanyFacts(cik)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get company facts: %v", err)), nil
		}

		ns, ok := facts.Facts["us-gaap"]
		if !ok {
			return errorResult("No us-gaap data available"), nil
		}

		yearSet := make(map[int]bool, len(years))
		for _, y := range years {
			yearSet[y] = true
		}
		minYear, maxYear := years[0], years[len(years)-1]

		type quarterEntry struct {
			Quarter     string  `json:"quarter"`
			Value       float64 `json:"value"`
			EndDate     string  `json:"end_date"`
			ShareOfYear float64 `json:"share_of_year_percent"`
		}
		type yearBreakdown struct {
			FiscalYear  int            `json:"fiscal_year"`
			AnnualTotal float64        `json:"annual_total"`
			Quarters    []quarterEntry `json:"quarters"`
		}
		type metricDistribution struct {
			Metric string          `json:"metric"`
			Years  []yearBreakdown `json:"years"`
		}

		results := make([]metricDistribution, 0, len(metrics))

		for _, metric := range metrics {
			data := deriveQuarterlyData(ns, metric, minYear, maxYear, "")
			annualData := collectMetricData(ns, metric, []string{"10-K"}, minYear, maxYear, "FY")

			annualByYear := make(map[int]float64)
			for _, dp := range annualData {
				annualByYear[dp.FiscalYear] = dp.Value
			}

			// Organize by year → quarter.
			yearQuarters := make(map[int]map[string]*periodDataPoint)
			for i := range data {
				dp := &data[i]
				if !yearSet[dp.FiscalYear] {
					continue
				}
				if yearQuarters[dp.FiscalYear] == nil {
					yearQuarters[dp.FiscalYear] = make(map[string]*periodDataPoint)
				}
				yearQuarters[dp.FiscalYear][dp.FiscalPeriod] = dp
			}

			if len(yearQuarters) == 0 {
				continue
			}

			yearBreakdowns := make([]yearBreakdown, 0, len(years))
			for _, y := range years {
				qMap := yearQuarters[y]
				if qMap == nil || len(qMap) == 0 {
					continue
				}

				// Use annual total if available, otherwise sum quarters.
				annualTotal, hasAnnual := annualByYear[y]
				if !hasAnnual {
					for _, dp := range qMap {
						annualTotal += dp.Value
					}
				}

				quarters := make([]quarterEntry, 0, 4)
				for _, q := range []string{"Q1", "Q2", "Q3", "Q4"} {
					dp, ok := qMap[q]
					if !ok {
						continue
					}
					share := 0.0
					if annualTotal > 0 {
						share = math.Round(dp.Value/annualTotal*10000) / 100
					}
					quarters = append(quarters, quarterEntry{
						Quarter:     q,
						Value:       dp.Value,
						EndDate:     dp.EndDate,
						ShareOfYear: share,
					})
				}

				yearBreakdowns = append(yearBreakdowns, yearBreakdown{
					FiscalYear:  y,
					AnnualTotal: annualTotal,
					Quarters:    quarters,
				})
			}

			results = append(results, metricDistribution{
				Metric: metric,
				Years:  yearBreakdowns,
			})
		}

		if len(results) == 0 {
			return errorResult("No quarterly data found for the requested metrics and years"), nil
		}

		return jsonResult(map[string]any{
			"success":       true,
			"cik":           cik,
			"name":          facts.EntityName,
			"distributions": results,
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
// When multiple points share the same end date, the shortest period is preferred
// (quarterly over year-to-date for 10-Q filings).
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
			if best == nil || p.End > best.End || (p.End == best.End && edgar.IsShorterPeriod(p, best)) {
				best = p
			}
		}
	}
	return best
}
