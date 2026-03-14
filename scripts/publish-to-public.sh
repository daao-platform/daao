#!/usr/bin/env bash
# ============================================================================
# publish-to-public.sh — Deterministic publish from private → public repo
# ============================================================================
#
# Safely publishes whitelisted files from the private dan3093/daao repo
# to the public daao-platform/daao repo using a persistent local clone.
#
# Expected folder structure:
#   projects/daao/private/   ← private repo (this script lives here)
#   projects/daao/public/    ← public repo (persistent clone, auto-created)
#
# Usage:
#   bash scripts/publish-to-public.sh
#
# Environment variables:
#   DAAO_PUBLIC_DIR  — override public repo path (default: ../public)
#   DAAO_DRY_RUN     — set to "1" to skip the push step
#   DAAO_SKIP_VERIFY — set to "1" to skip post-push verification
#
# ============================================================================

set -euo pipefail

# ---------- Colors & helpers ------------------------------------------------

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }
header(){ echo -e "\n${BOLD}═══════════════════════════════════════════════════════════${NC}"; echo -e "${BOLD}  $*${NC}"; echo -e "${BOLD}═══════════════════════════════════════════════════════════${NC}\n"; }

# ---------- Phase 0: Configuration & Validation ----------------------------

header "Phase 0: Configuration & Validation"

# Detect the private repo root (where this script lives)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PRIVATE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Public dir defaults to sibling ../public (i.e. projects/daao/public/)
PUBLIC_DIR="${DAAO_PUBLIC_DIR:-$(cd "$PRIVATE_DIR/.." && pwd)/public}"

PUBLIC_REMOTE="https://github.com/daao-platform/daao.git"

info "Private repo: $PRIVATE_DIR"
info "Public repo:  $PUBLIC_DIR"
info "Remote:       $PUBLIC_REMOTE"

# Validate we're in the right repo
[[ -f "$PRIVATE_DIR/go.mod" ]]       || fail "go.mod not found in $PRIVATE_DIR — not the DAAO private repo"
[[ -d "$PRIVATE_DIR/cmd/nexus" ]]    || fail "cmd/nexus/ not found in $PRIVATE_DIR — not the DAAO private repo"
[[ -d "$PRIVATE_DIR/internal/enterprise/_public_stubs" ]] || fail "_public_stubs/ not found — cannot swap enterprise stubs"

command -v git >/dev/null 2>&1  || fail "git is not installed"
command -v go  >/dev/null 2>&1  || fail "go is not installed"

ok "Configuration validated"

# ---------- Phase 1: Initialize or Clean Public Dir -------------------------

header "Phase 1: Initialize or Clean Public Directory"

if [[ ! -d "$PUBLIC_DIR/.git" ]]; then
    info "Public directory not found — initializing fresh repo..."
    mkdir -p "$PUBLIC_DIR"
    (
        cd "$PUBLIC_DIR"
        git init
        git checkout --orphan main
        git remote add origin "$PUBLIC_REMOTE"
    )
    ok "Initialized new git repo at $PUBLIC_DIR"
else
    info "Public directory exists — cleaning files (preserving .git/)..."
    (
        cd "$PUBLIC_DIR"
        # Remove everything except .git/
        find . -maxdepth 1 -not -name '.' -not -name '.git' -exec rm -rf {} +
    )
    ok "Cleaned public directory"
fi

# ---------- Phase 2: Whitelist Copy -----------------------------------------

header "Phase 2: Whitelist Copy"

# Helper: copy a directory, creating parent dirs as needed
copy_dir() {
    local src="$PRIVATE_DIR/$1"
    local dst="$PUBLIC_DIR/$1"
    if [[ -d "$src" ]]; then
        mkdir -p "$dst"
        cp -r "$src/." "$dst/"
        ok "  $1/"
    else
        warn "  $1/ — NOT FOUND in private repo (skipping)"
    fi
}

# Helper: copy a single file, creating parent dirs as needed
copy_file() {
    local src="$PRIVATE_DIR/$1"
    local dst="$PUBLIC_DIR/$1"
    if [[ -f "$src" ]]; then
        mkdir -p "$(dirname "$dst")"
        cp "$src" "$dst"
        ok "  $1"
    else
        warn "  $1 — NOT FOUND in private repo (skipping)"
    fi
}

info "Copying whitelisted directories..."

# --- cmd/nexus/ — Go files only, NOT certs/ ---
mkdir -p "$PUBLIC_DIR/cmd/nexus"
find "$PRIVATE_DIR/cmd/nexus" -maxdepth 1 -name '*.go' -exec cp {} "$PUBLIC_DIR/cmd/nexus/" \;
ok "  cmd/nexus/*.go (excluding certs/)"

# --- cmd/daao/ — satellite daemon CLI (full copy) ---
copy_dir "cmd/daao"

# --- cmd/daao-mock/ — mock satellite for testing (full copy) ---
copy_dir "cmd/daao-mock"

# --- cockpit/ — full dir, excluding node_modules, dist, .env* ---
if [[ -d "$PRIVATE_DIR/cockpit" ]]; then
    mkdir -p "$PUBLIC_DIR/cockpit"
    # Use tar to copy with exclusions (works in Git Bash)
    (cd "$PRIVATE_DIR" && tar cf - \
        --exclude='cockpit/node_modules' \
        --exclude='cockpit/dist' \
        --exclude='cockpit/certs' \
        --exclude='cockpit/.env' \
        --exclude='cockpit/.env.*' \
        --exclude='cockpit/.env.local' \
        --exclude='cockpit/.stitch-mockups' \
        cockpit) | (cd "$PUBLIC_DIR" && tar xf -)
    # Re-include .env.example if it exists
    [[ -f "$PRIVATE_DIR/cockpit/.env.example" ]] && cp "$PRIVATE_DIR/cockpit/.env.example" "$PUBLIC_DIR/cockpit/.env.example"
    ok "  cockpit/ (excluding node_modules/, dist/, .env*, .stitch-mockups/)"
else
    warn "  cockpit/ — NOT FOUND"
fi

# --- Full directory copies ---
copy_dir "db"
copy_dir "docs"
copy_dir "internal/agentstream"
copy_dir "internal/api"
copy_dir "internal/audit"
copy_dir "internal/auth"
copy_dir "internal/database"
copy_dir "internal/dispatch"
copy_dir "internal/grpc"
copy_dir "internal/license"
copy_dir "internal/logging"
copy_dir "internal/metrics"
copy_dir "internal/notification"
copy_dir "internal/recording"
copy_dir "internal/router"
copy_dir "internal/secrets"
copy_dir "internal/session"
copy_dir "internal/stream"
copy_dir "internal/transport"
copy_dir "internal/satellite"
copy_dir "pkg"
copy_dir "proto"
copy_dir "tests"

# --- Single file copies ---
info "Copying whitelisted files..."

copy_file "deploy/nginx.conf"
copy_file "deploy/satellite-entrypoint.sh"
copy_file "scripts/install-satellite.sh"
copy_file "scripts/install-satellite.ps1"
copy_file "scripts/publish-to-public.sh"
copy_file "scripts/setup.sh"
copy_file "scripts/setup.ps1"
copy_file ".github/workflows/ci.yml"
# docker-compose.community.yml → docker-compose.yml (no HA infra for community)
cp "$PRIVATE_DIR/docker-compose.community.yml" "$PUBLIC_DIR/docker-compose.yml"
ok "  docker-compose.yml (from docker-compose.community.yml)"
# Dockerfile.nexus — strip embedded certs (community uses bind-mount) and license build arg
sed -e '/COPY cmd\/nexus\/certs/d' \
    -e '/# Copy certificates/d' \
    -e '/LICENSE_PUB_KEY/d' \
    -e '/# License public key/d' \
    -e '/# Build the nexus binary with embedded license/d' \
    -e 's/-ldflags="-s -w -X main.embeddedLicensePubKey=\${LICENSE_PUB_KEY}" //' \
    "$PRIVATE_DIR/Dockerfile.nexus" > "$PUBLIC_DIR/Dockerfile.nexus"
ok "  Dockerfile.nexus (stripped embedded certs + license arg)"
copy_file "Dockerfile.cockpit"
copy_file "Dockerfile.satellite"
copy_file "go.mod"
copy_file "go.sum"
copy_file "LICENSE"
copy_file "README.md"
copy_file "CONTRIBUTING.md"
copy_file "THIRD_PARTY_LICENSES.md"
# .env.community.example → .env.example (no OIDC for community)
cp "$PRIVATE_DIR/.env.community.example" "$PUBLIC_DIR/.env.example"
ok "  .env.example (from .env.community.example)"
copy_file ".gitattributes"
copy_file ".gitignore"
copy_file ".golangci.yml"
copy_file "vitest.config.ts"
copy_file "Makefile"

ok "Whitelist copy complete"

# ---------- Phase 3: Enterprise Stub Swap -----------------------------------

header "Phase 3: Enterprise Stub Swap"

ENTERPRISE_SRC="$PRIVATE_DIR/internal/enterprise"
ENTERPRISE_DST="$PUBLIC_DIR/internal/enterprise"

mkdir -p "$ENTERPRISE_DST/forge"
mkdir -p "$ENTERPRISE_DST/ha"
mkdir -p "$ENTERPRISE_DST/hitl"
mkdir -p "$ENTERPRISE_DST/secrets"

# Copy the license file
cp "$ENTERPRISE_SRC/LICENSE_ENTERPRISE" "$ENTERPRISE_DST/LICENSE_ENTERPRISE"
ok "  LICENSE_ENTERPRISE"

# Copy stubs from _public_stubs/ → enterprise subdirs
for pkg in forge ha hitl secrets; do
    local_stub="$ENTERPRISE_SRC/_public_stubs/$pkg/stub.go"
    if [[ -f "$local_stub" ]]; then
        cp "$local_stub" "$ENTERPRISE_DST/$pkg/stub.go"
        ok "  $pkg/stub.go ← _public_stubs/$pkg/stub.go"
    else
        fail "Stub not found: $local_stub"
    fi
done

ok "Enterprise stubs installed"

# ---------- Phase 4: Blacklist Safety Scan ----------------------------------

header "Phase 4: Blacklist Safety Scan"

VIOLATIONS=0

# Helper: check if a pattern exists in the public dir
blacklist_check() {
    local pattern="$1"
    local description="$2"
    local matches

    matches=$(cd "$PUBLIC_DIR" && find . -path './.git' -prune -o -name "$pattern" -print 2>/dev/null || true)
    if [[ -n "$matches" ]]; then
        echo -e "${RED}  ✗ FOUND $description:${NC}"
        echo "$matches" | sed 's/^/      /'
        VIOLATIONS=$((VIOLATIONS + 1))
    fi
}

# Helper: check if a path exists in the public dir
blacklist_path() {
    local path="$1"
    local description="$2"
    if [[ -e "$PUBLIC_DIR/$path" ]]; then
        echo -e "${RED}  ✗ FOUND $description: $path${NC}"
        VIOLATIONS=$((VIOLATIONS + 1))
    fi
}

info "Scanning for blacklisted files..."

# Private keys and certs
blacklist_check "*.pem"  "private key"
blacklist_check "*.key"  "private key"
blacklist_check "*.p12"  "private key"
blacklist_check "*.pfx"  "private key"

# .env (but not .env.example)
if find "$PUBLIC_DIR" -path "$PUBLIC_DIR/.git" -prune -o -name '.env' -not -name '.env.example' -not -name '.env.*' -print 2>/dev/null | grep -q .; then
    echo -e "${RED}  ✗ FOUND .env file (not .env.example)${NC}"
    find "$PUBLIC_DIR" -path "$PUBLIC_DIR/.git" -prune -o -name '.env' -not -name '.env.example' -not -name '.env.*' -print 2>/dev/null | sed 's/^/      /'
    VIOLATIONS=$((VIOLATIONS + 1))
fi

# Directories that must never appear
blacklist_path "dan_notes"               "internal strategy docs"
blacklist_path ".agents"                 "agent workflow configs"
blacklist_path ".pi"                     "Pi workspace"
blacklist_path ".stitch-mockups"         "design mockups"
blacklist_path ".claude"                 "Claude config"
blacklist_path "site"                    "marketing site"
blacklist_path "cmd/daao-keygen"         "license key generator"
blacklist_path "extensions"              "enterprise extension pack"
blacklist_path "config"                  "local config directory"
blacklist_path "bin"                     "local binaries"

# Specific files that must not appear
blacklist_path "docker-compose.ha.yml"           "HA compose config"
blacklist_path "docker-compose.enterprise.yml"   "enterprise compose config"
blacklist_path ".env.ha.example"                 "HA env example"
blacklist_path "deploy/nginx.ha.conf"            "HA nginx config"
blacklist_path "AGENTS.md"                       "internal agent instructions"
blacklist_path "FIX_AGENT_FORGE.md"              "internal notes"
blacklist_path "docs/SECURITY-PUBLIC.md"         "redundant — content merged into SECURITY.md"

# Real enterprise code (should only have stubs)
blacklist_path "internal/enterprise/forge/scheduler.go"             "real enterprise code"
blacklist_path "internal/enterprise/forge/analytics.go"             "real enterprise code"
blacklist_path "internal/enterprise/forge/pipeline_executor.go"     "real enterprise code"
blacklist_path "internal/enterprise/forge/forge_test.go"            "enterprise test"
blacklist_path "internal/enterprise/hitl/hitl.go"                   "real enterprise code"
blacklist_path "internal/enterprise/secrets/vault.go"               "real enterprise code"
blacklist_path "internal/enterprise/secrets/azure.go"               "real enterprise code"
blacklist_path "internal/enterprise/secrets/infisical.go"           "real enterprise code"
blacklist_path "internal/enterprise/secrets/openbao.go"             "real enterprise code"
blacklist_path "internal/enterprise/secrets/secrets_test.go"        "enterprise test"
blacklist_path "internal/enterprise/ha/factory.go"                  "real enterprise code"
blacklist_path "internal/enterprise/ha/doc.go"                      "real enterprise code"
blacklist_path "internal/enterprise/ha/leader_scheduler_guard.go"   "real enterprise code"
blacklist_path "internal/enterprise/ha/nats_run_event_hub.go"       "real enterprise code"
blacklist_path "internal/enterprise/ha/nats_stream_registry.go"     "real enterprise code"
blacklist_path "internal/enterprise/ha/redis_rate_limiter.go"       "real enterprise code"
blacklist_path "internal/enterprise/ha/s3_recording_pool.go"        "real enterprise code"
blacklist_path "internal/enterprise/_public_stubs"                  "stubs source directory"

# Compiled binaries
blacklist_check "*.exe"  "compiled binary"

# License files
blacklist_path "license.jwt"  "license JWT"
blacklist_path "license.pub"  "license public key"

if [[ $VIOLATIONS -gt 0 ]]; then
    fail "ABORTING: Found $VIOLATIONS blacklist violation(s). Fix the whitelist and try again."
fi

ok "Blacklist scan passed — zero violations"

# ---------- Phase 5: Build Verification -------------------------------------

header "Phase 5: Build Verification"

info "Running go build ./cmd/nexus/... in public dir..."

BUILD_OUTPUT=""
BUILD_OK=true
if BUILD_OUTPUT=$(cd "$PUBLIC_DIR" && go build ./cmd/nexus/... 2>&1); then
    ok "go build PASSED"
else
    BUILD_OK=false
    echo -e "${RED}go build FAILED:${NC}"
    echo "$BUILD_OUTPUT"
    echo ""
    fail "Build failed — enterprise stubs may need updating. Fix stubs and retry."
fi

# Clean up the built binary if it was produced
rm -f "$PUBLIC_DIR/nexus" "$PUBLIC_DIR/nexus.exe" 2>/dev/null || true

# ---------- Phase 6: Human Approval Gate ------------------------------------

header "Phase 6: Human Approval Gate"

# File listing
echo -e "${BOLD}Files that will be published:${NC}"
echo "────────────────────────────────────────"
(cd "$PUBLIC_DIR" && find . -path './.git' -prune -o -type f -print | sort | sed 's|^\./||')
echo "────────────────────────────────────────"

# Stats
FILE_COUNT=$(cd "$PUBLIC_DIR" && find . -path './.git' -prune -o -type f -print | wc -l | tr -d ' ')
TOTAL_SIZE=$(cd "$PUBLIC_DIR" && find . -path './.git' -prune -o -type f -print0 | xargs -0 du -ch 2>/dev/null | tail -1 | cut -f1)
echo ""
echo -e "${BOLD}Total files:${NC} $FILE_COUNT"
echo -e "${BOLD}Total size:${NC}  $TOTAL_SIZE"
echo -e "${BOLD}Build:${NC}       ${GREEN}PASSED${NC}"
echo -e "${BOLD}Blacklist:${NC}   ${GREEN}CLEAN${NC}"

# Git diff (if there are previous commits)
echo ""
(
    cd "$PUBLIC_DIR"
    if git log --oneline -1 >/dev/null 2>&1; then
        echo -e "${BOLD}Changes since last publish:${NC}"
        git add -A >/dev/null 2>&1
        git diff --cached --stat 2>/dev/null || echo "  (no previous commit to diff against)"
        git reset HEAD >/dev/null 2>&1 || true
    else
        echo -e "${BOLD}First publish — all files are new.${NC}"
    fi
)

echo ""
echo -e "${YELLOW}════════════════════════════════════════════════════════════${NC}"
echo -e "${YELLOW}  Review the file list above carefully.${NC}"
echo -e "${YELLOW}  This will FORCE PUSH to: $PUBLIC_REMOTE${NC}"
echo -e "${YELLOW}════════════════════════════════════════════════════════════${NC}"
echo ""

if [[ "${DAAO_DRY_RUN:-}" == "1" ]]; then
    warn "DRY RUN — skipping push. Set DAAO_DRY_RUN=0 or unset to push."
    exit 0
fi

read -r -p "Type 'yes' to commit and force-push, anything else to abort: " CONFIRM
if [[ "$CONFIRM" != "yes" ]]; then
    info "Aborted by user."
    exit 0
fi

# ---------- Phase 7: Commit & Push ------------------------------------------

header "Phase 7: Commit & Push"

TIMESTAMP=$(date -u '+%Y-%m-%d %H:%M:%S UTC')

# Prompt for a commit message
echo ""
echo -e "${BOLD}Enter a commit message for this publish:${NC}"
echo -e "${CYAN}(Leave blank to use default: 'chore: sync public release $TIMESTAMP')${NC}"
read -r -p "> " COMMIT_MSG
if [[ -z "$COMMIT_MSG" ]]; then
    COMMIT_MSG="chore: sync public release $TIMESTAMP"
fi

(
    cd "$PUBLIC_DIR"
    git add -A

    if git diff --cached --quiet; then
        warn "No changes to commit — public repo is already up to date."
    else
        git commit -m "$COMMIT_MSG"
        info "Committed: $COMMIT_MSG"
    fi
)

info "Pushing to $PUBLIC_REMOTE..."
(
    cd "$PUBLIC_DIR"
    git push origin main
)

ok "Push complete!"

# ---------- Phase 8: Post-Push Verification ---------------------------------

if [[ "${DAAO_SKIP_VERIFY:-}" == "1" ]]; then
    warn "Skipping post-push verification (DAAO_SKIP_VERIFY=1)"
    header "Done!"
    exit 0
fi

header "Phase 8: Post-Push Verification"

VERIFY_DIR="/tmp/daao-verify-$(date +%s)"
info "Cloning public repo into $VERIFY_DIR..."

git clone "$PUBLIC_REMOTE" "$VERIFY_DIR"

# Verify build
info "Verifying go build in clone..."
if (cd "$VERIFY_DIR" && go build ./cmd/nexus/... 2>&1); then
    ok "Post-push build PASSED"
else
    warn "Post-push build FAILED — check the public repo"
fi

# Verify no blacklisted files
info "Running blacklist scan on clone..."
CLONE_VIOLATIONS=0

check_clone() {
    if [[ -e "$VERIFY_DIR/$1" ]]; then
        echo -e "${RED}  ✗ FOUND in clone: $1${NC}"
        CLONE_VIOLATIONS=$((CLONE_VIOLATIONS + 1))
    fi
}

check_clone "cmd/nexus/certs"
check_clone "dan_notes"
check_clone ".agents"
check_clone ".pi"
check_clone ".env"
check_clone "AGENTS.md"
check_clone "internal/enterprise/forge/scheduler.go"
check_clone "internal/enterprise/hitl/hitl.go"
check_clone "internal/enterprise/secrets/vault.go"
check_clone "internal/enterprise/ha/factory.go"
check_clone "internal/enterprise/_public_stubs"

# Check for key files
if find "$VERIFY_DIR" -name '*.pem' -o -name '*.key' 2>/dev/null | grep -q .; then
    echo -e "${RED}  ✗ FOUND private key files in clone${NC}"
    CLONE_VIOLATIONS=$((CLONE_VIOLATIONS + 1))
fi

if [[ $CLONE_VIOLATIONS -gt 0 ]]; then
    warn "Post-push verification found $CLONE_VIOLATIONS issue(s)!"
else
    ok "Post-push verification PASSED — zero issues"
fi

# Verify commit count (should be exactly 1)
COMMIT_COUNT=$(cd "$VERIFY_DIR" && git rev-list --count HEAD)
if [[ "$COMMIT_COUNT" -eq 1 ]]; then
    ok "Commit count: 1 (orphan commit, no history)"
else
    warn "Commit count: $COMMIT_COUNT (expected 1 — history may be leaking!)"
fi

# Clean up
rm -rf "$VERIFY_DIR"
ok "Cleaned up verification clone"

# ---------- Done ------------------------------------------------------------

header "✅ Publish Complete!"

echo -e "${GREEN}The public repo has been updated at:${NC}"
echo -e "  ${BOLD}$PUBLIC_REMOTE${NC}"
echo ""
echo -e "Local public repo: ${BOLD}$PUBLIC_DIR${NC}"
echo ""
