# SEC EDGAR MCP Server

An MCP server for querying SEC EDGAR financial data — company info, financial statements, XBRL metrics, filings, insider trading, and more. Built in Go with [mcp-go](https://github.com/mark3labs/mcp-go).

## Prerequisites

- Docker (recommended) **or** Go 1.26+
- A SEC EDGAR user agent string (required by [SEC's fair access policy](https://www.sec.gov/os/accessing-edgar-data))

## Getting Started

### Option A: Docker (Recommended)

#### 1. Clone the repo

```bash
git clone https://github.com/ChinmayKhachane/SecEdgarMCP.git
cd SecEdgarMCP
```

#### 2. Build the Docker image

```bash
./run.sh build
```

This builds a multi-stage Docker image — Go 1.26 compiles the binary, then it's copied into a slim Debian image with just the essentials.

#### 3. Configure Claude Code

Add to your `.mcp.json` (project-level) or `~/.claude/settings.json` (global):

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

#### 4. Restart Claude Code

Claude Code will now launch the MCP server inside Docker automatically.

> **Note:** If you get a Docker permission error, add your user to the `docker` group:
> ```bash
> sudo usermod -aG docker $USER
> ```
> Then log out and back in for it to take effect.

### Option B: Run Directly with Go

#### 1. Clone and build

```bash
git clone https://github.com/ChinmayKhachane/SecEdgarMCP.git
cd SecEdgarMCP
go build .
```

#### 2. Set your user agent

```bash
export SEC_EDGAR_USER_AGENT="Your Name (you@email.com)"
```

#### 3. Configure Claude Code

```json
{
  "mcpServers": {
    "sec-edgar": {
      "command": "go",
      "args": ["run", "."],
      "cwd": "/absolute/path/to/SecEdgarMCP",
      "env": {
        "SEC_EDGAR_USER_AGENT": "Your Name (you@email.com)"
      }
    }
  }
}
```

## Recreating This Environment Step by Step

1. **Install Docker** — Follow the [official Docker install guide](https://docs.docker.com/engine/install/) for your OS. Ensure your user is in the `docker` group (`sudo usermod -aG docker $USER`, then re-login).

2. **Install Claude Code** — Available as a CLI (`npm install -g @anthropic-ai/claude-code`), desktop app, or IDE extension (VS Code, JetBrains).

3. **Clone this repo** — `git clone https://github.com/ChinmayKhachane/SecEdgarMCP.git && cd SecEdgarMCP`

4. **Register for SEC EDGAR access** — SEC requires a user agent string identifying you. Format: `"Your Name (your@email.com)"`. No API key needed.

5. **Build the Docker image** — `./run.sh build`

6. **Add the MCP config** — Create a `.mcp.json` in your project directory (or add to `~/.claude/settings.json` for global access) with the Docker config shown above. Replace the user agent with your own.

7. **Start Claude Code** — Launch Claude Code. It will automatically start the MCP server in a Docker container when you use any SEC EDGAR tool.

8. **Verify** — Ask Claude to run `get_company_info` with any ticker (e.g. `AAPL`) to confirm the server is connected.

## Connecting to Other MCP Clients

Any MCP-compatible client can launch this server. The only requirements are:

1. Set the `SEC_EDGAR_USER_AGENT` environment variable
2. Launch via `docker run --rm -i` or `go run .` with stdio transport

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `SEC_EDGAR_USER_AGENT` | Yes | Your identity for SEC EDGAR (format: `"Name (email)"`) |
| `SEC_EDGAR_LOG_FILE` | No | Log file path (default: `sec-edgar-mcp.log`) |

## What It Can Do

- Look up companies by ticker or name
- Pull income statements, balance sheets, and cash flow data
- Compare financial metrics across periods — annual YoY, same-quarter YoY (e.g. Q4 2023 vs Q4 2024 vs Q4 2025), and sequential quarter comparisons — with period-over-period growth percentages and CAGR
- View quarterly metric distribution across fiscal years to see which quarters drive the most value, with each quarter's share of the annual total
- Automatic Q4 derivation when companies don't report standalone Q4 data (Q4 = annual total - Q1 - Q2 - Q3)
- Standalone quarter filtering — cumulative YTD values from 10-Q filings are automatically excluded using period duration checks (≤95 days)
- Extract and browse XBRL data with automatic tag resolution across accounting standard changes (e.g. `Revenues` and `RevenueFromContractWithCustomerExcludingAssessedTax` resolve to the same data)
- Read filing content and extract specific 10-K/10-Q sections
- Analyze 8-K reports for material events
- Track insider trading activity and sentiment

Once connected, ask your MCP client to use `get_recommended_tools` with a form type (e.g. `10-K`) to see which tools to use for a given task.

## XBRL Concept Resolution

Companies migrate between XBRL tags as accounting standards change (e.g. ASC 606 for revenue, ASC 842 for leases). This server automatically resolves equivalent concepts bidirectionally — requesting any tag in a group checks all alternatives and returns the most recent data. Currently handled groups:

- **Revenue**: `RevenueFromContractWithCustomerExcludingAssessedTax`, `Revenues`, `SalesRevenueNet`, `SalesRevenueGoodsNet`
- **Cost of Revenue**: `CostOfRevenue`, `CostOfGoodsAndServicesSold`
- **Leases**: `OperatingLeaseRightOfUseAsset`, `OperatingLeaseLiability`, `OperatingLeasesRentExpenseNet`
- **Credit Losses**: `AccountsReceivableAllowanceForCreditLossExcludingAccruedInterest`, `AllowanceForDoubtfulAccountsReceivableCurrent`
- **Long-Term Debt**: `LongTermDebt`, `LongTermDebtNoncurrent`
- **Cash Change**: `CashCashEquivalentsRestrictedCashAndRestrictedCashEquivalentsPeriodIncreaseDecreaseIncludingExchangeRateEffect`, `CashAndCashEquivalentsPeriodIncreaseDecrease`
- **Debt Repayment**: `RepaymentsOfDebtAndCapitalLeaseObligations`, `RepaymentsOfDebt`

## Project Structure

```
SecEdgarMCP/
├── main.go              # MCP server setup, registers all tool groups
├── edgar/
│   ├── client.go        # SEC EDGAR API client (HTTP requests, CIK resolution)
│   └── models.go        # Data structures (filings, facts, Form 4, etc.)
├── tools/
│   ├── financial.go     # Financial tools (statements, metrics, comparisons, distributions)
│   ├── company.go       # Company info and facts tools
│   ├── filings.go       # Filing content and section extraction
│   └── insider.go       # Insider trading tools (Form 4, sentiment)
├── Dockerfile           # Multi-stage build (Go 1.26 → slim Debian)
├── .dockerignore        # Excludes logs, git, non-essential files
├── run.sh               # Build and run helper script
├── go.mod               # Go module definition
└── .mcp.json            # MCP client config (Docker mode)
```

## License

MIT
