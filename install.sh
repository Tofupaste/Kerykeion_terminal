#!/usr/bin/env bash
# Kerykeion Terminal - One-click Installer Script
set -e

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd -- "$SCRIPT_DIR"

echo "=================================================="
echo "       Kerykeion Terminal - One-Click Installer   "
echo "=================================================="

# Check if go compiler is installed
if ! command -v go &> /dev/null; then
    echo "[!] Go compiler not found on this system."
    echo "[>] Detecting your operating system for Go installation instructions..."

    if [ -f /etc/os-release ]; then
        . /etc/os-release
        DISTRO_ID="${ID:-unknown}"
        DISTRO_LIKE="${ID_LIKE:-unknown}"
    elif [ "$(uname -s)" = "Darwin" ]; then
        DISTRO_ID="darwin"
    fi

    echo ""
    echo "Install Go first using the following command:"
    case "$DISTRO_ID" in
        cachyos|arch|manjaro|endeavouros)
            echo "  sudo pacman -S go git"
            ;;
        fedora|rhel|centos)
            echo "  sudo dnf install -y golang git"
            ;;
        ubuntu|debian|pop|linuxmint)
            echo "  sudo apt update && sudo apt install -y golang git"
            ;;
        darwin)
            echo "  brew install go git"
            ;;
        *)
            if [[ "$DISTRO_LIKE" == *"arch"* ]]; then
                echo "  sudo pacman -S go git"
            elif [[ "$DISTRO_LIKE" == *"debian"* ]]; then
                echo "  sudo apt update && sudo apt install -y golang git"
            else
                echo "  Download Go from https://go.dev/dl/"
            fi
            ;;
    esac
    echo ""
    exit 1
fi

# Execute automated installer using go run
echo "[>] Running the Kerykeion Terminal installer..."
go run ./cmd/kerxt install "$@"
