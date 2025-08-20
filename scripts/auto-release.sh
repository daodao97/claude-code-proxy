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
    
    # Check for required commands
    if ! command -v gh &> /dev/null; then
        log_error "GitHub CLI (gh) is not installed"
        log_info "Install with: brew install gh"
        all_good=false
    fi
    
    if ! command -v git &> /dev/null; then
        log_error "Git is not installed"
        all_good=false
    fi
    
    if [[ ! -f "$BUILD_SCRIPT" ]]; then
        log_error "Build script not found at $BUILD_SCRIPT"
        all_good=false
    fi
    
    if [[ "$all_good" == "false" ]]; then
        log_error "Some dependencies are missing. Please install them before continuing."
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
        local latest_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "0.0.0")
        local commit_count=$(git rev-list --count ${latest_tag}..HEAD 2>/dev/null || echo "1")
        local short_commit=$(git rev-parse --short HEAD)
        # Use GitHub-compatible pre-release format
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
    
    # Change to tray directory
    local original_dir=$(pwd)
    cd "$PROJECT_ROOT/tray" || { log_error "Failed to change to tray directory"; return 1; }
    
    # Build for Mac and Windows using individual commands (more reliable)
    log_info "Building for macOS..."
    if ! ./build.sh mac arm64; then
        log_error "Failed to build for macOS ARM64"
        cd "$original_dir"
        return 1
    fi
    
    if ! ./build.sh mac amd64; then
        log_error "Failed to build for macOS AMD64"
        cd "$original_dir"
        return 1
    fi
    
    # Build for Windows
    log_info "Building for Windows..."
    if ! ./build.sh windows amd64; then
        log_error "Failed to build for Windows"
        cd "$original_dir"
        return 1
    fi
    
    # Return to original directory
    cd "$original_dir"
    
    log_success "All binaries built successfully"
}

# Auto commit changes if there are any
auto_commit_changes() {
    log_info "Checking for uncommitted changes..."
    
    # Check if there are any changes to commit (including untracked files)
    if ! git diff --quiet HEAD || [[ -n "$(git status --porcelain)" ]]; then
        log_info "Found uncommitted changes, preparing to commit..."
        
        # Show what will be committed
        log_verbose "Changes to be committed:"
        if [[ "$VERBOSE" == "true" ]]; then
            git status --porcelain
        fi
        
        # Add all changes
        git add .
        
        # Generate commit message based on recent changes
        local commit_msg="chore: prepare release $(get_version)"
        
        # Create commit
        log_info "Creating commit: $commit_msg"
        if git commit -m "$commit_msg

🤖 Generated with [Claude Code](https://claude.ai/code)

Co-Authored-By: Claude <noreply@anthropic.com>"; then
            log_success "Changes committed successfully"
            
            # Push changes
            log_info "Pushing changes to remote..."
            if git push; then
                log_success "Changes pushed to remote"
            else
                log_warn "Failed to push changes to remote, but continuing with release"
            fi
        else
            log_error "Failed to commit changes"
            return 1
        fi
    else
        log_info "No uncommitted changes found"
    fi
    
    return 0
}

# Upload assets to GitHub release
upload_release_assets() {
    local version="$1"
    local release_exists=false
    
    # Check if release already exists
    if gh release view "$version" &>/dev/null; then
        log_info "Release $version already exists"
        release_exists=true
    else
        log_info "Creating new release $version..."
        if ! gh release create "$version" --title "Release $version" --notes "$(generate_release_notes "$version")"; then
            log_error "Failed to create release $version"
            return 1
        fi
        log_success "Release $version created"
    fi
    
    # Find and upload zip files
    log_info "Uploading build artifacts..."
    
    local uploaded_count=0
    local zip_files=()
    
    # Collect all zip files
    if [[ -d "$BUILD_DIR" ]]; then
        while IFS= read -r -d '' file; do
            zip_files+=("$file")
        done < <(find "$BUILD_DIR" -name "*.zip" -type f -print0)
    fi
    
    if [[ ${#zip_files[@]} -eq 0 ]]; then
        log_error "No zip files found in $BUILD_DIR"
        return 1
    fi
    
    log_info "Found ${#zip_files[@]} zip files to upload"
    
    # Upload each zip file
    for zip_file in "${zip_files[@]}"; do
        local filename=$(basename "$zip_file")
        log_verbose "Uploading $filename..."
        
        # Remove existing asset if it exists (for re-uploads)
        if gh release view "$version" --json assets -q ".assets[].name" | grep -q "^$filename$"; then
            log_warn "Asset $filename already exists, deleting first..."
            gh release delete-asset "$version" "$filename" --yes 2>/dev/null || true
        fi
        
        if gh release upload "$version" "$zip_file"; then
            log_success "Uploaded: $filename"
            ((uploaded_count++))
        else
            log_error "Failed to upload: $filename"
            return 1
        fi
    done
    
    log_success "Successfully uploaded $uploaded_count assets to release $version"
    
    # Show release URL
    local repo_url=$(gh repo view --json url -q .url)
    log_info "Release URL: $repo_url/releases/tag/$version"
}

# Generate release notes
generate_release_notes() {
    local version="$1"
    local previous_tag=""
    
    # Try to get previous tag
    previous_tag=$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || echo "")
    
    cat << EOF
## Release $version

### 📦 Binary Downloads
- **macOS**: Download \`CCProxy-mac-amd64.zip\` (Intel) or \`CCProxy-mac-arm64.zip\` (Apple Silicon)
- **Windows**: Download \`CCProxy-win-amd64.zip\`

### 🔄 Changes
EOF
    
    if [[ -n "$previous_tag" ]]; then
        echo ""
        echo "**Commits since $previous_tag:**"
        git log --pretty=format:"- %s (%h)" "$previous_tag..HEAD" 2>/dev/null || echo "- Various improvements and bug fixes"
    else
        echo "- Initial release with core functionality"
    fi
    
    cat << EOF

### 🚀 Installation
1. Download the appropriate zip file for your platform
2. Extract the archive
3. Run the executable

### 💡 Usage
Refer to the project documentation for detailed usage instructions.

---
🤖 *This release was created automatically*
EOF
}

# Show help
show_help() {
    cat << EOF
Auto Release Script for CCProxy

USAGE:
    $0 [OPTIONS] [VERSION]

OPTIONS:
    -h, --help          Show this help message
    -v, --verbose       Enable verbose output
    -c, --clean-only    Only clean build artifacts and exit
    -n, --no-build      Skip building, only upload existing files
    --no-commit        Skip auto-commit of changes
    --dry-run          Show what would be done without actually doing it

ARGUMENTS:
    VERSION            Version tag to use (optional, auto-detected from git)

EXAMPLES:
    $0                 # Auto-detect version, build and release
    $0 v1.2.3          # Use specific version tag
    $0 -v              # Verbose output
    $0 --clean-only    # Just clean artifacts
    $0 --no-build      # Upload existing builds without rebuilding

ENVIRONMENT VARIABLES:
    VERBOSE            Enable verbose output (true/false)

This script will:
1. Check dependencies
2. Auto-commit any uncommitted changes (unless --no-commit)
3. Clean previous build artifacts
4. Build binaries for Mac and Windows
5. Create/update GitHub release
6. Upload zip files as release assets
EOF
}

# Main execution
main() {
    local version_arg=""
    local clean_only=false
    local no_build=false
    local no_commit=false
    local dry_run=false
    
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
            -n|--no-build)
                no_build=true
                shift
                ;;
            --no-commit)
                no_commit=true
                shift
                ;;
            --dry-run)
                dry_run=true
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
    
    log_info "CCProxy Auto Release Script started"
    
    # Check dependencies
    check_dependencies
    
    # Clean artifacts (unless we're using existing builds)
    if [[ "$no_build" != "true" ]]; then
        clean_builds
    fi
    
    # If clean-only mode, exit here
    if [[ "$clean_only" == "true" ]]; then
        clean_builds
        log_success "Clean completed"
        exit 0
    fi
    
    # Get version
    if [[ -n "$version_arg" ]]; then
        local version="$version_arg"
        log_info "Using specified version: $version"
    else
        local version="$(get_version)"
    fi
    
    log_info "Target version: $version"
    
    if [[ "$dry_run" == "true" ]]; then
        log_info "DRY RUN MODE - No actual changes will be made"
        exit 0
    fi
    
    # Auto commit changes before building/releasing (unless skipped)
    if [[ "$no_commit" != "true" ]]; then
        auto_commit_changes
    else
        log_info "Skipping auto-commit as requested"
    fi
    
    # Build binaries (unless skipped)
    if [[ "$no_build" != "true" ]]; then
        build_binaries
    else
        log_info "Skipping build as requested"
    fi
    
    # Upload to GitHub release
    upload_release_assets "$version"
    
    log_success "Auto release completed successfully!"
    log_info "Version: $version"
}

# Execute main function with all arguments
main "$@"