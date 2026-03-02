#!/usr/bin/env bash
set -euo pipefail

# SIGTERM graceful shutdown integration test
# Runs from the host — orchestrates docker compose to verify:
#   1. Shout persists state to disk on SIGTERM
#   2. State survives restart (reloaded from persisted DB)

SHOUT_URL="http://localhost:7100"
MOCK_SLACK_URL="http://localhost:8080"
OPS_CREDS="shouty:abouty"
ADMIN_CREDS="admin:sadmin"

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

wait_for_healthy() {
  local service="$1" url="$2" max_wait="${3:-60}"
  bold "Waiting for $service..."
  for i in $(seq 1 "$max_wait"); do
    if curl -sf "$url" >/dev/null 2>&1; then
      green "  $service ready"
      return 0
    fi
    sleep 2
  done
  red "  $service not ready after ${max_wait}s"
  return 1
}

state_count() {
  curl -sf -u "$OPS_CREDS" "$SHOUT_URL/states" \
    | python3 -c "import sys,json; print(len(json.load(sys.stdin)))"
}

cleanup() {
  bold "Cleaning up..."
  docker compose down -v 2>/dev/null || true
}

# ── Start fresh ─────────────────────────────────────────────────
bold "═══════════════════════════════════════════════"
bold "  SIGTERM Graceful Shutdown Integration Test"
bold "═══════════════════════════════════════════════"
echo ""

trap cleanup EXIT

cleanup
bold "Step 1: Starting services..."
docker compose up -d
wait_for_healthy "mock-slack" "$MOCK_SLACK_URL/health" 30
wait_for_healthy "shout" "$SHOUT_URL/info" 120

# ── Load rules + create state ──────────────────────────────────
bold "Step 2: Loading rules and creating state..."
RULES='((for *
      (when *
        (slack :webhook "http://mock-slack:8080/"
               :text "$topic is $status"))))'

curl -sf -u "$ADMIN_CREDS" -X POST "$SHOUT_URL/rules" \
  -H "Content-Type: text/plain" \
  -d "$RULES" >/dev/null

# Baseline event (establishes state, no notification)
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"sigterm/test","ok":true,"message":"build passed","link":"http://ci/1"}' \
  >/dev/null

sleep 2

# Broken event (creates a transition so state is meaningful)
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"sigterm/test","ok":false,"message":"build failed","link":"http://ci/2"}' \
  >/dev/null

sleep 2

# ── Capture state before shutdown ──────────────────────────────
bold "Step 3: Capturing state before shutdown..."
pre_count=$(state_count)
assert_eq "State exists before shutdown" "1" "$pre_count"

pre_states=$(curl -sf -u "$OPS_CREDS" "$SHOUT_URL/states")
pre_status=$(echo "$pre_states" | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['state'])")
assert_eq "State is broken before shutdown" "broken" "$pre_status"

# ── Send SIGTERM via docker compose stop ──────────────────────
bold "Step 4: Stopping shout (SIGTERM)..."
docker compose stop shout

# ── Check logs for graceful shutdown message ──────────────────
bold "Step 5: Checking shutdown logs..."
logs=$(docker compose logs shout 2>&1)
assert_contains "Log contains graceful shutdown message" "graceful shutdown complete" "$logs"

# ── Restart and verify state persisted ─────────────────────────
bold "Step 6: Restarting shout..."
docker compose start shout
wait_for_healthy "shout" "$SHOUT_URL/info" 120

bold "Step 7: Verifying state survived restart..."
post_count=$(state_count)
assert_eq "State count after restart" "$pre_count" "$post_count"

post_states=$(curl -sf -u "$OPS_CREDS" "$SHOUT_URL/states")
post_status=$(echo "$post_states" | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['state'])")
assert_eq "State is still broken after restart" "broken" "$post_status"

post_topic=$(echo "$post_states" | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['name'])")
assert_eq "Topic preserved after restart" "sigterm/test" "$post_topic"

# ── Summary ─────────────────────────────────────────────────────
echo ""
bold "════════════════════════════════════════"
bold "  Results: $pass passed, $fail failed"
bold "════════════════════════════════════════"

exit "$fail"
