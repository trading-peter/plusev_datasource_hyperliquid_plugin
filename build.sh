#!/bin/bash
set -e

echo "Building plugin..."
go mod tidy
GOOS=wasip1 GOARCH=wasm go build -o plugin.wasm -buildmode=c-shared .
echo "Plugin built successfully"
