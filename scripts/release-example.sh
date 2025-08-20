#!/usr/bin/env bash

# Example script showing how to use the auto-release script
# This demonstrates the typical workflow for creating a release

set -euo pipefail

echo "=== CCProxy Release Example ==="
echo ""
echo "This script demonstrates how to create a release:"
echo ""
echo "1. Make sure you have the GitHub CLI installed:"
echo "   brew install gh"
echo ""
echo "2. Authenticate with GitHub (first time only):"
echo "   gh auth login"
echo ""
echo "3. Basic usage - auto-detect version and release:"
echo "   ./scripts/auto-release.sh"
echo ""
echo "4. Specify a version:"
echo "   ./scripts/auto-release.sh v0.0.5"
echo ""
echo "5. Test run without actually releasing:"
echo "   ./scripts/auto-release.sh --dry-run"
echo ""
echo "6. Build only (skip upload):"
echo "   ./scripts/auto-release.sh --no-build"
echo ""
echo "7. Clean build artifacts:"
echo "   ./scripts/auto-release.sh --clean-only"
echo ""

# Check current status
echo "=== Current Repository Status ==="
echo "Current branch: $(git branch --show-current)"
echo "Latest commit: $(git log -1 --oneline)"
echo "Latest tag: $(git describe --tags --abbrev=0 2>/dev/null || echo 'No tags found')"
echo ""

# Check what would be built
echo "=== Build Status ==="
if [[ -d "tray/build" ]]; then
    echo "Existing build artifacts:"
    ls -la tray/build/*.zip 2>/dev/null || echo "No zip files found"
else
    echo "No build directory found"
fi
echo ""

echo "=== Next Steps ==="
echo "1. Ensure your code is committed and pushed"
echo "2. Tag your release if needed: git tag v0.0.5 && git push origin v0.0.5" 
echo "3. Run: ./scripts/auto-release.sh"
echo ""