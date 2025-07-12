#!/bin/bash
# Extract changelog section for the current version from CHANGELOG.md

VERSION=$1
if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version>"
  exit 1
fi

# Remove 'v' prefix
VERSION_CLEAN=${VERSION#v}

if [ -f "CHANGELOG.md" ]; then
  awk -v version="$VERSION_CLEAN" '
    BEGIN { found=0; printing=0 }
    /^## \[/ { 
        if (printing) exit
        if ($0 ~ "\\[" version) {
            found=1
            printing=1
            next
        }
    }
    printing && /^## \[/ { exit }
    printing { print }
    ' CHANGELOG.md
else
  echo "No changelog available for this release."
fi
