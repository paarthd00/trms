#!/bin/bash
{
  # Check for prerequisites (Go, etc.)
  if ! command -v go &>/dev/null; then
    echo "Go is not installed. Please install Go from https://go.dev/doc/install"
    exit 1
  fi

  INSTALL_DIR="${1:-/usr/local/bin}"

  go build -o trms main.go

  sudo mv trms "$INSTALL_DIR"

  sudo chmod +x "$INSTALL_DIR/trms"

  PWD=$(pwd)

  current_shell=$(echo $SHELL | awk -F '/' '{print $NF}')

  echo "alias my-trm-search='my-trm-search -envfile $PWD/.env'" >>~/."$current_shell"rc

  echo "my-trm-search installed successfully in $INSTALL_DIR"

} 2>/dev/null
