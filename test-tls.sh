#!/usr/bin/env bash
set -euo pipefail

# TLS integration test
# Runs from the host — starts Shout with a self-signed cert and verifies
# HTTPS serves requests correctly.
#
# Prerequisites: sbcl, openssl, curl, make quicklisp libs

PORT=7119
CERT_DIR=$(mktemp -d)
SHOUT_PID=""

pass=0
fail=0

green() { printf '\033[32m%s\033[0m\n' "$*"; }
red()   { printf '\033[31m%s\033[0m\n' "$*"; }
bold()  { printf '\033[1m%s\033[0m\n' "$*"; }

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    green "  PASS: $label (got $actual)"
    pass=$((pass + 1))
  else
    red "  FAIL: $label (expected $expected, got $actual)"
    fail=$((fail + 1))
  fi
}

assert_contains() {
  local label="$1" pattern="$2" haystack="$3"
  if echo "$haystack" | grep -q "$pattern"; then
    green "  PASS: $label"
    pass=$((pass + 1))
  else
    red "  FAIL: $label (pattern '$pattern' not found)"
    fail=$((fail + 1))
  fi
}

cleanup() {
  bold "Cleaning up..."
  if [ -n "$SHOUT_PID" ] && kill -0 "$SHOUT_PID" 2>/dev/null; then
    kill "$SHOUT_PID" 2>/dev/null || true
    wait "$SHOUT_PID" 2>/dev/null || true
  fi
  rm -rf "$CERT_DIR"
}
trap cleanup EXIT

# ── Generate self-signed cert ────────────────────────────────────
bold "══════════════════════════════════════════"
bold "  TLS Integration Test"
bold "══════════════════════════════════════════"
echo ""

bold "Step 1: Generating self-signed certificate..."
openssl req -x509 -newkey rsa:2048 \
  -keyout "$CERT_DIR/key.pem" \
  -out "$CERT_DIR/cert.pem" \
  -days 1 -nodes \
  -subj '/CN=localhost' 2>/dev/null
green "  Certificate generated in $CERT_DIR"

# ── Ensure version.lisp exists ───────────────────────────────────
make version.lisp 2>/dev/null

# ── Start Shout with TLS ─────────────────────────────────────────
bold "Step 2: Starting Shout with TLS on port $PORT..."
SHOUT_PORT=$PORT \
SHOUT_IT_OUT_LOUD=no \
SHOUT_TLS_CERT="$CERT_DIR/cert.pem" \
SHOUT_TLS_KEY="$CERT_DIR/key.pem" \
SHOUT_DATABASE="$CERT_DIR/shout.db" \
  sbcl --no-sysinit --no-userinit --script <(cat <<'LISP'
(load "build/quicklisp/setup.lisp")
(push (truename ".") asdf:*central-registry*)
(asdf:load-system :shout)
(shout:shout)
LISP
) &>"$CERT_DIR/shout.log" &
SHOUT_PID=$!

# Wait for Shout to start listening
for i in $(seq 1 30); do
  if curl -sk "https://localhost:$PORT/info" >/dev/null 2>&1; then
    green "  Shout ready (TLS)"
    break
  fi
  if ! kill -0 "$SHOUT_PID" 2>/dev/null; then
    red "  Shout process died during startup"
    echo "--- shout.log ---"
    cat "$CERT_DIR/shout.log"
    exit 1
  fi
  [ "$i" -eq 30 ] && { red "Shout not ready after 60s"; cat "$CERT_DIR/shout.log"; exit 1; }
  sleep 2
done

# ── Test 1: HTTPS /info returns valid JSON ────────────────────────
bold "Step 3: Testing HTTPS /info endpoint..."
response=$(curl -sk "https://localhost:$PORT/info")
assert_contains "HTTPS /info returns version field" '"version"' "$response"

# ── Test 2: HTTPS with auth works ─────────────────────────────────
bold "Step 4: Testing HTTPS with authentication..."
http_code=$(curl -sk -o /dev/null -w '%{http_code}' \
  -u "shout:shout" "https://localhost:$PORT/states")
assert_eq "HTTPS /states returns 200 with valid auth" "200" "$http_code"

# ── Test 3: Verify TLS is actually being used ─────────────────────
bold "Step 5: Verifying TLS certificate..."
cert_cn=$(echo | openssl s_client -connect "localhost:$PORT" 2>/dev/null \
  | openssl x509 -noout -subject 2>/dev/null | sed 's/.*CN *= *//')
assert_eq "TLS certificate CN matches" "localhost" "$cert_cn"

# ── Test 4: Plain HTTP should not work ────────────────────────────
bold "Step 6: Verifying plain HTTP is rejected..."
if curl -sf --max-time 3 "http://localhost:$PORT/info" >/dev/null 2>&1; then
  red "  FAIL: Plain HTTP should not work on TLS port"
  fail=$((fail + 1))
else
  green "  PASS: Plain HTTP correctly rejected"
  pass=$((pass + 1))
fi

# ── Test 5: Startup log indicates TLS ─────────────────────────────
bold "Step 7: Checking startup log for TLS indicator..."
log_output=$(cat "$CERT_DIR/shout.log")
assert_contains "Startup log shows (TLS)" "(TLS)" "$log_output"

# ── Summary ───────────────────────────────────────────────────────
echo ""
bold "════════════════════════════════════════"
bold "  Results: $pass passed, $fail failed"
bold "════════════════════════════════════════"

exit "$fail"
