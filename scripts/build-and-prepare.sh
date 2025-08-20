#!/usr/bin/env bash

set -euo pipefail

# Color codes for better output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BUILD_SCRIPT="$PROJECT_ROOT/tray/build.sh"
BUILD_DIR="$PROJECT_ROOT/tray/build"
VERBOSE=${VERBOSE:-"false"}

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

log_verbose() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "${NC}[VERBOSE]${NC} $1"
    fi
}

# Check if required tools are available
check_dependencies() {
    log_info "Checking dependencies..."
    
    local all_good=true
    
    if [[ ! -f "$BUILD_SCRIPT" ]]; then
        log_error "Build script not found at $BUILD_SCRIPT"
        all_good=false
    fi
    
    if [[ "$all_good" == "false" ]]; then
        log_error "Some dependencies are missing."
        exit 1
    fi
    
    log_success "All dependencies are available"
}

# Get the current version/tag
get_version() {
    local version=""
    
    # Try to get version from git tag
    if git describe --tags --exact-match HEAD &>/dev/null; then
        version=$(git describe --tags --exact-match HEAD)
        log_info "Using current git tag: $version"
    else
        # Generate version based on latest tag + commit
        local latest_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "0.0.4")
        local commit_count=$(git rev-list --count ${latest_tag}..HEAD 2>/dev/null || echo "1")
        local short_commit=$(git rev-parse --short HEAD)
        version="${latest_tag}-dev-${commit_count}-${short_commit}"
        log_info "Generated version: $version"
    fi
    
    echo "$version"
}

# Clean previous build artifacts
clean_builds() {
    log_info "Cleaning previous build artifacts..."
    
    if [[ -d "$BUILD_DIR" ]]; then
        rm -f "$BUILD_DIR"/*.zip
        log_verbose "Removed existing zip files"
    fi
    
    log_success "Build artifacts cleaned"
}

# Build binaries for Mac and Windows
build_binaries() {
    log_info "Building binaries for Mac and Windows..."
    
    cd "$PROJECT_ROOT/tray"
    
    # Build for Mac (both architectures)
    log_info "Building for macOS..."
    if ! "$BUILD_SCRIPT" -p "darwin/amd64,darwin/arm64"; then
        log_error "Failed to build for macOS"
        return 1
    fi
    
    # Build for Windows
    log_info "Building for Windows..."
    if ! "$BUILD_SCRIPT" -p "windows/amd64"; then
        log_error "Failed to build for Windows"
        return 1
    fi
    
    log_success "All binaries built successfully"
}

# Show what was built
show_build_results() {
    local version="$1"
    
    log_info "Build Results for version $version:"
    echo ""
    
    if [[ -d "$BUILD_DIR" ]]; then
        local zip_files=()
        while IFS= read -r -d '' file; do
            zip_files+=("$file")
        done < <(find "$BUILD_DIR" -name "*.zip" -type f -print0)
        
        if [[ ${#zip_files[@]} -gt 0 ]]; then
            echo "Built artifacts:"
            for zip_file in "${zip_files[@]}"; do
                local filename=$(basename "$zip_file")
                local size=$(ls -lh "$zip_file" | awk '{print $5}')
                echo "  ✅ $filename ($size)"
            done
            echo ""
            echo "Manual upload instructions:"
            echo "1. Go to: https://github.com/daodao97/claude-code-proxy/releases"
            echo "2. Create new release with tag: $version"
            echo "3. Upload these files:"
            for zip_file in "${zip_files[@]}"; do
                echo "   - $zip_file"
            done
            echo ""
            echo "Or use GitHub CLI:"
            echo "gh release create $version --title \"Release $version\" --notes \"Release notes here\""
            for zip_file in "${zip_files[@]}"; do
                echo "gh release upload $version \"$zip_file\""
            done
        else
            log_error "No zip files were created"
            return 1
        fi
    else
        log_error "Build directory does not exist"
        return 1
    fi
}

# Show help
show_help() {
    cat << EOF
Build and Prepare Release Script for CCProxy

USAGE:
    $0 [OPTIONS] [VERSION]

OPTIONS:
    -h, --help          Show this help message
    -v, --verbose       Enable verbose output
    -c, --clean-only    Only clean build artifacts and exit

ARGUMENTS:
    VERSION            Version tag to use (optional, auto-detected from git)

EXAMPLES:
    $0                 # Auto-detect version and build
    $0 v1.2.3          # Use specific version tag
    $0 -v              # Verbose output
    $0 --clean-only    # Just clean artifacts

This script will:
1. Check dependencies
2. Clean previous build artifacts
3. Build binaries for Mac and Windows
4. Show manual upload instructions
EOF
}

# Main execution
main() {
    local version_arg=""
    local clean_only=false
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -v|--verbose)
                VERBOSE="true"
                log_info "Verbose mode enabled"
                shift
                ;;
            -c|--clean-only)
                clean_only=true
                shift
                ;;
            -*)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
            *)
                version_arg="$1"
                shift
                ;;
        esac
    done
    
    log_info "CCProxy Build and Prepare Script started"
    
    # Check dependencies
    check_dependencies
    
    # Clean artifacts
    clean_builds
    
    # If clean-only mode, exit here
    if [[ "$clean_only" == "true" ]]; then
        log_success "Clean completed"
        exit 0
    fi
    
    # Get version
    local version="${version_arg:-$(get_version)}"
    log_info "Target version: $version"
    
    # Build binaries
    build_binaries
    
    # Show results
    show_build_results "$version"
    
    log_success "Build and prepare completed successfully!"
}

# Execute main function with all arguments
main "$@"