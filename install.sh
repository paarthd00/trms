#!/bin/bash
{
  if ! command -v go &>/dev/null; then
    echo "Go is not installed. Please install Go from https://go.dev/doc/install"
    exit 1
  fi

  INSTALL_DIR="${1:-/usr/local/bin}"

  go build -o trms main.go

  sudo mv trms "$INSTALL_DIR"

  sudo chmod +x "$INSTALL_DIR/trms"

  current_shell=$(echo $SHELL | awk -F '/' '{print $NF}')

  echo "alias trms='trms -envfile $(pwd)/.env'" >>~/."$current_shell"rc
  
  exec $SHELL

  echo "trms installed successfully in $INSTALL_DIR"

} 2>/dev/null
