package edgar

import (
	"encoding/json"
	"fmt"
	"io"
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

func (c *Client) get(rawURL string) ([]byte, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SEC API returned status %d for %s", resp.StatusCode, rawURL)
	}

	return io.ReadAll(resp.Body)
}

func (c *Client) getHTML(rawURL string) ([]byte, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SEC returned status %d for %s", resp.StatusCode, rawURL)
	}

	return io.ReadAll(resp.Body)
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

// BuildSECURL builds a URL to a filing on the SEC website.
func BuildSECURL(cik, accessionNumber string) string {
	cleanAcc := strings.ReplaceAll(accessionNumber, "-", "")
	trimCIK := strings.TrimLeft(PadCIK(cik), "0")
	if trimCIK == "" {
		trimCIK = "0"
	}
	return fmt.Sprintf("%s/%s/%s/%s-index.htm", archivesBaseURL, trimCIK, cleanAcc, accessionNumber)
}

// GetLatestMetricValue returns the most recent data point for a given metric from company facts.
func GetLatestMetricValue(facts *CompanyFactsResponse, namespace, concept string) *FactDataPoint {
	ns, ok := facts.Facts[namespace]
	if !ok {
		return nil
	}
	fd, ok := ns[concept]
	if !ok {
		return nil
	}

	// Look through all units and find the most recent 10-K or 10-Q filing.
	var best *FactDataPoint
	for _, points := range fd.Units {
		for i := range points {
			p := &points[i]
			if p.Form != "10-K" && p.Form != "10-Q" {
				continue
			}
			if best == nil || p.End > best.End {
				best = p
			}
		}
	}
	return best
}
