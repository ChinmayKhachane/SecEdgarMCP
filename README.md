# SEC EDGAR MCP Server

An MCP server for querying SEC EDGAR financial data — company info, financial statements, XBRL metrics, filings, insider trading, and more. Built with Go.

## Prerequisites

- **Docker** (recommended, no local toolchain needed) **or** Go 1.21+
- For `view_filing`: native (non-Docker) server + `w3m` + ghostty or konsole terminal

## Getting Started

### Option A: Docker

```bash
git clone https://github.com/ChinmayKhachane/SecEdgarMCP.git
cd SecEdgarMCP
./run.sh build
```

Add to your `.mcp.json`:

```json
{
  "mcpServers": {
    "sec-edgar": {
      "command": "docker",
      "args": ["run", "--rm", "-i", "--platform", "linux/amd64",
               "-e", "SEC_EDGAR_USER_AGENT=Your Name (you@email.com)",
               "sec-edgar-mcp"]
    }
  }
}
```

> If you see a permissions error, run `sudo usermod -aG docker $USER` and re-login.

### Option B: Native Binary (required for `view_filing`)

```bash
git clone https://github.com/ChinmayKhachane/SecEdgarMCP.git
cd SecEdgarMCP
./run.sh build-local          # compiles ./sec-edgar-mcp
```

Add to your `.mcp.json`:

```json
{
  "mcpServers": {
    "sec-edgar-local": {
      "command": "/absolute/path/to/SecEdgarMCP/sec-edgar-mcp",
      "env": {
        "SEC_EDGAR_USER_AGENT": "Your Name (you@email.com)"
      }
    }
  }
}
```

The repo ships a `.mcp.json` you can edit in place — it already has both entries.

#### run.sh commands

| Command | Description |
|---------|-------------|
| `./run.sh build` | Build the Docker image |
| `./run.sh run` | Run the server via Docker (stdio) |
| `./run.sh build-local` | Compile the native Go binary |
| `./run.sh local` | Build (if needed) and run natively |

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `SEC_EDGAR_USER_AGENT` | Yes | Your identity for SEC EDGAR — `"Name (email)"` |
| `SEC_EDGAR_LOG_FILE` | No | Log file path (default: `sec-edgar-mcp.log`) |
| `SEC_EDGAR_TERMINAL` | No | Terminal emulator for `view_filing` (default: auto-detect ghostty/konsole) |

## Available Tools

### Company

| Tool | Description |
|------|-------------|
| `get_company_info` | Name, ticker, SIC, exchange, state, fiscal year end |
| `get_cik_by_ticker` | Resolve a ticker to its SEC CIK |
| `search_companies` | Search companies by name or ticker |
| `get_company_facts` | All XBRL facts reported by a company |

### Filings

| Tool | Description |
|------|-------------|
| `get_recent_filings` | Recent filings, optionally filtered by form type and date window |
| `get_latest_filing` | Single most recent filing of a given type (10-K, 10-Q, 8-K, …) |
| `get_filing_content` | Raw text of a filing with pagination |
| `get_filing_sections` | Extract named sections from 10-K/10-Q (MD&A, Risk Factors, etc.) |
| `analyze_8k` | Parse an 8-K for material event items |
| `view_filing` | Open a filing in a new terminal window via w3m (native server only) |

`view_filing` accepts either a direct `url` (primary-document URL from `get_recent_filings`) or a `form_type` to auto-resolve the latest filing. It strips inline XBRL and CSS before rendering.

### Financials

| Tool | Description |
|------|-------------|
| `get_financials` | Income statement, balance sheet, or cash flow data |
| `get_key_metrics` | Revenue, net income, EPS, and other key metrics |
| `compare_periods` | Year-over-year or quarter-over-quarter comparison |
| `get_quarterly_distribution` | Metric values broken down by quarter |
| `discover_company_metrics` | List all reported financial metrics for a company |
| `get_xbrl_concepts` | Fetch data for a specific XBRL concept |
| `discover_xbrl_concepts` | Search for XBRL concept names |

### Insider Trading

| Tool | Description |
|------|-------------|
| `get_insider_transactions` | Recent Form 4 transactions |
| `get_insider_summary` | Aggregated buy/sell activity by insider |
| `get_form4_details` | Full details of a specific Form 4 filing |
| `analyze_form4_transactions` | Parse and classify transactions in a Form 4 |
| `analyze_insider_sentiment` | Overall insider buying/selling sentiment |

## License

MIT
