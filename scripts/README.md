# CCProxy Release Scripts

This directory contains scripts to automate the build and release process for CCProxy.

## Scripts Overview

### 1. `auto-release.sh` - Full Automated Release
Complete automation script that builds binaries and uploads to GitHub releases.

**Features:**
- Builds for Mac (amd64/arm64) and Windows (amd64)
- Creates GitHub release automatically
- Uploads zip files as release assets
- Auto-detects version from git tags

**Requirements:**
- GitHub CLI (`gh`) installed and authenticated
- All build dependencies (see `tray/build.sh` requirements)

**Usage:**
```bash
# Full automatic release
./scripts/auto-release.sh

# Specify version
./scripts/auto-release.sh v0.0.5

# Dry run to test
./scripts/auto-release.sh --dry-run

# Upload existing builds only
./scripts/auto-release.sh --no-build
```

### 2. `build-and-prepare.sh` - Build Only
Builds binaries and provides manual upload instructions. Good for when you don't have GitHub CLI or want more control.

**Usage:**
```bash
# Build and show upload instructions
./scripts/build-and-prepare.sh

# Specify version
./scripts/build-and-prepare.sh v0.0.5

# Verbose output
./scripts/build-and-prepare.sh -v
```

### 3. `release-example.sh` - Documentation
Shows example usage and current repository status.

## Quick Start

1. **First time setup:**
   ```bash
   # Install GitHub CLI (for auto-release.sh)
   brew install gh
   
   # Authenticate
   gh auth login
   ```

2. **Create a release:**
   ```bash
   # Option A: Fully automated (recommended)
   ./scripts/auto-release.sh

   # Option B: Manual control
   ./scripts/build-and-prepare.sh
   # Then manually upload to GitHub releases
   ```

## Build Process

Both scripts use `tray/build.sh` to build binaries for:
- **macOS**: `CCProxy-mac-amd64.zip` and `CCProxy-mac-arm64.zip` 
- **Windows**: `CCProxy-win-amd64.zip`

Build artifacts are created in `tray/build/` directory.

## Version Detection

Scripts auto-detect version using this priority:
1. Command line argument (e.g., `v0.0.5`)
2. Current git tag (if HEAD is tagged)
3. Generated from latest tag + commits (e.g., `0.0.4-dev.3.abc1234`)

## GitHub Release Process

When using `auto-release.sh`:
1. Checks if release exists (creates if not)
2. Uploads all zip files from `tray/build/`
3. Overwrites existing assets if they exist
4. Generates release notes automatically

## Manual Upload Process

When using `build-and-prepare.sh`:
1. Build artifacts are created in `tray/build/`
2. Script shows manual upload instructions
3. You manually create release on GitHub and upload files

## Troubleshooting

### "GitHub CLI (gh) is not installed"
Install with: `brew install gh`

### "Build script not found"
Make sure you're running from the project root directory.

### "Failed to build for macOS/Windows"
Check build dependencies in `tray/build.sh`. Common issues:
- Missing Xcode tools (macOS)
- Missing MinGW (Windows cross-compilation)
- Missing Go toolchain

### Permission errors
Make scripts executable:
```bash
chmod +x scripts/*.sh
```

## Examples

### Release workflow with git tags:
```bash
# Commit your changes
git add .
git commit -m "feat: new feature"
git push

# Tag release
git tag v0.0.5
git push origin v0.0.5

# Create release
./scripts/auto-release.sh
```

### Development release:
```bash
# Creates version like: 0.0.4-dev.2.abc1234
./scripts/auto-release.sh
```

### Build without releasing:
```bash
./scripts/build-and-prepare.sh v0.0.5
# Then manually upload the files shown in output
```