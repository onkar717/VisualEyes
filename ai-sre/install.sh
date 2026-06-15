#!/usr/bin/env bash
# VisualEyes AI-SRE Python CLI installer
# Usage: bash ai-sre/install.sh
set -e

VEYE_DIR="$HOME/.visualeyes"
VENV_DIR="$VEYE_DIR/venv"
BIN_DIR="$HOME/.local/bin"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "════════════════════════════════════════"
echo "  VisualEyes AI-SRE Python CLI Setup"
echo "════════════════════════════════════════"
echo ""

mkdir -p "$VEYE_DIR" "$BIN_DIR"

# Virtual environment
if [ ! -d "$VENV_DIR" ]; then
    echo "Creating Python virtual environment at $VENV_DIR ..."
    python3 -m venv "$VENV_DIR"
fi

source "$VENV_DIR/bin/activate"

echo "Installing dependencies (this takes ~60s on first run)..."
pip install --upgrade pip --quiet
pip install -e "$SCRIPT_DIR" --quiet

# Symlink veye-ai into ~/.local/bin
ln -sf "$VENV_DIR/bin/veye-ai" "$BIN_DIR/veye-ai"

# Create default .env if missing
if [ ! -f "$VEYE_DIR/.env" ]; then
    cat > "$VEYE_DIR/.env" <<'EOF'
# VisualEyes AI-SRE Configuration
# Provider: groq | openai | anthropic | mistral | ollama
LLM_PROVIDER=groq
LLM_MODEL=groq/llama-3.3-70b-versatile
GROQ_API_KEY=your_groq_api_key_here
# OPENAI_API_KEY=
# ANTHROPIC_API_KEY=
# MISTRAL_API_KEY=

# Kubernetes namespaces to monitor (comma-separated)
K8S_NAMESPACES=default

# Safety: dry-run by default; set to false to allow kubectl execution
DRY_RUN=true
AUTO_REMEDIATE=false
EOF
    echo "Created configuration: $VEYE_DIR/.env"
fi

# PATH hint
if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    echo ""
    echo "⚠  $BIN_DIR is not in your PATH."
    echo "   Add to ~/.bashrc or ~/.zshrc:"
    echo "   export PATH=\"\$HOME/.local/bin:\$PATH\""
fi

echo ""
echo "════════════════════════════════════════"
echo "  Installation Complete ✓"
echo "════════════════════════════════════════"
echo ""
echo "Next steps:"
echo "  1. Edit $VEYE_DIR/.env and add your API key"
echo "  2. Run: veye-ai status     # instant cluster snapshot (no LLM)"
echo "  3. Run: veye-ai scan       # full 6-agent AI-SRE scan"
echo "  4. Run: veye-ai --help     # all commands"
echo ""
