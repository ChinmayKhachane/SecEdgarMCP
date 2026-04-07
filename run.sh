#!/bin/bash
set -e

IMAGE_NAME="sec-edgar-mcp"
USER_AGENT="${SEC_EDGAR_USER_AGENT:-}"

usage() {
    echo "Usage: ./run.sh [build|run]"
    echo ""
    echo "  build   Build the Docker image"
    echo "  run     Run the MCP server via Docker (stdio)"
    echo ""
    echo "Environment:"
    echo "  SEC_EDGAR_USER_AGENT  Required for 'run'. Your SEC EDGAR identity (e.g. \"Name (email)\")"
}

case "${1:-}" in
    build)
        echo "Building $IMAGE_NAME..."
        docker build --platform linux/amd64 -t "$IMAGE_NAME" .
        echo "Done. Image: $IMAGE_NAME"
        ;;
    run)
        if [ -z "$USER_AGENT" ]; then
            echo "Error: SEC_EDGAR_USER_AGENT is not set."
            echo "  export SEC_EDGAR_USER_AGENT=\"Your Name (you@email.com)\""
            exit 1
        fi
        docker run --rm -i \
            --platform linux/amd64 \
            -e "SEC_EDGAR_USER_AGENT=$USER_AGENT" \
            "$IMAGE_NAME"
        ;;
    *)
        usage
        ;;
esac
