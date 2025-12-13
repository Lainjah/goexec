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

# Forbidden patterns - direct file I/O operations that should use safepath
FORBIDDEN_PATTERNS=(
  "os\.Open[^e]"
  "os\.OpenFile"
  "os\.Create"
  "os\.Mkdir"
  "os\.MkdirAll"
  "os\.Remove[^A]"
  "os\.RemoveAll"
  "os\.ReadFile"
  "os\.WriteFile"
  "os\.ReadDir"
  "os\.Rename"
  "os\.Chmod"
  "os\.Chown"
  "os\.Stat[^u]"
  "os\.Lstat"
  "ioutil\.ReadFile"
  "ioutil\.WriteFile"
  "ioutil\.ReadDir"
  "ioutil\.ReadAll"
  "ioutil\.TempFile"
  "ioutil\.TempDir"
)

# Warning patterns - should prefer safepath but not strictly required
WARNING_PATTERNS=(
  "filepath\.Walk"
  "filepath\.WalkDir"
)

# Build regex pattern from array
FORBIDDEN_REGEX=$(IFS='|'; echo "${FORBIDDEN_PATTERNS[*]}")
WARNING_REGEX=$(IFS='|'; echo "${WARNING_PATTERNS[*]}")

# Files to check (excluding vendor, test files, and generated files)
FILES=$(find . -type f -name '*.go' \
  -not -path './vendor/*' \
  -not -path './.git/*' \
  -not -name '*_test.go' \
  -not -name '*_generated.go' \
  -not -name '*.pb.go' \
  2>/dev/null || true)

# Check each file for forbidden patterns
for file in $FILES; do
  [ -f "$file" ] || continue

  # Check for forbidden file operations
  if grep -qE "($FORBIDDEN_REGEX)" "$file" 2>/dev/null; then
    # Allow if safepath is imported
    if ! grep -q "gowritter/safepath" "$file" 2>/dev/null; then
      echo -e "${RED}❌ ERROR:${NC} $file uses forbidden file operations without safepath"
      grep -nE "($FORBIDDEN_REGEX)" "$file" | head -5
      ((ERRORS++)) || true
    fi
  fi

  # Check for deprecated ioutil import
  if grep -qE "\"io/ioutil\"" "$file" 2>/dev/null; then
    echo -e "${RED}❌ ERROR:${NC} $file imports deprecated io/ioutil package"
    grep -n "io/ioutil" "$file" | head -3
    ((ERRORS++)) || true
  fi

  # Check for warning patterns
  if grep -qE "($WARNING_REGEX)" "$file" 2>/dev/null; then
    if ! grep -q "gowritter/safepath" "$file" 2>/dev/null; then
      echo -e "${YELLOW}⚠️  WARNING:${NC} $file uses $WARNING_REGEX without safepath"
      grep -nE "($WARNING_REGEX)" "$file" | head -3
      ((WARNINGS++)) || true
    fi
  fi
done

# Check test files separately (more lenient - warnings only)
TEST_FILES=$(find . -type f -name '*_test.go' \
  -not -path './vendor/*' \
  -not -path './.git/*' \
  2>/dev/null || true)

for file in $TEST_FILES; do
  [ -f "$file" ] || continue

  # Check for deprecated ioutil in tests
  if grep -qE "\"io/ioutil\"" "$file" 2>/dev/null; then
    echo -e "${YELLOW}⚠️  WARNING:${NC} Test file $file imports deprecated io/ioutil"
    ((WARNINGS++)) || true
  fi
done

# Summary
echo ""
echo "========================================"
echo "  Safepath Verification Summary"
echo "========================================"
echo "  Errors:   $ERRORS"
echo "  Warnings: $WARNINGS"
echo "========================================"
echo ""

if [ $ERRORS -eq 0 ] && [ $WARNINGS -eq 0 ]; then
  echo -e "${GREEN}✅ All file operations use safepath correctly!${NC}"
  exit 0
elif [ $ERRORS -eq 0 ]; then
  echo -e "${YELLOW}⚠️  Verification completed with $WARNINGS warning(s)${NC}"
  exit 0
else
  echo -e "${RED}❌ Verification failed with $ERRORS error(s) and $WARNINGS warning(s)${NC}"
  echo ""
  echo "Please use github.com/victoralfred/gowritter/safepath for file operations."
  exit 1
fi
