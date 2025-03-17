#!/bin/bash

# Exit on error
set -e

# Install mockery if not installed
if ! command -v mockery &>/dev/null; then
    echo "Installing mockery..."
    go install github.com/vektra/mockery/v2@latest
fi

# Remove all existing mocks
echo "Removing all existing mocks..."
find . -name "mock*" -exec rm -rf {} +

# Generate mocks using command line arguments
echo "Generating mocks for all interfaces..."
mockery --config .mockery.yaml

echo "Mock generation completed successfully!"
