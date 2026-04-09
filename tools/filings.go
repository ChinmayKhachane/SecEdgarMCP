package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sec-edgar-mcp/edgar"
	"strings"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func RegisterFilingTools(s *server.MCPServer, client *edgar.Client) {
	s.AddTool(
		mcp.NewTool("get_recent_filings",
			mcp.WithDescription("Get recent SEC filings for a company, optionally filtered by form type (10-K, 10-Q, 8-K, etc.)"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("form_type", mcp.Description("Filter by form type (e.g., 10-K, 10-Q, 8-K, 4)")),
			mcp.WithNumber("days", mcp.Description("Number of days to look back (default: 30)")),
			mcp.WithNumber("limit", mcp.Description("Maximum results (default: 40)")),
		),
		WithTiming("get_recent_filings", getRecentFilings(client)),
	)

	s.AddTool(
		mcp.NewTool("get_filing_content",
			mcp.WithDescription("Fetch the text content of a specific SEC filing with pagination support"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("accession_number", mcp.Required(), mcp.Description("Filing accession number")),
			mcp.WithNumber("offset", mcp.Description("Character offset to start reading from (default: 0)")),
			mcp.WithNumber("max_chars", mcp.Description("Maximum characters to return (default: 50000)")),
		),
		WithTiming("get_filing_content", getFilingContent(client)),
	)

	s.AddTool(
		mcp.NewTool("analyze_8k",
			mcp.WithDescription("Analyze an 8-K current report for material events, extracting item codes and descriptions"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("accession_number", mcp.Required(), mcp.Description("Accession number of the 8-K filing")),
		),
		WithTiming("analyze_8k", analyze8K(client)),
	)

	s.AddTool(
		mcp.NewTool("get_filing_sections",
			mcp.WithDescription("Extract specific sections from 10-K or 10-Q filings (e.g., Risk Factors, MD&A, Business)"),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("accession_number", mcp.Required(), mcp.Description("Filing accession number")),
			mcp.WithString("form_type", mcp.Required(), mcp.Description("Form type: 10-K or 10-Q")),
		),
		WithTiming("get_filing_sections", getFilingSections(client)),
	)

	s.AddTool(
		mcp.NewTool("get_latest_filing",
			mcp.WithDescription("Find the single most recent filing of a specific form type (e.g., 10-K, 10-Q, 8-K, DEF 14A). Unlike get_recent_filings which is bounded by a day window, this scans all available history to guarantee returning the latest one of that type even if it was filed long ago."),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number")),
			mcp.WithString("form_type", mcp.Required(), mcp.Description("SEC form type (e.g., 10-K, 10-Q, 8-K, DEF 14A)")),
		),
		WithTiming("get_latest_filing", getLatestFiling(client)),
	)

	s.AddTool(
		mcp.NewTool("view_filing",
			mcp.WithDescription("Open an SEC filing in a NEW terminal window using w3m. Strips inline XBRL tags and CSS for readability. Provide either `url` (the primary-document URL, typically the sec_url from get_recent_filings) or `form_type` (to auto-resolve the latest filing of that type). Requires w3m and a supported terminal emulator (ghostty or konsole; override with SEC_EDGAR_TERMINAL). Only usable when the MCP server runs natively, not inside Docker."),
			mcp.WithString("identifier", mcp.Required(), mcp.Description("Stock ticker or CIK number. Used for logging context and for auto-resolving with form_type.")),
			mcp.WithString("url", mcp.Description("Full SEC archive URL to the primary document (e.g. https://www.sec.gov/Archives/edgar/data/1045810/000104581026000021/nvda-20260125.htm). If provided, it's fetched directly — no lookup.")),
			mcp.WithString("form_type", mcp.Description("Form type to auto-resolve (e.g. 10-K, 10-Q, 8-K, DEF 14A). Used only when url is not supplied; the latest filing of this type is opened.")),
		),
		WithTiming("view_filing", viewFiling(client)),
	)
}

func getRecentFilings(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		formType := req.GetString("form_type", "")
		days := req.GetInt("days", 30)
		limit := req.GetInt("limit", 40)

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		filings, err := client.GetRecentFilings(cik, formType, days, limit)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get filings: %v", err)), nil
		}

		return jsonResult(map[string]any{
			"success": true,
			"filings": filings,
			"count":   len(filings),
		})
	}
}

func getFilingContent(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		accessionNumber, _ := req.RequireString("accession_number")
		offset := req.GetInt("offset", 0)
		maxChars := req.GetInt("max_chars", 50000)

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		content, formType, err := client.GetFilingContent(cik, accessionNumber)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get filing content: %v", err)), nil
		}

		content = stripHTML(content)
		totalLen := len(content)

		if offset >= totalLen {
			return jsonResult(map[string]any{
				"success":          true,
				"accession_number": accessionNumber,
				"form_type":        formType,
				"content":          "",
				"pagination_data": map[string]any{
					"total_length": totalLen,
					"offset":       offset,
					"returned":     0,
					"has_more":     false,
				},
			})
		}

		end := offset + maxChars
		if end > totalLen {
			end = totalLen
		}

		chunk := content[offset:end]
		hasMore := end < totalLen

		result := map[string]any{
			"success":          true,
			"accession_number": accessionNumber,
			"form_type":        formType,
			"content":          chunk,
			"url":              edgar.BuildSECURL(cik, accessionNumber),
			"pagination_data": map[string]any{
				"total_length": totalLen,
				"offset":       offset,
				"returned":     len(chunk),
				"has_more":     hasMore,
			},
		}
		if hasMore {
			result["pagination_data"].(map[string]any)["next_offset"] = end
		}

		return jsonResult(result)
	}
}

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]*>`)
	multiSpaceRe = regexp.MustCompile(`\s{3,}`)
	multiNewline = regexp.MustCompile(`\n{3,}`)
)

func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&#8217;", "'")
	s = strings.ReplaceAll(s, "&#8220;", "\"")
	s = strings.ReplaceAll(s, "&#8221;", "\"")
	s = multiSpaceRe.ReplaceAllString(s, "  ")
	s = multiNewline.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

var eightKItems = map[string]string{
	"1.01": "Entry into a Material Definitive Agreement",
	"1.02": "Termination of a Material Definitive Agreement",
	"1.03": "Bankruptcy or Receivership",
	"1.04": "Mine Safety - Reporting of Shutdowns and Patterns of Violations",
	"2.01": "Completion of Acquisition or Disposition of Assets",
	"2.02": "Results of Operations and Financial Condition",
	"2.03": "Creation of a Direct Financial Obligation",
	"2.04": "Triggering Events That Accelerate or Increase a Direct Financial Obligation",
	"2.05": "Costs Associated with Exit or Disposal Activities",
	"2.06": "Material Impairments",
	"3.01": "Notice of Delisting or Failure to Satisfy a Continued Listing Rule",
	"3.02": "Unregistered Sales of Equity Securities",
	"3.03": "Material Modification to Rights of Security Holders",
	"4.01": "Changes in Registrant's Certifying Accountant",
	"4.02": "Non-Reliance on Previously Issued Financial Statements",
	"5.01": "Changes in Control of Registrant",
	"5.02": "Departure of Directors or Certain Officers; Election of Directors; Appointment of Certain Officers",
	"5.03": "Amendments to Articles of Incorporation or Bylaws",
	"5.04": "Temporary Suspension of Trading Under Registrant's Employee Benefit Plans",
	"5.05": "Amendment to Registrant's Code of Ethics",
	"5.06": "Change in Shell Company Status",
	"5.07": "Submission of Matters to a Vote of Security Holders",
	"5.08": "Shareholder Nominations Pursuant to Exchange Act Rule 14a-11",
	"7.01": "Regulation FD Disclosure",
	"8.01": "Other Events",
	"9.01": "Financial Statements and Exhibits",
}

func analyze8K(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		accessionNumber, _ := req.RequireString("accession_number")

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		content, _, err := client.GetFilingContent(cik, accessionNumber)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get 8-K content: %v", err)), nil
		}

		textContent := stripHTML(content)

		itemPattern := regexp.MustCompile(`(?i)item\s+(\d+\.\d+)`)
		matches := itemPattern.FindAllStringSubmatch(textContent, -1)

		foundItems := make([]map[string]string, 0)
		seen := make(map[string]bool)
		for _, m := range matches {
			code := m[1]
			if seen[code] {
				continue
			}
			seen[code] = true
			desc, ok := eightKItems[code]
			if !ok {
				desc = "Unknown Item"
			}
			foundItems = append(foundItems, map[string]string{
				"item_code":   code,
				"description": desc,
			})
		}

		return jsonResult(map[string]any{
			"success": true,
			"analysis": map[string]any{
				"accession_number": accessionNumber,
				"items":            foundItems,
				"item_count":       len(foundItems),
				"sec_url":          edgar.BuildSECURL(cik, accessionNumber),
			},
		})
	}
}

var tenKSections = []struct {
	pattern string
	name    string
}{
	{`(?i)item\s+1[.\s]*business`, "business"},
	{`(?i)item\s+1a[.\s]*risk\s+factors`, "risk_factors"},
	{`(?i)item\s+2[.\s]*properties`, "properties"},
	{`(?i)item\s+3[.\s]*legal\s+proceedings`, "legal_proceedings"},
	{`(?i)item\s+7[.\s]*management`, "mdna"},
	{`(?i)item\s+7a[.\s]*quantitative`, "market_risk"},
	{`(?i)item\s+8[.\s]*financial\s+statements`, "financial_statements"},
	{`(?i)item\s+9a[.\s]*controls`, "controls"},
}

func getFilingSections(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		accessionNumber, _ := req.RequireString("accession_number")
		formType, _ := req.RequireString("form_type")

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		content, _, err := client.GetFilingContent(cik, accessionNumber)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to get filing content: %v", err)), nil
		}

		textContent := stripHTML(content)

		sections := make(map[string]string)
		availableSections := make([]string, 0)

		for _, sec := range tenKSections {
			re := regexp.MustCompile(sec.pattern)
			loc := re.FindStringIndex(textContent)
			if loc != nil {
				availableSections = append(availableSections, sec.name)
				start := loc[0]
				end := start + 5000
				if end > len(textContent) {
					end = len(textContent)
				}
				sections[sec.name] = textContent[start:end]
			}
		}

		return jsonResult(map[string]any{
			"success":            true,
			"form_type":          formType,
			"sections":           sections,
			"available_sections": availableSections,
			"sec_url":            edgar.BuildSECURL(cik, accessionNumber),
		})
	}
}

// getLatestFiling returns the most recent filing of a given form type.
// Useful for grabbing the latest 10-K or 10-Q without worrying about how far
// back the company filed it.
func getLatestFiling(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		formType, _ := req.RequireString("form_type")

		cik, err := client.ResolveCIK(identifier)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
		}

		filing, err := client.GetLatestFiling(cik, formType)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to find latest %s: %v", formType, err)), nil
		}

		return jsonResult(map[string]any{
			"success": true,
			"filing":  filing,
		})
	}
}

// Regexes for stripping inline XBRL and styling so w3m renders the filing as
// readable text. Mirrors tests/test.go::cleanHTML.
var (
	ixHeaderRe   = regexp.MustCompile(`(?is)<ix:header[^>]*>.*?</ix:header>`)
	ixTagRe      = regexp.MustCompile(`(?i)</?ix:[a-z]+[^>]*>`)
	styleBlockRe = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	styleAttrRe  = regexp.MustCompile(`(?i)\s+style="[^"]*"`)
)

func cleanFilingHTML(s string) string {
	s = ixHeaderRe.ReplaceAllString(s, "")
	s = ixTagRe.ReplaceAllString(s, "")
	s = styleBlockRe.ReplaceAllString(s, "")
	s = styleAttrRe.ReplaceAllString(s, "")
	return s
}

// resolveTerminal picks the terminal emulator to spawn w3m in. Honors
// SEC_EDGAR_TERMINAL if set, otherwise auto-detects ghostty then konsole.
func resolveTerminal() (string, error) {
	if t := os.Getenv("SEC_EDGAR_TERMINAL"); t != "" {
		path, err := exec.LookPath(t)
		if err != nil {
			return "", fmt.Errorf("SEC_EDGAR_TERMINAL=%q not found in PATH", t)
		}
		return path, nil
	}
	for _, t := range []string{"ghostty", "konsole"} {
		if path, err := exec.LookPath(t); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no supported terminal found (tried ghostty, konsole); set SEC_EDGAR_TERMINAL to override")
}

// viewFiling fetches a filing's primary document, strips inline XBRL/CSS, and
// opens it in a brand-new terminal window running w3m. The temp file is
// removed when w3m exits via a bash trap.
func viewFiling(client *edgar.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		identifier, _ := req.RequireString("identifier")
		docURL := req.GetString("url", "")
		formTypeArg := req.GetString("form_type", "")

		if docURL == "" && formTypeArg == "" {
			return errorResult("must provide either url or form_type"), nil
		}

		if _, err := exec.LookPath("w3m"); err != nil {
			return errorResult("w3m not found in PATH; install w3m to use view_filing"), nil
		}
		terminal, err := resolveTerminal()
		if err != nil {
			return errorResult(err.Error()), nil
		}

		formType := ""
		accessionNumber := ""

		// form_type shortcut: resolve the latest filing of that type and take
		// its primary-document URL straight from the FilingInfo. No extra
		// lookups beyond the submissions fetch GetLatestFiling already does.
		if docURL == "" {
			cik, err := client.ResolveCIK(identifier)
			if err != nil {
				return errorResult(fmt.Sprintf("Failed to resolve identifier: %v", err)), nil
			}
			latest, err := client.GetLatestFiling(cik, formTypeArg)
			if err != nil {
				return errorResult(fmt.Sprintf("Failed to find latest %s: %v", formTypeArg, err)), nil
			}
			docURL = latest.SECURL
			formType = latest.FormType
			accessionNumber = latest.AccessionNumber
		}

		rawHTML, err := client.FetchFilingDocument(docURL)
		if err != nil {
			return errorResult(fmt.Sprintf("Failed to fetch filing: %v", err)), nil
		}

		cleaned := cleanFilingHTML(string(rawHTML))

		tmp, err := os.CreateTemp("", "sec-edgar-*.html")
		if err != nil {
			return errorResult(fmt.Sprintf("failed to create temp file: %v", err)), nil
		}
		if _, err := tmp.WriteString(cleaned); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return errorResult(fmt.Sprintf("failed to write temp file: %v", err)), nil
		}
		tmp.Close()
		tmpPath := tmp.Name()

		// Wrap w3m in bash so the temp file is cleaned up on any exit (normal,
		// Ctrl-C, window close → SIGHUP).
		shellCmd := fmt.Sprintf(`trap 'rm -f %q' EXIT HUP INT TERM; w3m -T text/html %q`, tmpPath, tmpPath)
		cmd := exec.Command(terminal, "-e", "bash", "-c", shellCmd)
		// Detach from the MCP server's session so the window survives if the
		// MCP client disconnects.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			os.Remove(tmpPath)
			return errorResult(fmt.Sprintf("failed to launch %s: %v", terminal, err)), nil
		}
		// Reap the child in the background so we don't leak a zombie.
		go func() { _ = cmd.Wait() }()

		return jsonResult(map[string]any{
			"success":          true,
			"accession_number": accessionNumber,
			"form_type":        formType,
			"terminal":         filepath.Base(terminal),
			"temp_file":        tmpPath,
			"url":              docURL,
			"message":          fmt.Sprintf("Opened filing in new %s window", filepath.Base(terminal)),
		})
	}
}
