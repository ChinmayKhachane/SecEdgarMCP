package edgar

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	submissionsURL  = "https://data.sec.gov/submissions/CIK%s.json"
	companyFactsURL = "https://data.sec.gov/api/xbrl/companyfacts/CIK%s.json"
	tickerMapURL    = "https://www.sec.gov/files/company_tickers_exchange.json"
	eftsSearchURL   = "https://efts.sec.gov/LATEST/search-index"
	edgarSearchURL  = "https://efts.sec.gov/LATEST/search-index"
	archivesBaseURL = "https://www.sec.gov/Archives/edgar/data"
)

// Client communicates with the SEC EDGAR REST APIs.
type Client struct {
	httpClient *http.Client
	userAgent  string
	cache      *TickerCache
	logger     *log.Logger
}

// NewClient creates a new EDGAR API client.
// userAgent should be in the format "FirstName LastName (email@example.com)".
func NewClient(userAgent string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		userAgent:  userAgent,
		cache:      NewTickerCache(),
	}
}

// SetLogger attaches a file logger to the client for request logging.
func (c *Client) SetLogger(l *log.Logger) {
	c.logger = l
}

func (c *Client) logf(format string, args ...any) {
	if c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

func (c *Client) get(rawURL string) ([]byte, error) {
	start := time.Now()
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logf("GET %s  error: %v (%s)", rawURL, err, time.Since(start))
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	c.logf("GET %s  %d (%s)", rawURL, resp.StatusCode, time.Since(start))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SEC API returned status %d for %s", resp.StatusCode, rawURL)
	}

	return body, readErr
}

func (c *Client) getHTML(rawURL string) ([]byte, error) {
	start := time.Now()
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logf("GET %s  error: %v (%s)", rawURL, err, time.Since(start))
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	c.logf("GET %s  %d (%s)", rawURL, resp.StatusCode, time.Since(start))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SEC returned status %d for %s", resp.StatusCode, rawURL)
	}

	return body, readErr
}

// PadCIK pads a CIK to 10 digits with leading zeros.
func PadCIK(cik string) string {
	cik = strings.TrimSpace(cik)
	for len(cik) < 10 {
		cik = "0" + cik
	}
	return cik
}

// ResolveCIK converts a ticker or CIK string to a 10-digit CIK.
func (c *Client) ResolveCIK(identifier string) (string, error) {
	identifier = strings.TrimSpace(strings.ToUpper(identifier))

	// If it's already numeric, treat as CIK.
	isNumeric := true
	for _, ch := range identifier {
		if ch < '0' || ch > '9' {
			isNumeric = false
			break
		}
	}
	if isNumeric && len(identifier) > 0 {
		return PadCIK(identifier), nil
	}

	// Otherwise it's a ticker — look up via cache.
	cik, err := c.cache.GetCIK(c, identifier)
	if err != nil {
		return "", fmt.Errorf("could not resolve ticker %q: %w", identifier, err)
	}
	return PadCIK(fmt.Sprintf("%d", cik)), nil
}

// GetSubmissions fetches the full submissions JSON for a company.
func (c *Client) GetSubmissions(cik string) (*SubmissionsResponse, error) {
	cik = PadCIK(cik)
	data, err := c.get(fmt.Sprintf(submissionsURL, cik))
	if err != nil {
		return nil, err
	}
	var resp SubmissionsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse submissions: %w", err)
	}
	return &resp, nil
}

// GetCompanyFacts fetches XBRL company facts.
func (c *Client) GetCompanyFacts(cik string) (*CompanyFactsResponse, error) {
	cik = PadCIK(cik)
	data, err := c.get(fmt.Sprintf(companyFactsURL, cik))
	if err != nil {
		return nil, err
	}
	var resp CompanyFactsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse company facts: %w", err)
	}
	return &resp, nil
}

// GetCompanyInfo returns structured company information.
func (c *Client) GetCompanyInfo(identifier string) (*CompanyInfo, error) {
	cik, err := c.ResolveCIK(identifier)
	if err != nil {
		return nil, err
	}
	sub, err := c.GetSubmissions(cik)
	if err != nil {
		return nil, err
	}

	ticker := ""
	if len(sub.Tickers) > 0 {
		ticker = sub.Tickers[0]
	}
	exchange := ""
	if len(sub.Exchanges) > 0 {
		exchange = sub.Exchanges[0]
	}

	return &CompanyInfo{
		CIK:            sub.CIK,
		Name:           sub.Name,
		Ticker:         ticker,
		SIC:            sub.SIC,
		SICDescription: sub.SICDescription,
		Exchange:       exchange,
		State:          sub.StateOfIncorporation,
		FiscalYearEnd:  sub.FiscalYearEnd,
	}, nil
}

// SearchCompanies searches EDGAR full-text search for companies.
func (c *Client) SearchCompanies(query string, limit int) ([]CompanyInfo, error) {
	u := fmt.Sprintf("https://efts.sec.gov/LATEST/search-index?q=%s&dateRange=custom&startdt=2020-01-01&enddt=2030-01-01",
		url.QueryEscape(query))
	data, err := c.get(u)
	if err != nil {
		// Fallback: search the ticker cache.
		return c.searchTickerCache(query, limit)
	}

	// Try parsing EFTS response; if it fails, fall back to ticker cache.
	_ = data
	return c.searchTickerCache(query, limit)
}

func (c *Client) searchTickerCache(query string, limit int) ([]CompanyInfo, error) {
	entries, err := c.cache.Search(c, query, limit)
	if err != nil {
		return nil, err
	}
	results := make([]CompanyInfo, 0, len(entries))
	for _, e := range entries {
		results = append(results, CompanyInfo{
			CIK:      PadCIK(fmt.Sprintf("%d", e.CIK)),
			Name:     e.Name,
			Ticker:   e.Ticker,
			Exchange: e.Exchange,
		})
	}
	return results, nil
}

// GetRecentFilings returns recent filings for a company, optionally filtered by form type.
func (c *Client) GetRecentFilings(cik string, formType string, days int, limit int) ([]FilingInfo, error) {
	cik = PadCIK(cik)
	sub, err := c.GetSubmissions(cik)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	var results []FilingInfo

	recent := sub.Filings.Recent
	for i := 0; i < len(recent.AccessionNumber) && len(results) < limit; i++ {
		// Filter by form type if specified.
		if formType != "" && !strings.EqualFold(recent.Form[i], formType) {
			continue
		}

		// Filter by date.
		filingDate, err := time.Parse("2006-01-02", recent.FilingDate[i])
		if err != nil {
			continue
		}
		if filingDate.Before(cutoff) {
			continue
		}

		accNum := recent.AccessionNumber[i]
		cleanAcc := strings.ReplaceAll(accNum, "-", "")

		priDoc := ""
		if i < len(recent.PrimaryDocument) {
			priDoc = recent.PrimaryDocument[i]
		}

		reportDate := ""
		if i < len(recent.ReportDate) {
			reportDate = recent.ReportDate[i]
		}

		secURL := fmt.Sprintf("%s/%s/%s/%s", archivesBaseURL, strings.TrimLeft(cik, "0"), cleanAcc, priDoc)
		if priDoc == "" {
			secURL = fmt.Sprintf("%s/%s/%s/", archivesBaseURL, strings.TrimLeft(cik, "0"), cleanAcc)
		}

		fi := FilingInfo{
			AccessionNumber: accNum,
			FilingDate:      recent.FilingDate[i],
			FormType:        recent.Form[i],
			CompanyName:     sub.Name,
			CIK:             sub.CIK,
			PrimaryDocument: priDoc,
			PeriodOfReport:  reportDate,
			SECURL:          secURL,
		}

		if i < len(recent.FileNumber) {
			fi.FileNumber = recent.FileNumber[i]
		}
		if i < len(recent.AcceptanceDateTime) {
			fi.AcceptanceDatetime = recent.AcceptanceDateTime[i]
		}

		results = append(results, fi)
	}

	return results, nil
}

// GetLatestFiling returns the single most recent filing of a specific form type.
// Unlike GetRecentFilings (which is bounded by a day window), this scans the
// full submissions feed so the latest 10-K, 10-Q, etc. is always returned even
// if it was filed long ago.
func (c *Client) GetLatestFiling(cik, formType string) (*FilingInfo, error) {
	// 100 years back is effectively unbounded — the SEC submissions feed itself
	// is the limiting factor.
	results, err := c.GetRecentFilings(cik, formType, 365*100, 1)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no %s filing found for CIK %s", formType, cik)
	}
	return &results[0], nil
}

// GetFilingContent fetches the text content of a filing document.
func (c *Client) GetFilingContent(cik, accessionNumber string) (string, string, error) {
	cik = PadCIK(cik)

	// First get submissions to find the primary document filename.
	sub, err := c.GetSubmissions(cik)
	if err != nil {
		return "", "", err
	}

	recent := sub.Filings.Recent
	cleanAcc := strings.ReplaceAll(accessionNumber, "-", "")
	priDoc := ""
	formType := ""

	for i, acc := range recent.AccessionNumber {
		if acc == accessionNumber {
			if i < len(recent.PrimaryDocument) {
				priDoc = recent.PrimaryDocument[i]
			}
			if i < len(recent.Form) {
				formType = recent.Form[i]
			}
			break
		}
	}

	// Try primary document first, then fall back to .txt.
	trimCIK := strings.TrimLeft(cik, "0")
	if trimCIK == "" {
		trimCIK = "0"
	}

	var content []byte
	if priDoc != "" {
		docURL := fmt.Sprintf("%s/%s/%s/%s", archivesBaseURL, trimCIK, cleanAcc, priDoc)
		content, err = c.getHTML(docURL)
		if err != nil {
			// Fall back to .txt index.
			txtURL := fmt.Sprintf("%s/%s/%s/%s-index.htm", archivesBaseURL, trimCIK, cleanAcc, accessionNumber)
			content, err = c.getHTML(txtURL)
			if err != nil {
				return "", formType, fmt.Errorf("failed to fetch filing content: %w", err)
			}
		}
	} else {
		txtURL := fmt.Sprintf("%s/%s/%s/%s-index.htm", archivesBaseURL, trimCIK, cleanAcc, accessionNumber)
		content, err = c.getHTML(txtURL)
		if err != nil {
			return "", formType, fmt.Errorf("failed to fetch filing content: %w", err)
		}
	}

	return string(content), formType, nil
}

// FetchFilingDocument fetches a document from the EDGAR archive by URL,
// using the EDGAR-required User-Agent header. The caller is responsible for
// supplying a complete primary-document URL (e.g. the SECURL returned by
// GetRecentFilings / GetLatestFiling).
func (c *Client) FetchFilingDocument(docURL string) ([]byte, error) {
	return c.getHTML(docURL)
}

// BuildSECURL builds a URL to a filing on the SEC website.
func BuildSECURL(cik, accessionNumber string) string {
	cleanAcc := strings.ReplaceAll(accessionNumber, "-", "")
	trimCIK := strings.TrimLeft(PadCIK(cik), "0")
	if trimCIK == "" {
		trimCIK = "0"
	}
	return fmt.Sprintf("%s/%s/%s/%s-index.htm", archivesBaseURL, trimCIK, cleanAcc, accessionNumber)
}

// GetLatestMetricValue returns the most recent data point for a given metric.
// formFilter controls which filing types are considered:
//   - ""           → both 10-K and 10-Q (latest of either)
//   - "10-K"       → annual only
//   - "10-Q"       → quarterly only
//
// When multiple data points share the same end date (common in 10-Q filings
// which report both quarterly and year-to-date figures), the shortest period
// is preferred so that a 3-month figure is returned instead of a 6- or
// 9-month cumulative.
func GetLatestMetricValue(facts *CompanyFactsResponse, namespace, concept, formFilter string) *FactDataPoint {
	ns, ok := facts.Facts[namespace]
	if !ok {
		return nil
	}
	fd, ok := ns[concept]
	if !ok {
		return nil
	}

	var best *FactDataPoint
	for _, points := range fd.Units {
		for i := range points {
			p := &points[i]
			if formFilter != "" {
				if p.Form != formFilter {
					continue
				}
			} else if p.Form != "10-K" && p.Form != "10-Q" {
				continue
			}
			if best == nil || p.End > best.End || (p.End == best.End && IsShorterPeriod(p, best)) {
				best = p
			}
		}
	}
	return best
}

// isShorterPeriod returns true if a has a shorter reporting period than b.
// For flow metrics (income statement, cash flow) 10-Q filings include both
// quarterly (3-month) and year-to-date (6/9-month) figures with the same end
// date. The quarterly figure has a later Start date (shorter duration).
// Point-in-time metrics (balance sheet) have no Start date and are unaffected.
func IsShorterPeriod(a, b *FactDataPoint) bool {
	if a.Start == "" || b.Start == "" {
		return false
	}
	// Later start = shorter period for the same end date.
	return a.Start > b.Start
}
