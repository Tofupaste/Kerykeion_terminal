#!/usr/bin/env bash
# Kerykeion Terminal - One-click Installer Script
set -e

echo "=================================================="
echo "       Kerykeion Terminal - One-Click Installer   "
echo "=================================================="

# Check if go compiler is installed
if ! command -v go &> /dev/null; then
    echo "[!] Go compiler tidak ditemukan di sistem kamu."
    echo "[>] Mendeteksi sistem operasi untuk panduan instalasi Go..."

    if [ -f /etc/os-release ]; then
        . /etc/os-release
        DISTRO_ID="${ID:-unknown}"
        DISTRO_LIKE="${ID_LIKE:-unknown}"
    elif [ "$(uname -s)" = "Darwin" ]; then
        DISTRO_ID="darwin"
    fi

    echo ""
    echo "Silakan pasang Go terlebih dahulu dengan perintah berikut:"
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
                echo "  Unduh Go dari: https://go.dev/dl/"
            fi
            ;;
    esac
    echo ""
    exit 1
fi

# Execute automated installer using go run
echo "[>] Menjalankan installer otomatis Kerykeion Terminal..."
go run ./cmd/kerxt install "$@"
