# SEC EDGAR MCP Server

An MCP server for querying SEC EDGAR financial data — company info, financial statements, XBRL metrics, filings, insider trading, and more. Built in Go with [mcp-go](https://github.com/mark3labs/mcp-go).

## Prerequisites

- Docker (recommended) **or** Go 1.26+

## Getting Started

### Option A: Docker 

#### 1. Clone the repo

```bash
git clone https://github.com/ChinmayKhachane/SecEdgarMCP.git
cd SecEdgarMCP
```

#### 2. Build the Docker image

```bash
./run.sh build
```

#### 3. Configure Claude Code

Add to your `.mcp.json`

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

> ```bash
> sudo usermod -aG docker $USER
> ```

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

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `SEC_EDGAR_USER_AGENT` | Yes | Your identity for SEC EDGAR (format: `"Name (email)"`) |
| `SEC_EDGAR_LOG_FILE` | No | Log file path (default: `sec-edgar-mcp.log`) |

## What It Can Do

- Look up companies by ticker or name
- Pull income statements, balance sheets, and cash flow data
- Compare financial metrics across periods
- Read filing content and extract specific 10-K/10-Q sections
- Analyze 8-K reports for material events
- Track insider trading activity and sentiment


## License

MIT
