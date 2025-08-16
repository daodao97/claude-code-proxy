#!/usr/bin/env bash

set -euo pipefail

# tips
# brew install sips
# brew install mingw-w64
# go install -v github.com/akavel/rsrc@latest

# Color codes for better output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Build configuration
PLATFORMS=${PLATFORMS:-"darwin/amd64,darwin/arm64,linux/amd64,linux/arm64,windows/amd64"}
OUTPUT_DIR=${OUTPUT_DIR:-"build"}
VERBOSE=${VERBOSE:-"false"}

# OpenSSL paths (mainly for macOS)
export LDFLAGS="-L/usr/local/opt/openssl@3/lib"
export CPPFLAGS="-I/usr/local/opt/openssl@3/include"

# Go build command
readonly GOBUILD="go build"

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

# Command existence check function
check_command() {
    local cmd=$1
    local desc=${2:-"$cmd"}
    local install_hint=${3:-""}
    
    if ! command -v "$cmd" &> /dev/null; then
        log_error "$desc is not installed or not in PATH"
        if [[ -n "$install_hint" ]]; then
            log_info "Install hint: $install_hint"
        fi
        return 1
    fi
    log_verbose "$desc found: $(command -v "$cmd")"
    return 0
}

# Check all required dependencies
check_dependencies() {
    log_info "Checking build dependencies..."
    local all_good=true
    
    # Go is essential
    if ! check_command "go" "Go" "Visit https://golang.org/dl/"; then
        all_good=false
    fi
    
    # Platform-specific tools
    local current_os=$(uname -s)
    case "$current_os" in
        Darwin*)
            check_command "sips" "sips (macOS image processing)" "sips is built into macOS" || all_good=false
            check_command "iconutil" "iconutil (macOS icon utility)" "iconutil is built into macOS" || all_good=false
            ;;
        Linux*)
            check_command "zip" "zip utility" "sudo apt-get install zip" || all_good=false
            # For cross-compilation to Windows/macOS from Linux
            if [[ "$PLATFORMS" == *"windows"* ]]; then
                check_command "x86_64-w64-mingw32-gcc" "MinGW cross-compiler" "sudo apt-get install gcc-mingw-w64" || log_warn "MinGW not found - Windows builds will fail"
            fi
            ;;
        MINGW*|MSYS*|CYGWIN*)
            check_command "zip" "zip utility" "Install zip utility" || all_good=false
            ;;
    esac
    
    # Windows-specific tools (when building for Windows)
    if [[ "$PLATFORMS" == *"windows"* ]]; then
        check_command "rsrc" "rsrc (Windows resource compiler)" "go install github.com/akavel/rsrc@latest" || log_warn "rsrc not found - Windows resource embedding will fail"
    fi
    
    if [[ "$all_good" == "false" ]]; then
        log_error "Some dependencies are missing. Please install them before continuing."
        exit 1
    fi
    
    log_success "All dependencies are available"
}

function buildMacIcon() {
    log_info "Building macOS icons..."
    
    if [[ ! -f "icon/logo.png" ]]; then
        log_error "icon/logo.png not found"
        return 1
    fi
    
    rm -rf icons.iconset
    mkdir -p icons.iconset
    mkdir -p "$OUTPUT_DIR"
    
    log_verbose "Creating icon variants..."
    sips -z 16 16     icon/logo.png --out icons.iconset/icon_16x16.png      2>/dev/null || { log_error "Failed to create 16x16 icon"; return 1; }
    sips -z 32 32     icon/logo.png --out icons.iconset/icon_16x16@2x.png   2>/dev/null || { log_error "Failed to create 16x16@2x icon"; return 1; }
    sips -z 32 32     icon/logo.png --out icons.iconset/icon_32x32.png      2>/dev/null || { log_error "Failed to create 32x32 icon"; return 1; }
    sips -z 64 64     icon/logo.png --out icons.iconset/icon_32x32@2x.png   2>/dev/null || { log_error "Failed to create 32x32@2x icon"; return 1; }
    sips -z 128 128   icon/logo.png --out icons.iconset/icon_128x128.png    2>/dev/null || { log_error "Failed to create 128x128 icon"; return 1; }
    sips -z 256 256   icon/logo.png --out icons.iconset/icon_128x128@2x.png 2>/dev/null || { log_error "Failed to create 128x128@2x icon"; return 1; }
    sips -z 256 256   icon/logo.png --out icons.iconset/icon_256x256.png    2>/dev/null || { log_error "Failed to create 256x256 icon"; return 1; }
    sips -z 512 512   icon/logo.png --out icons.iconset/icon_256x256@2x.png 2>/dev/null || { log_error "Failed to create 256x256@2x icon"; return 1; }
    sips -z 512 512   icon/logo.png --out icons.iconset/icon_512x512.png    2>/dev/null || { log_error "Failed to create 512x512 icon"; return 1; }
    sips -z 1024 1024 icon/logo.png --out icons.iconset/icon_512x512@2x.png 2>/dev/null || { log_error "Failed to create 512x512@2x icon"; return 1; }
    
    log_verbose "Converting iconset to icns..."
    iconutil -c icns icons.iconset -o "$OUTPUT_DIR/icon.icns" 2>/dev/null || { log_error "Failed to create icns file"; return 1; }
    
    log_success "macOS icons created successfully"
}

function macIconClear() {
    log_verbose "Cleaning up icon artifacts..."
    rm -rf icons.iconset
    rm -rf "$OUTPUT_DIR/icon.icns"
}

function buildMac() {
    local arch=$1
    local amd64_variant=${2:-""}
    local name="${arch}${amd64_variant:+-$amd64_variant}"
    
    log_info "Building for macOS-${name}..."
    
    # Ensure we have the icon
    if [[ ! -f "$OUTPUT_DIR/icon.icns" ]]; then
        log_error "icon.icns not found. Run buildMacIcon first."
        return 1
    fi
    
    # Prepare app bundle
    rm -rf "$OUTPUT_DIR/CCProxy.app"
    cp -rf "$OUTPUT_DIR/meta/CCProxy.app" "$OUTPUT_DIR/" || { log_error "Failed to copy app template"; return 1; }
    mkdir -p "$OUTPUT_DIR/CCProxy.app/Contents/Resources"
    mkdir -p "$OUTPUT_DIR/CCProxy.app/Contents/MacOS"
    cp -f "$OUTPUT_DIR/icon.icns" "$OUTPUT_DIR/CCProxy.app/Contents/Resources/icon.icns" || { log_error "Failed to copy icon"; return 1; }
    
    # Build binary
    log_verbose "Compiling Go binary for darwin/$arch..."
    local build_env="GOOS=darwin GOARCH=$arch CGO_ENABLED=1"
    if [[ -n "$amd64_variant" ]]; then
        build_env="$build_env GOAMD64=$amd64_variant"
    fi
    
    if ! env $build_env $GOBUILD -o "$OUTPUT_DIR/CCProxy.app/Contents/MacOS/bin" .; then
        log_error "Failed to build macOS binary for $arch"
        return 1
    fi
    
    # Create zip package
    log_verbose "Creating zip package..."
    (cd "$OUTPUT_DIR" && zip -r "CCProxy-mac-${name}.zip" CCProxy.app >/dev/null 2>&1) || { log_error "Failed to create zip package"; return 1; }
    
    # Clean up the app bundle after zipping
    log_verbose "Cleaning up app bundle..."
    rm -rf "$OUTPUT_DIR/CCProxy.app"
    
    log_success "macOS build completed: CCProxy-mac-${name}.zip"
}

function buildWin() {
    local arch=${1:-"amd64"}
    local name="windows-${arch}"
    
    log_info "Building for ${name}..."
    
    # Check for required files
    if [[ ! -f "icon/logo.ico" ]]; then
        log_error "icon/logo.ico not found"
        return 1
    fi
    
    local manifest_file="$OUTPUT_DIR/meta/win/bin.exe.manifest"
    if [[ ! -f "$manifest_file" ]]; then
        log_warn "Windows manifest not found at $manifest_file"
        manifest_file=""
    fi
    
    mkdir -p "$OUTPUT_DIR"
    
    # Generate Windows resources
    log_verbose "Generating Windows resources..."
    local rsrc_args="-ico icon/logo.ico -o bin.exe.syso"
    if [[ -n "$manifest_file" ]]; then
        rsrc_args="-manifest $manifest_file $rsrc_args"
    fi
    
    if ! rsrc $rsrc_args; then
        log_error "Failed to generate Windows resources"
        return 1
    fi
    
    # Set up cross-compilation environment
    local build_env="GOOS=windows GOARCH=$arch CGO_ENABLED=1"
    case "$arch" in
        amd64)
            build_env="$build_env CC=x86_64-w64-mingw32-gcc"
            ;;
        386)
            build_env="$build_env CC=i686-w64-mingw32-gcc"
            ;;
        *)
            log_error "Unsupported Windows architecture: $arch"
            return 1
            ;;
    esac
    
    # Build binary
    log_verbose "Compiling Go binary for windows/$arch..."
    if ! env $build_env $GOBUILD -ldflags "-H=windowsgui" -o "$OUTPUT_DIR/CCProxy.exe" ./; then
        log_error "Failed to build Windows binary for $arch"
        rm -f bin.exe.syso
        return 1
    fi
    
    # Create zip package
    log_verbose "Creating zip package..."
    (cd "$OUTPUT_DIR" && zip -r "CCProxy-win-${arch}.zip" CCProxy.exe >/dev/null 2>&1) || { log_error "Failed to create zip package"; return 1; }
    
    # Cleanup
    rm -f bin.exe.syso
    rm -f "$OUTPUT_DIR/CCProxy.exe"
    
    log_success "Windows build completed: CCProxy-win-${arch}.zip"
}

function buildLinux() {
    local arch=$1
    local amd64_variant=${2:-""}
    local name="${arch}${amd64_variant:+-$amd64_variant}"
    
    log_info "Building for linux-${name}..."
    
    mkdir -p "$OUTPUT_DIR"
    
    # Set up cross-compilation environment
    local build_env="GOOS=linux GOARCH=$arch CGO_ENABLED=1"
    if [[ -n "$amd64_variant" ]]; then
        build_env="$build_env GOAMD64=$amd64_variant"
    fi
    
    # Use musl for static linking if available
    case "$arch" in
        amd64)
            if command -v x86_64-linux-musl-gcc &> /dev/null; then
                build_env="$build_env CC=x86_64-linux-musl-gcc CXX=x86_64-linux-musl-g++"
                log_verbose "Using musl toolchain for static linking"
            else
                log_verbose "musl toolchain not available, using default"
            fi
            ;;
        arm64)
            if command -v aarch64-linux-musl-gcc &> /dev/null; then
                build_env="$build_env CC=aarch64-linux-musl-gcc CXX=aarch64-linux-musl-g++"
                log_verbose "Using musl toolchain for static linking"
            else
                log_verbose "musl toolchain not available, using default"
            fi
            ;;
    esac
    
    # Build binary
    log_verbose "Compiling Go binary for linux/$arch..."
    if ! env $build_env $GOBUILD -o "$OUTPUT_DIR/CCProxy-linux-${name}" .; then
        log_error "Failed to build Linux binary for $arch"
        return 1
    fi
    
    # Create zip package
    log_verbose "Creating zip package..."
    (cd "$OUTPUT_DIR" && zip -r "CCProxy-linux-${name}.zip" "CCProxy-linux-${name}" >/dev/null 2>&1) || { log_error "Failed to create zip package"; return 1; }
    
    # Cleanup
    rm -f "$OUTPUT_DIR/CCProxy-linux-${name}"
    
    log_success "Linux build completed: CCProxy-linux-${name}.zip"
}

# Cross-platform build function
build_all_platforms() {
    local platforms_to_build=${1:-$PLATFORMS}
    IFS=',' read -ra PLATFORM_ARRAY <<< "$platforms_to_build"
    
    log_info "Building for platforms: $platforms_to_build"
    
    local mac_built=false
    
    for platform in "${PLATFORM_ARRAY[@]}"; do
        IFS='/' read -ra PLATFORM_PARTS <<< "$platform"
        local os="${PLATFORM_PARTS[0]}"
        local arch="${PLATFORM_PARTS[1]}"
        
        case "$os" in
            darwin)
                if [[ "$mac_built" == "false" ]]; then
                    buildMacIcon || { log_error "Failed to build macOS icons"; return 1; }
                    mac_built=true
                fi
                buildMac "$arch" || { log_error "Failed to build for macOS/$arch"; return 1; }
                ;;
            linux)
                buildLinux "$arch" || { log_error "Failed to build for Linux/$arch"; return 1; }
                ;;
            windows)
                buildWin "$arch" || { log_error "Failed to build for Windows/$arch"; return 1; }
                ;;
            *)
                log_warn "Unsupported platform: $platform"
                ;;
        esac
    done
    
    if [[ "$mac_built" == "true" ]]; then
        macIconClear
    fi
    
    log_success "All builds completed successfully!"
}

# Help function
show_help() {
    cat << EOF
CCProxy Build Script

USAGE:
    $0 [OPTIONS] [COMMAND]

OPTIONS:
    -h, --help          Show this help message
    -v, --verbose       Enable verbose output
    -p, --platforms     Comma-separated list of platforms to build (default: $PLATFORMS)
    -o, --output        Output directory (default: $OUTPUT_DIR)

COMMANDS:
    all                 Build for all configured platforms (default)
    mac [arch]          Build for macOS (amd64 or arm64)
    linux [arch]        Build for Linux (amd64 or arm64)
    windows [arch]      Build for Windows (amd64 or 386)
    check               Check build dependencies only
    clean               Clean build artifacts

ENVIRONMENT VARIABLES:
    PLATFORMS           Platforms to build (e.g., "darwin/arm64,linux/amd64")
    OUTPUT_DIR          Output directory for build artifacts
    VERBOSE             Enable verbose output (true/false)

EXAMPLES:
    $0                                      # Build all platforms
    $0 mac arm64                           # Build macOS ARM64 only
    $0 -p "darwin/arm64,linux/amd64"       # Build specific platforms
    $0 -v clean                            # Clean with verbose output
    $0 check                               # Check dependencies only

EOF
}

# Clean function
clean_build() {
    log_info "Cleaning build artifacts..."
    rm -rf "$OUTPUT_DIR"/*.zip
    rm -rf "$OUTPUT_DIR"/*.app
    rm -rf "$OUTPUT_DIR"/*.exe
    rm -rf "$OUTPUT_DIR"/CCProxy-*
    rm -rf icons.iconset
    rm -f bin.exe.syso
    log_success "Build artifacts cleaned"
}

# Parse command line arguments
parse_args() {
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
            -p|--platforms)
                PLATFORMS="$2"
                log_info "Platforms set to: $PLATFORMS"
                shift 2
                ;;
            -o|--output)
                OUTPUT_DIR="$2"
                log_info "Output directory set to: $OUTPUT_DIR"
                shift 2
                ;;
            all)
                build_all_platforms
                exit $?
                ;;
            mac)
                local arch=${2:-"arm64"}
                buildMacIcon && buildMac "$arch" && macIconClear
                exit $?
                ;;
            linux)
                local arch=${2:-"amd64"}
                buildLinux "$arch"
                exit $?
                ;;
            windows)
                local arch=${2:-"amd64"}
                buildWin "$arch"
                exit $?
                ;;
            check)
                check_dependencies
                exit $?
                ;;
            clean)
                clean_build
                exit $?
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# Main execution
main() {
    log_info "CCProxy Build Script started"
    
    # Check dependencies first
    check_dependencies
    
    # If no arguments provided, build all platforms
    if [[ $# -eq 0 ]]; then
        build_all_platforms
    else
        parse_args "$@"
    fi
}

# Execute main function with all arguments
main "$@"