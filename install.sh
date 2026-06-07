#!/bin/bash
set -e

REPO="onkar717/VisualEyes"
BINARY="${1:-veye}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect OS and arch
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  linux|darwin) ;;
  msys*|cygwin*|mingw*) OS="windows" ;;
  *)
    echo "Unsupported OS: $OS"
    exit 1
    ;;
esac

# Resolve latest release tag
VERSION="${VERSION:-$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)}"

if [ -z "$VERSION" ]; then
  echo "Could not determine latest version. Set VERSION env var manually."
  exit 1
fi

# Build asset name
if [ "$OS" = "windows" ]; then
  ASSET="${BINARY}-${OS}-${ARCH}.exe"
else
  ASSET="${BINARY}-${OS}-${ARCH}"
fi

URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"

echo "Installing ${BINARY} ${VERSION} (${OS}/${ARCH})..."
echo "Downloading: ${URL}"

TMP="$(mktemp)"
curl -fsSL "$URL" -o "$TMP"
chmod +x "$TMP"

if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP" "${INSTALL_DIR}/${BINARY}"
else
  sudo mv "$TMP" "${INSTALL_DIR}/${BINARY}"
fi

echo ""
echo "${BINARY} installed to ${INSTALL_DIR}/${BINARY}"
echo "Run: ${BINARY} --help"
