#!/bin/bash
set -e

IMAGE_NAME="sec-edgar-mcp"
BINARY_NAME="sec-edgar-mcp"
USER_AGENT="${SEC_EDGAR_USER_AGENT:-}"

usage() {
    echo "Usage: ./run.sh [build|run|build-local|local]"
    echo ""
    echo "  build         Build the Docker image"
    echo "  run           Run the MCP server via Docker (stdio)"
    echo "  build-local   Build the Go binary locally (requires Go toolchain)"
    echo "  local         Build (if needed) and run the MCP server natively (stdio)"
    echo ""
    echo "Environment:"
    echo "  SEC_EDGAR_USER_AGENT  Required for 'run' and 'local'. Your SEC EDGAR identity (e.g. \"Name (email)\")"
}

require_user_agent() {
    if [ -z "$USER_AGENT" ]; then
        echo "Error: SEC_EDGAR_USER_AGENT is not set."
        echo "  export SEC_EDGAR_USER_AGENT=\"Your Name (you@email.com)\""
        exit 1
    fi
}

case "${1:-}" in
    build)
        echo "Building $IMAGE_NAME..."
        docker build --platform linux/amd64 -t "$IMAGE_NAME" .
        echo "Done. Image: $IMAGE_NAME"
        ;;
    run)
        require_user_agent
        docker run --rm -i \
            --platform linux/amd64 \
            -e "SEC_EDGAR_USER_AGENT=$USER_AGENT" \
            "$IMAGE_NAME"
        ;;
    build-local)
        if ! command -v go >/dev/null 2>&1; then
            echo "Error: 'go' not found in PATH. Install Go (>=1.25) to build locally."
            exit 1
        fi
        echo "Building $BINARY_NAME (native)..."
        go build -o "$BINARY_NAME" .
        echo "Done. Binary: ./$BINARY_NAME"
        ;;
    local)
        require_user_agent
        if [ ! -x "./$BINARY_NAME" ]; then
            if ! command -v go >/dev/null 2>&1; then
                echo "Error: ./$BINARY_NAME not found and 'go' not in PATH."
                echo "  Install Go (>=1.25) or run './run.sh build-local' first."
                exit 1
            fi
            echo "Building $BINARY_NAME (native)..."
            go build -o "$BINARY_NAME" .
        fi
        SEC_EDGAR_USER_AGENT="$USER_AGENT" exec "./$BINARY_NAME"
        ;;
    *)
        usage
        ;;
esac
