package edgar

import (
	"fmt"
	"time"
)

// CompanyInfo holds basic company information from SEC EDGAR.
type CompanyInfo struct {
	CIK            string `json:"cik"`
	Name           string `json:"name"`
	Ticker         string `json:"ticker,omitempty"`
	SIC            string `json:"sic,omitempty"`
	SICDescription string `json:"sic_description,omitempty"`
	Exchange       string `json:"exchange,omitempty"`
	State          string `json:"state,omitempty"`
	FiscalYearEnd  string `json:"fiscal_year_end,omitempty"`
}

// FilingInfo represents a single SEC filing.
type FilingInfo struct {
	AccessionNumber    string `json:"accession_number"`
	FilingDate         string `json:"filing_date"`
	FormType           string `json:"form_type"`
	CompanyName        string `json:"company_name"`
	CIK                string `json:"cik"`
	FileNumber         string `json:"file_number,omitempty"`
	AcceptanceDatetime string `json:"acceptance_datetime,omitempty"`
	PeriodOfReport     string `json:"period_of_report,omitempty"`
	PrimaryDocument    string `json:"primary_document,omitempty"`
	SECURL             string `json:"sec_url,omitempty"`
}

// TransactionInfo represents an insider trading transaction.
type TransactionInfo struct {
	TransactionDate string  `json:"transaction_date"`
	SecurityTitle   string  `json:"security_title"`
	TransactionType string  `json:"transaction_type"`
	Shares          float64 `json:"shares"`
	PricePerShare   float64 `json:"price_per_share,omitempty"`
	TotalValue      float64 `json:"total_value,omitempty"`
	OwnershipType   string  `json:"ownership_type,omitempty"`
	OwnerName       string  `json:"owner_name,omitempty"`
	OwnerTitle      string  `json:"owner_title,omitempty"`
}

// TickerEntry represents a single entry from the SEC ticker mapping.
type TickerEntry struct {
	CIK      int    `json:"cik"`
	Name     string `json:"name"`
	Ticker   string `json:"ticker"`
	Exchange string `json:"exchange"`
}

// ---- SEC EDGAR JSON API response structures ----

// SubmissionsResponse is the shape of /submissions/CIK{cik}.json
type SubmissionsResponse struct {
	CIK                    string         `json:"cik"`
	EntityType             string         `json:"entityType"`
	SIC                    string         `json:"sic"`
	SICDescription         string         `json:"sicDescription"`
	Name                   string         `json:"name"`
	Tickers                []string       `json:"tickers"`
	Exchanges              []string       `json:"exchanges"`
	StateOfIncorporation   string         `json:"stateOfIncorporation"`
	FiscalYearEnd          string         `json:"fiscalYearEnd"`
	Filings                FilingsWrapper `json:"filings"`
}

type FilingsWrapper struct {
	Recent RecentFilings `json:"recent"`
	Files  []FilingsFile `json:"files"`
}

type RecentFilings struct {
	AccessionNumber    []string `json:"accessionNumber"`
	FilingDate         []string `json:"filingDate"`
	Form               []string `json:"form"`
	PrimaryDocument    []string `json:"primaryDocument"`
	FileNumber         []string `json:"fileNumber,omitempty"`
	AcceptanceDateTime []string `json:"acceptanceDatetime,omitempty"`
	ReportDate         []string `json:"reportDate,omitempty"`
}

type FilingsFile struct {
	Name        string `json:"name"`
	FilingCount int    `json:"filingCount"`
	FilingFrom  string `json:"filingFrom"`
	FilingTo    string `json:"filingTo"`
}

// CompanyFactsResponse is the shape of /api/xbrl/companyfacts/CIK{cik}.json
type CompanyFactsResponse struct {
	CIK        int                            `json:"cik"`
	EntityName string                         `json:"entityName"`
	Facts      map[string]map[string]FactData `json:"facts"` // e.g. facts["us-gaap"]["Assets"]
}

type FactData struct {
	Label       string                     `json:"label"`
	Description string                     `json:"description"`
	Units       map[string][]FactDataPoint `json:"units"` // e.g. units["USD"]
}

type FactDataPoint struct {
	End        string  `json:"end"`
	Val        float64 `json:"val"`
	AccN       string  `json:"accn"`
	FY         int     `json:"fy"`
	FP         string  `json:"fp"`
	Form       string  `json:"form"`
	Filed      string  `json:"filed"`
	Frame      string  `json:"frame,omitempty"`
	Start      string  `json:"start,omitempty"`
}

// FullTextSearchResponse is the shape of efts.sec.gov/LATEST/search-index
type FullTextSearchResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []SearchHit `json:"hits"`
	} `json:"hits"`
}

type SearchHit struct {
	ID     string         `json:"_id"`
	Source SearchHitSource `json:"_source"`
}

type SearchHitSource struct {
	CompanyName     string   `json:"entity_name"`
	CIK             string   `json:"entity_id"`
	FileNum         string   `json:"file_num"`
	FormType        string   `json:"form_type"`
	FiledAt         string   `json:"filed_at"`
	PeriodOfReport  string   `json:"period_of_report"`
	FileDescription string   `json:"file_description"`
}

// EFTSSearchResponse matches the EDGAR full-text search system response.
type EFTSSearchResponse struct {
	Query   string       `json:"q"`
	Total   EFTSTotal    `json:"total"`
	Filings []EFTSFiling `json:"filings"`
}

type EFTSTotal struct {
	Value    int    `json:"value"`
	Relation string `json:"relation"`
}

type EFTSFiling struct {
	AccessionNo string `json:"accessionNo"`
	CIK         string `json:"cik,omitempty"`
	EntityName  string `json:"entityName,omitempty"`
	FormType    string `json:"formType,omitempty"`
	FiledAt     string `json:"filedAt,omitempty"`
	FileDate    string `json:"fileDate"`
	PeriodOfReport string `json:"periodOfReport,omitempty"`
}

// Form4XML structures for parsing Form 4 XML content.
type Form4Document struct {
	Issuer          Form4Issuer          `json:"issuer"`
	ReportingOwner  []Form4Owner         `json:"reportingOwner"`
	Transactions    []Form4Transaction   `json:"transactions"`
	Holdings        []Form4Holding       `json:"holdings"`
	PeriodOfReport  string               `json:"periodOfReport"`
}

type Form4Issuer struct {
	CIK    string `json:"issuerCik"`
	Name   string `json:"issuerName"`
	Ticker string `json:"issuerTradingSymbol"`
}

type Form4Owner struct {
	CIK           string `json:"rptOwnerCik"`
	Name          string `json:"rptOwnerName"`
	IsDirector    bool   `json:"isDirector"`
	IsOfficer     bool   `json:"isOfficer"`
	IsTenPctOwner bool   `json:"isTenPercentOwner"`
	OfficerTitle  string `json:"officerTitle,omitempty"`
}

type Form4Transaction struct {
	SecurityTitle          string  `json:"securityTitle"`
	TransactionDate        string  `json:"transactionDate"`
	TransactionCode        string  `json:"transactionCode"`
	Shares                 float64 `json:"shares"`
	PricePerShare          float64 `json:"pricePerShare"`
	AcquiredDisposed       string  `json:"acquiredDisposedCode"`
	SharesOwnedAfter       float64 `json:"sharesOwnedFollowingTransaction"`
	DirectOrIndirect       string  `json:"directOrIndirectOwnership"`
}

type Form4Holding struct {
	SecurityTitle    string  `json:"securityTitle"`
	SharesOwned      float64 `json:"sharesOwned"`
	DirectOrIndirect string  `json:"directOrIndirectOwnership"`
}

// Helper to parse dates.
func ParseDate(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
		"01/02/2006",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse date: %s", s)
}
