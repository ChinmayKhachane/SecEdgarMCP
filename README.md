# SEC EDGAR MCP Server

A Model Context Protocol (MCP) server that provides tools for querying SEC EDGAR financial data. Built in Go using [mcp-go](https://github.com/mark3labs/mcp-go).

## Features

- **Company Lookup** -- search companies, resolve tickers to CIK numbers, get company info
- **Financial Statements** -- extract income statements, balance sheets, and cash flow data from 10-K/10-Q filings
- **Key Metrics** -- quick access to revenue, net income, assets, EPS, and more
- **XBRL Data** -- extract and discover XBRL concepts with automatic tag resolution across accounting standard migrations (ASC 606, ASC 842, ASC 326)
- **Period Comparison** -- compare metrics across fiscal years with growth and CAGR calculations
- **Filing Access** -- retrieve recent filings, read filing content with pagination, extract specific 10-K/10-Q sections
- **8-K Analysis** -- detect material events and item codes from current reports
- **Insider Trading** -- track Form 3/4/5 filings, analyze transactions, and assess insider sentiment

## Setup

### Prerequisites

- Go 1.21+
- A SEC EDGAR user agent string (required by SEC fair access policy)

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `SEC_EDGAR_USER_AGENT` | Yes | Your identity for SEC EDGAR requests (format: `"Name (email@example.com)"`) |
| `SEC_EDGAR_LOG_FILE` | No | Path to log file (default: `sec-edgar-mcp.log`) |

### Running

```bash
SEC_EDGAR_USER_AGENT="Your Name (you@email.com)" go run .
```

### MCP Configuration

Add to your `.mcp.json` or Claude Code MCP settings:

```json
{
  "mcpServers": {
    "sec-edgar": {
      "command": "go",
      "args": ["run", "."],
      "cwd": "/path/to/secEdgarMCP",
      "env": {
        "SEC_EDGAR_USER_AGENT": "Your Name (you@email.com)"
      }
    }
  }
}
```

## Tools

### Company Tools

| Tool | Description |
|------|-------------|
| `get_cik_by_ticker` | Convert a stock ticker to SEC CIK number |
| `get_company_info` | Get company details (name, SIC code, exchange, state, fiscal year end) |
| `search_companies` | Search for companies by name |
| `get_company_facts` | Get key financial metrics from XBRL company facts |

### Financial Tools

| Tool | Description |
|------|-------------|
| `get_financials` | Extract income statement, balance sheet, and/or cash flow data |
| `get_key_metrics` | Get 8 key financial metrics (revenue, net income, assets, EPS, etc.) |
| `compare_periods` | Compare a metric across fiscal years with growth/CAGR calculations |
| `get_xbrl_concepts` | Extract specific XBRL concept values from filings |
| `discover_company_metrics` | Discover available XBRL metrics for a company |
| `discover_xbrl_concepts` | Browse all XBRL concepts in a company's filings (paginated) |

### Filing Tools

| Tool | Description |
|------|-------------|
| `get_recent_filings` | Get recent SEC filings, optionally filtered by form type |
| `get_filing_content` | Fetch filing text content with pagination |
| `get_filing_sections` | Extract specific sections from 10-K or 10-Q filings |
| `analyze_8k` | Analyze 8-K reports for material events and item codes |

### Insider Trading Tools

| Tool | Description |
|------|-------------|
| `get_insider_summary` | Summary of insider trading activity (filing counts, unique insiders) |
| `get_insider_transactions` | List insider trading filings (Forms 3, 4, 5) |
| `get_form4_details` | Detailed Form 4 data (owner info, transactions, holdings) |
| `analyze_form4_transactions` | Aggregate transaction analysis with share counts and prices |
| `analyze_insider_sentiment` | Buy/sell sentiment analysis over a configurable period |

### Utility Tools

| Tool | Description |
|------|-------------|
| `get_recommended_tools` | Get tool recommendations and tips for a given SEC form type |

## XBRL Concept Resolution

Companies change XBRL tags over time as accounting standards evolve. This server automatically resolves equivalent concepts so you don't have to worry about which tag a company uses.

For example, passing any of these revenue concepts will search across all of them and return the most recent data:

- `RevenueFromContractWithCustomerExcludingAssessedTax` (ASC 606, post-2018)
- `Revenues` (legacy)
- `SalesRevenueNet`
- `SalesRevenueGoodsNet`

This applies to all financial tools including `get_financials`, `get_key_metrics`, `compare_periods`, `get_xbrl_concepts`, and `get_company_facts`.

## Project Structure

```
.
├── main.go              # MCP server setup & utility tools
├── edgar/
│   ├── client.go        # SEC EDGAR API client & HTTP requests
│   ├── models.go        # Data structures
│   └── cache.go         # Ticker-to-CIK lookup caching
└── tools/
    ├── company.go       # Company info & facts tools
    ├── financial.go     # Financial metrics & XBRL tools
    ├── filings.go       # Filing retrieval tools
    └── insider.go       # Insider trading tools
```

## License

MIT
