#!/bin/bash

# Docker Network Doctor Installation Script
# This script detects the OS and installs the plugin appropriately

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
REPO="yourusername/docker-net-doctor"
BINARY_NAME="docker-net-doctor"

# Functions for colored output
info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    
    # Convert architecture names
    case $ARCH in
        x86_64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) error "Unsupported architecture: $ARCH" ;;
    esac
    
    # Handle special cases for OS
    case $OS in
        darwin) OS="darwin" ;;
        linux) OS="linux" ;;
        mingw*|msys*|cygwin*) OS="windows" ;;
        *) error "Unsupported OS: $OS" ;;
    esac
    
    PLATFORM="${OS}-${ARCH}"
    info "Detected platform: $PLATFORM"
}

# Check if Docker is installed and get plugin directory
check_docker() {
    if ! command -v docker &> /dev/null; then
        error "Docker is not installed. Please install Docker first."
    fi
    
    # Verify Docker is running
    if ! docker info &> /dev/null; then
        error "Docker daemon is not running. Please start Docker."
    fi
    
    info "Docker detected: $(docker --version)"
}

# Determine installation directory
get_install_dir() {
    # Check if user wants system-wide installation
    if [[ $EUID -eq 0 ]] || [[ "${INSTALL_SYSTEM:-false}" == "true" ]]; then
        # System-wide installation
        if [[ -d "/usr/local/lib/docker/cli-plugins" ]]; then
            INSTALL_DIR="/usr/local/lib/docker/cli-plugins"
        elif [[ -d "/usr/lib/docker/cli-plugins" ]]; then
            INSTALL_DIR="/usr/lib/docker/cli-plugins"
        else
            INSTALL_DIR="/usr/local/lib/docker/cli-plugins"
            warn "Creating system plugin directory: $INSTALL_DIR"
            mkdir -p "$INSTALL_DIR"
        fi
        NEEDS_SUDO=true
    else
        # User installation
        INSTALL_DIR="$HOME/.docker/cli-plugins"
        mkdir -p "$INSTALL_DIR"
        NEEDS_SUDO=false
    fi
    
    info "Installation directory: $INSTALL_DIR"
}

# Download the latest release
download_binary() {
    # Get latest release URL from GitHub
    info "Fetching latest release..."
    
    if command -v curl &> /dev/null; then
        DOWNLOAD_URL=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" \
            | grep "browser_download_url.*$PLATFORM" \
            | cut -d '"' -f 4)
    elif command -v wget &> /dev/null; then
        DOWNLOAD_URL=$(wget -qO- "https://api.github.com/repos/$REPO/releases/latest" \
            | grep "browser_download_url.*$PLATFORM" \
            | cut -d '"' -f 4)
    else
        error "Neither curl nor wget found. Please install one."
    fi
    
    if [[ -z "$DOWNLOAD_URL" ]]; then
        error "Could not find release for platform: $PLATFORM"
    fi
    
    info "Downloading from: $DOWNLOAD_URL"
    
    # Download to temporary file
    TEMP_FILE=$(mktemp)
    if command -v curl &> /dev/null; then
        curl -L -o "$TEMP_FILE" "$DOWNLOAD_URL" || error "Download failed"
    else
        wget -O "$TEMP_FILE" "$DOWNLOAD_URL" || error "Download failed"
    fi
    
    # Extract if it's a tar.gz
    if [[ "$DOWNLOAD_URL" == *.tar.gz ]]; then
        TEMP_DIR=$(mktemp -d)
        tar -xzf "$TEMP_FILE" -C "$TEMP_DIR"
        TEMP_FILE="$TEMP_DIR/$BINARY_NAME-$PLATFORM"
    fi
    
    BINARY_PATH="$TEMP_FILE"
}

# Install the binary
install_binary() {
    info "Installing $BINARY_NAME..."
    
    TARGET_PATH="$INSTALL_DIR/$BINARY_NAME"
    
    if [[ "$NEEDS_SUDO" == "true" ]]; then
        sudo cp "$BINARY_PATH" "$TARGET_PATH"
        sudo chmod +x "$TARGET_PATH"
    else
        cp "$BINARY_PATH" "$TARGET_PATH"
        chmod +x "$TARGET_PATH"
    fi
    
    # Cleanup temporary files
    rm -rf "$BINARY_PATH" "$(dirname "$BINARY_PATH")" 2>/dev/null || true
}

# Verify installation
verify_installation() {
    info "Verifying installation..."
    
    # Check if Docker recognizes the plugin
    if docker net-doctor --version &> /dev/null; then
        info "✅ Installation successful!"
        info "Version: $(docker net-doctor --version)"
        echo ""
        info "You can now use: docker net-doctor"
        info "Try: docker net-doctor diagnose"
    else
        error "Installation verification failed"
    fi
}

# Main installation flow
main() {
    echo "🔧 Docker Network Doctor Installer"
    echo "=================================="
    echo ""
    
    detect_platform
    check_docker
    get_install_dir
    download_binary
    install_binary
    verify_installation
    
    echo ""
    echo "📚 For documentation, visit: https://github.com/$REPO"
    echo "🐛 Report issues at: https://github.com/$REPO/issues"
}

# Handle command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --system)
            INSTALL_SYSTEM=true
            shift
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --system    Install system-wide (requires sudo)"
            echo "  --help      Show this help message"
            exit 0
            ;;
        *)
            error "Unknown option: $1"
            ;;
    esac
done

# Run main installation
main
