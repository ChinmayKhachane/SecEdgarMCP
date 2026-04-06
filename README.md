# SEC EDGAR MCP Server

An MCP server for querying SEC EDGAR financial data — company info, financial statements, XBRL metrics, filings, insider trading, and more. Built in Go with [mcp-go](https://github.com/mark3labs/mcp-go).

## Prerequisites

- Go 1.25+
- A SEC EDGAR user agent string (required by [SEC's fair access policy](https://www.sec.gov/os/accessing-edgar-data))

## Getting Started

### 1. Clone and build

```bash
git clone https://github.com/ChinmayKhachane/SecEdgarMCP.git
cd SecEdgarMCP
go build .
```

### 2. Set your user agent

SEC EDGAR requires a user agent identifying who you are. Set it as an environment variable:

```bash
export SEC_EDGAR_USER_AGENT="Your Name (you@email.com)"
```

### 3. Run the server

```bash
go run .
```

The server communicates over stdio using the MCP protocol. It's designed to be launched by an MCP client, not run standalone in a terminal.

## Connecting to Claude Code

Add to your `.mcp.json` (project-level) or `~/.claude/settings.json` (global):

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

Alternatively, if you've already built the binary:

```json
{
  "mcpServers": {
    "sec-edgar": {
      "command": "/absolute/path/to/SecEdgarMCP/sec-edgar-mcp",
      "env": {
        "SEC_EDGAR_USER_AGENT": "Your Name (you@email.com)"
      }
    }
  }
}
```

## Connecting to Other MCP Clients

Any MCP-compatible client can launch this server. The only requirements are:

1. Set the `SEC_EDGAR_USER_AGENT` environment variable
2. Launch the binary or `go run .` with stdio transport

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `SEC_EDGAR_USER_AGENT` | Yes | Your identity for SEC EDGAR (format: `"Name (email)"`) |
| `SEC_EDGAR_LOG_FILE` | No | Log file path (default: `sec-edgar-mcp.log`) |

## What It Can Do

- Look up companies by ticker or name
- Pull income statements, balance sheets, and cash flow data
- Compare financial metrics across fiscal years with growth/CAGR
- Extract and browse XBRL data with automatic tag resolution across accounting standard changes (e.g. `Revenues` and `RevenueFromContractWithCustomerExcludingAssessedTax` resolve to the same data)
- Read filing content and extract specific 10-K/10-Q sections
- Analyze 8-K reports for material events
- Track insider trading activity and sentiment

Once connected, ask your MCP client to use `get_recommended_tools` with a form type (e.g. `10-K`) to see which tools to use for a given task.

## License

MIT
