#!/usr/bin/env bash
set -euo pipefail

SHOUT_URL="${SHOUT_URL:-http://shout:7100}"
MOCK_SLACK_URL="${MOCK_SLACK_URL:-http://mock-slack:8080}"
OPS_CREDS="${OPS_CREDS:-shouty:abouty}"
ADMIN_CREDS="${ADMIN_CREDS:-admin:sadmin}"

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

msg_count() {
  curl -sf "$MOCK_SLACK_URL/messages" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))"
}

# ── Wait for services ───────────────────────────────────────────
bold "Waiting for mock-slack..."
for i in $(seq 1 30); do
  if curl -sf "$MOCK_SLACK_URL/health" >/dev/null 2>&1; then
    green "  mock-slack ready"
    break
  fi
  [ "$i" -eq 30 ] && { red "mock-slack not ready"; exit 1; }
  sleep 1
done

bold "Waiting for shout..."
for i in $(seq 1 60); do
  if curl -sf "$SHOUT_URL/info" >/dev/null 2>&1; then
    green "  shout ready"
    break
  fi
  [ "$i" -eq 60 ] && { red "shout not ready"; exit 1; }
  sleep 2
done

# ── Clear any prior messages ────────────────────────────────────
curl -sf -X DELETE "$MOCK_SLACK_URL/messages" >/dev/null

# ── Load rules ──────────────────────────────────────────────────
bold "Step 1: Loading rules..."
RULES='((for *
      (when *
        (remind 5 minutes)
        (slack :webhook "http://mock-slack:8080/"
               :text "$topic is $status"
               :color (if ok? "good" "danger")
               :attach (if link
                           "$message <$link>"
                           "$message")))))'

response=$(curl -sf -u "$ADMIN_CREDS" -X POST "$SHOUT_URL/rules" \
  -H "Content-Type: text/plain" \
  -d "$RULES")
green "  Rules loaded"

# ── Baseline: establish initial "working" state (no notification) ─
bold "Step 2: Establishing baseline (working event, no notification expected)..."
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"test/pipeline","ok":true,"message":"build #41 passed","link":"http://ci.example.com/41"}' \
  >/dev/null

sleep 2
count=$(msg_count)
assert_eq "Baseline event does NOT trigger notification" "0" "$count"

# ── Test 1: Broken event → transition working→broken → notify ───
bold "Step 3: Sending BROKEN event (working → broken transition)..."
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"test/pipeline","ok":false,"message":"build #42 failed","link":"http://ci.example.com/42"}' \
  >/dev/null

sleep 2
count=$(msg_count)
assert_eq "Broken transition triggers notification" "1" "$count"

# ── Test 2: Second broken event (same topic) → no new notify ────
bold "Step 4: Sending second BROKEN event (no state change)..."
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"test/pipeline","ok":false,"message":"build #43 failed","link":"http://ci.example.com/43"}' \
  >/dev/null

sleep 2
count=$(msg_count)
assert_eq "Duplicate broken event does NOT trigger notification" "1" "$count"

# ── Test 3: Fixed event → transition broken→fixed → notify ──────
bold "Step 5: Sending FIXED event (broken → fixed transition)..."
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"test/pipeline","ok":true,"message":"build #44 passed","link":"http://ci.example.com/44"}' \
  >/dev/null

sleep 2
count=$(msg_count)
assert_eq "Fixed transition triggers notification" "2" "$count"

# ── Test 4: Working event (already ok) → no new notify ──────────
bold "Step 6: Sending WORKING event (no state change)..."
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"test/pipeline","ok":true,"message":"build #45 passed","link":"http://ci.example.com/45"}' \
  >/dev/null

sleep 2
count=$(msg_count)
assert_eq "Duplicate working event does NOT trigger notification" "2" "$count"

# ── Summary ─────────────────────────────────────────────────────
echo ""
bold "════════════════════════════════════════"
bold "  Results: $pass passed, $fail failed"
bold "════════════════════════════════════════"

# Show all received messages
echo ""
bold "All mock-slack messages received:"
curl -sf "$MOCK_SLACK_URL/messages" | python3 -m json.tool

exit "$fail"
