#!/bin/bash

# Check if the package name is provided as an argument
if [ -z "$1" ]; then
  echo "Please provide a Go package name as an argument."
  exit 1
fi

# Get the current version of the package
current_version=$(go list -m $1 2>/dev/null | awk '{print $2}')

# Check if the package is found in the go.mod file
if [ -z "$current_version" ]; then
  echo "Package not found in the go.mod file."
  exit 1
fi

# Get the local pathname for the package
package_path=$(go list -f "{{.Dir}}" $1@$current_version 2>/dev/null)

# Check if the package is downloaded and available locally
if [ -z "$package_path" ]; then
  echo "Package is not available locally. Please run 'go get $1@$current_version' to download it."
  exit 1
fi

echo "Local pathname for package '$1' at version '$current_version':"
echo "$package_path"

