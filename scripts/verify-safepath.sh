#!/bin/bash
# verify-safepath.sh
# Verifies that all file I/O operations use safepath instead of os/ioutil packages

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

ERRORS=0
WARNINGS=0

echo "🔍 Verifying safepath usage..."

# Directories to check (excluding vendor, test fixtures, and generated files)
FILES=$(find . -type f -name '*.go' \
  -not -path './vendor/*' \
  -not -path './.git/*' \
  -not -path '*/_test.go' \
  -not -path './scripts/*' \
  -not -name '*_test.go' \
  -not -name '*_generated.go' \
  2>/dev/null || true)

# Check for forbidden imports
FORBIDDEN_IMPORTS=(
  "os\\.Open"
  "os\\.Create"
  "os\\.Mkdir"
  "os\\.MkdirAll"
  "os\\.Remove"
  "os\\.RemoveAll"
  "os\\.ReadFile"
  "os\\.WriteFile"
  "os\\.ReadDir"
  "ioutil\\.ReadFile"
  "ioutil\\.WriteFile"
  "ioutil\\.ReadDir"
  "ioutil\\.ReadAll"
  "ioutil\\.TempFile"
  "ioutil\\.TempDir"
  "filepath\\.Walk"
  "filepath\\.WalkDir"
)

# Check for direct file operations
for file in $FILES; do
  # Skip if file doesn't exist or is empty
  [ -f "$file" ] || continue
  
  # Check for forbidden imports
  if grep -qE "^\s*import\s+.*\"os\"" "$file" 2>/dev/null; then
    # Check if it's using safepath or just os for other purposes
    if grep -qE "(os\.Open|os\.Create|os\.Mkdir|os\.MkdirAll|os\.Remove|os\.RemoveAll|os\.ReadFile|os\.WriteFile|os\.ReadDir)" "$file" 2>/dev/null; then
      # Check if safepath is also imported
      if ! grep -q "gowritter/safepath" "$file" 2>/dev/null; then
        echo -e "${RED}❌ ERROR:${NC} $file uses os file operations without safepath"
        grep -nE "(os\.Open|os\.Create|os\.Mkdir|os\.MkdirAll|os\.Remove|os\.RemoveAll|os\.ReadFile|os\.WriteFile|os\.ReadDir)" "$file" | head -5
        ((ERRORS++)) || true
      fi
    fi
  fi
  
  # Check for ioutil usage (deprecated but still check)
  if grep -qE "^\s*import\s+.*\"io/ioutil\"" "$file" 2>/dev/null; then
    echo -e "${RED}❌ ERROR:${NC} $file imports deprecated io/ioutil package"
    grep -n "io/ioutil" "$file" | head -3
    ((ERRORS++)) || true
  fi
  
  # Check for filepath.Walk/WalkDir (should use safepath)
  if grep -qE "(filepath\.Walk|filepath\.WalkDir)" "$file" 2>/dev/null; then
    if ! grep -q "gowritter/safepath" "$file" 2>/dev/null; then
      echo -e "${YELLOW}⚠️  WARNING:${NC} $file uses filepath.Walk without safepath"
      grep -nE "(filepath\.Walk|filepath\.WalkDir)" "$file" | head -3
      ((WARNINGS++)) || true
    fi
  fi
done

# Check test files separately (more lenient)
TEST_FILES=$(find . -type f -name '*_test.go' \
  -not -path './vendor/*' \
  -not -path './.git/*' \
  2>/dev/null || true)

for file in $TEST_FILES; do
  [ -f "$file" ] || continue
  
  # Check for ioutil in tests (should still avoid if possible)
  if grep -qE "^\s*import\s+.*\"io/ioutil\"" "$file" 2>/dev/null; then
    echo -e "${YELLOW}⚠️  WARNING:${NC} Test file $file imports deprecated io/ioutil"
    ((WARNINGS++)) || true
  fi
done

# Summary
echo ""
if [ $ERRORS -eq 0 ] && [ $WARNINGS -eq 0 ]; then
  echo -e "${GREEN}✅ All file operations use safepath correctly!${NC}"
  exit 0
elif [ $ERRORS -eq 0 ]; then
  echo -e "${YELLOW}⚠️  Verification completed with $WARNINGS warning(s)${NC}"
  exit 0
else
  echo -e "${RED}❌ Verification failed with $ERRORS error(s) and $WARNINGS warning(s)${NC}"
  exit 1
fi

