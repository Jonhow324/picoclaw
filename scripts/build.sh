#!/usr/bin/env bash

# Exit immediately if a command exits with a non-zero status
set -euo pipefail

# ANSI color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper logging functions
info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# Print help message
show_help() {
    cat << EOF
PicoClaw Build Helper Script

Usage:
  $0 [options]

Options:
  -b, --base-path <path>    Set custom base path for Web UI (e.g. /picoclaw/).
                            Standardized automatically if slashes are missing.
  -p, --platform <os>       Target OS platform (e.g. linux, darwin, windows).
                            Defaults to host OS.
  -a, --arch <arch>         Target CPU architecture (e.g. amd64, arm64, mipsle).
                            Defaults to host architecture.
  -c, --clean               Clean build artifacts before building.
  --only-agent              Only build the core PicoClaw agent client.
  --only-launcher           Only build the web launcher (including frontend).
  -h, --help                Show this help message.

Examples:
  $0                        # Builds both agent and launcher for host platform
  $0 --clean                # Clean and rebuild both
  $0 -b /picoclaw/          # Build launcher with base path /picoclaw/
  $0 -p linux -a arm64      # Cross-compile for linux/arm64
EOF
}

# Default parameters
BASE_PATH="/"
PLATFORM=""
ARCH=""
CLEAN=false
BUILD_AGENT=true
BUILD_LAUNCHER=true

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        -b|--base-path)
            if [[ -n "${2:-}" && ! "$2" =~ ^- ]]; then
                BASE_PATH="$2"
                shift 2
            else
                error "Missing value for --base-path option."
                exit 1
            fi
            ;;
        -p|--platform)
            if [[ -n "${2:-}" && ! "$2" =~ ^- ]]; then
                PLATFORM="$2"
                shift 2
            else
                error "Missing value for --platform option."
                exit 1
            fi
            ;;
        -a|--arch)
            if [[ -n "${2:-}" && ! "$2" =~ ^- ]]; then
                ARCH="$2"
                shift 2
            else
                error "Missing value for --arch option."
                exit 1
            fi
            ;;
        -c|--clean)
            CLEAN=true
            shift
            ;;
        --only-agent)
            BUILD_AGENT=true
            BUILD_LAUNCHER=false
            shift
            ;;
        --only-launcher)
            BUILD_AGENT=false
            BUILD_LAUNCHER=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Normalize BASE_PATH
# Ensure it starts and ends with a slash.
if [[ "$BASE_PATH" != "/" ]]; then
    # Ensure it starts with /
    if [[ "$BASE_PATH" != /* ]]; then
        BASE_PATH="/$BASE_PATH"
    fi
    # Ensure it ends with /
    if [[ "$BASE_PATH" != */ ]]; then
        BASE_PATH="$BASE_PATH/"
    fi
fi

# Add common global npm and local binary directories to PATH
export PATH="$HOME/.npm-global/bin:$HOME/.local/bin:$PATH"

# Check dependencies
check_dep() {
    if ! command -v "$1" &> /dev/null; then
        error "Dependency '$1' is not installed or not in PATH."
        exit 1
    fi
}

check_dep "go"
check_dep "make"

if [[ "$BUILD_LAUNCHER" == "true" ]]; then
    check_dep "pnpm"
fi

# Prepare make arguments
MAKE_ARGS=()
if [[ -n "$PLATFORM" ]]; then
    MAKE_ARGS+=("PLATFORM=$PLATFORM")
fi
if [[ -n "$ARCH" ]]; then
    MAKE_ARGS+=("ARCH=$ARCH")
fi

# Determine target platform details
CURRENT_OS=$(go env GOOS)
CURRENT_ARCH=$(go env GOARCH)
TARGET_OS="${PLATFORM:-$CURRENT_OS}"
TARGET_ARCH="${ARCH:-$CURRENT_ARCH}"

TARGET_EXT=""
if [[ "$TARGET_OS" == "windows" ]]; then
    TARGET_EXT=".exe"
fi

# Execute Clean if requested
if [[ "$CLEAN" == "true" ]]; then
    step "Cleaning build artifacts..."
    make clean
    info "Cleanup finished."
fi

# Inject custom base path via Vite environment variable
export VITE_BASE_URL="$BASE_PATH"
info "Target Platform: ${TARGET_OS}/${TARGET_ARCH}"
info "Web UI Base Path: $VITE_BASE_URL"

# Helper function to print file details
print_file_info() {
    local file_path="$1"
    local name="$2"
    if [[ -f "$file_path" ]]; then
        local size
        if [[ "$OSTYPE" == "darwin"* ]]; then
            size=$(stat -f%z "$file_path")
        else
            size=$(stat -c%s "$file_path")
        fi
        
        # Calculate human-readable size using awk
        local size_str
        size_str=$(awk -v size="$size" 'BEGIN {
            if (size > 1048576) {
                printf "%.2f MB", size / 1048576
            } else {
                printf "%.2f KB", size / 1024
            }
        }')
        
        echo -e "  - ${GREEN}${name}${NC}: $file_path (${size_str})"
    else
        echo -e "  - ${RED}${name}${NC}: Not found at $file_path"
    fi
}

start_total_time=$SECONDS

# Build PicoClaw Core Agent
if [[ "$BUILD_AGENT" == "true" ]]; then
    step "Building PicoClaw Core Agent client..."
    start_time=$SECONDS
    
    make build "${MAKE_ARGS[@]}"
    
    duration=$((SECONDS - start_time))
    info "Core Agent build finished in ${duration}s."
fi

# Build PicoClaw Web Launcher
if [[ "$BUILD_LAUNCHER" == "true" ]]; then
    step "Building PicoClaw Web Launcher console..."
    start_time=$SECONDS
    
    make build-launcher "${MAKE_ARGS[@]}"
    
    duration=$((SECONDS - start_time))
    info "Web Launcher build finished in ${duration}s."
fi

total_duration=$((SECONDS - start_total_time))
echo ""
step "Build summary (Total time: ${total_duration}s):"

if [[ "$BUILD_AGENT" == "true" ]]; then
    print_file_info "build/picoclaw-${TARGET_OS}-${TARGET_ARCH}${TARGET_EXT}" "Agent Binary"
    if [[ -L "build/picoclaw${TARGET_EXT}" || -f "build/picoclaw${TARGET_EXT}" ]]; then
        print_file_info "build/picoclaw${TARGET_EXT}" "Agent Alias/Link"
    fi
fi

if [[ "$BUILD_LAUNCHER" == "true" ]]; then
    print_file_info "build/picoclaw-launcher-${TARGET_OS}-${TARGET_ARCH}${TARGET_EXT}" "Launcher Binary"
    if [[ -L "build/picoclaw-launcher${TARGET_EXT}" || -f "build/picoclaw-launcher${TARGET_EXT}" ]]; then
        print_file_info "build/picoclaw-launcher${TARGET_EXT}" "Launcher Alias/Link"
    fi
fi

info "All requested builds completed successfully!"
