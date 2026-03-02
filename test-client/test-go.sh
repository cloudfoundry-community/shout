#!/usr/bin/env bash
set -euo pipefail

SHOUT_URL="${SHOUT_URL:-http://shout:7109}"
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
for i in $(seq 1 30); do
  if curl -sf "$SHOUT_URL/info" >/dev/null 2>&1; then
    green "  shout ready"
    break
  fi
  [ "$i" -eq 30 ] && { red "shout not ready"; exit 1; }
  sleep 1
done

# ── Clear any prior messages ────────────────────────────────────
curl -sf -X DELETE "$MOCK_SLACK_URL/messages" >/dev/null

# ── Test 1: GET /info returns version ───────────────────────────
bold "Test 1: GET /info returns version"
version=$(curl -sf "$SHOUT_URL/info" | python3 -c "import sys,json; print(json.load(sys.stdin).get('version',''))")
if [ -n "$version" ]; then
  green "  PASS: version=$version"
  pass=$((pass + 1))
else
  red "  FAIL: no version in /info"
  fail=$((fail + 1))
fi

# ── Test 2: Baseline working event → 0 notifications ───────────
bold "Test 2: Baseline working event (no notification)"
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"test/pipeline","ok":true,"message":"build #41 passed","link":"http://ci.example.com/41"}' \
  >/dev/null
sleep 1
count=$(msg_count)
assert_eq "no notification on baseline" "0" "$count"

# ── Test 3: Working→broken → +1 notification ───────────────────
bold "Test 3: Working→broken transition"
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"test/pipeline","ok":false,"message":"build #42 failed","link":"http://ci.example.com/42"}' \
  >/dev/null
sleep 1
count=$(msg_count)
assert_eq "broken notification" "1" "$count"

# ── Test 4: Duplicate broken → still 1 ─────────────────────────
bold "Test 4: Duplicate broken (no new notification)"
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"test/pipeline","ok":false,"message":"build #43 failed"}' \
  >/dev/null
sleep 1
count=$(msg_count)
assert_eq "no notification on duplicate broken" "1" "$count"

# ── Test 5: Broken→fixed → +1 notification ─────────────────────
bold "Test 5: Broken→fixed transition"
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"test/pipeline","ok":true,"message":"build #44 passed"}' \
  >/dev/null
sleep 1
count=$(msg_count)
assert_eq "fixed notification" "2" "$count"

# ── Test 6: Duplicate fixed → still 2 ──────────────────────────
bold "Test 6: Duplicate fixed (no new notification)"
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"test/pipeline","ok":true,"message":"build #45 passed"}' \
  >/dev/null
sleep 1
count=$(msg_count)
assert_eq "no notification on duplicate fixed" "2" "$count"

# ── Test 7: GET /rules returns YAML ─────────────────────────────
bold "Test 7: GET /rules returns YAML"
content_type=$(curl -sf -u "$ADMIN_CREDS" -o /dev/null -w '%{content_type}' "$SHOUT_URL/rules")
assert_eq "rules content type" "text/x-yaml" "$content_type"

# ── Test 8: POST /rules updates engine ──────────────────────────
bold "Test 8: POST /rules updates engine"
NEW_RULES='rules:
  - for: "*"
    when:
      - match: "*"
        do:
          - slack:
              webhook: "http://mock-slack:8080/"
              text: "updated: {{ .Topic }} is {{ .Status }}"
'
status=$(curl -sf -u "$ADMIN_CREDS" -X POST "$SHOUT_URL/rules" \
  -H "Content-Type: text/x-yaml" \
  -d "$NEW_RULES" \
  -o /dev/null -w '%{http_code}')
assert_eq "POST /rules status" "200" "$status"

# ── Test 9: GET /state returns topic ────────────────────────────
bold "Test 9: GET /state returns topic"
state_name=$(curl -sf -u "$OPS_CREDS" "$SHOUT_URL/state?topic=test/pipeline" | \
  python3 -c "import sys,json; print(json.load(sys.stdin).get('name',''))")
assert_eq "state topic name" "test/pipeline" "$state_name"

# ── Test 10: GET /states returns all ────────────────────────────
bold "Test 10: GET /states returns all topics"
topic_count=$(curl -sf -u "$OPS_CREDS" "$SHOUT_URL/states" | \
  python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
if [ "$topic_count" -ge 1 ]; then
  green "  PASS: states has $topic_count topic(s)"
  pass=$((pass + 1))
else
  red "  FAIL: expected at least 1 topic, got $topic_count"
  fail=$((fail + 1))
fi

# ── Test 11: POST /announcements triggers notification ──────────
bold "Test 11: POST /announcements"
curl -sf -X DELETE "$MOCK_SLACK_URL/messages" >/dev/null
curl -sf -u "$OPS_CREDS" -X POST "$SHOUT_URL/announcements" \
  -H "Content-Type: application/json" \
  -d '{"topic":"deploy","message":"deploying v2.0"}' \
  >/dev/null
sleep 1
count=$(msg_count)
assert_eq "announcement notification" "1" "$count"

# ── Test 12: Auth required (401 without creds) ──────────────────
bold "Test 12: Auth required"
status=$(curl -sf -o /dev/null -w '%{http_code}' -X POST "$SHOUT_URL/events" \
  -H "Content-Type: application/json" \
  -d '{"topic":"unauth","ok":true}' 2>/dev/null || echo "401")
assert_eq "401 without credentials" "401" "$status"

# ── Summary ─────────────────────────────────────────────────────
echo ""
bold "═══════════════════════════════════════"
bold "Results: $pass passed, $fail failed"
bold "═══════════════════════════════════════"

if [ "$fail" -gt 0 ]; then
  exit 1
fi
