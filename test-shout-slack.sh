#!/usr/bin/env bash
# test-shout-slack.sh — Curl-based integration test for Shout! + Slack
#
# Usage:
#   ./test-shout-slack.sh                              # Runs all handlers
#   ./test-shout-slack.sh --test webhook               # Webhook handler only
#   ./test-shout-slack.sh --test slack-app              # Slack App handler only
#   ./test-shout-slack.sh --test webhook,slack-app      # Both (same as default)
#   ./test-shout-slack.sh --webhook <slack-url>         # Real Slack webhook
#   ./test-shout-slack.sh --token <xoxb-...> --channel <#chan>  # Real Slack Web API
#   ./test-shout-slack.sh --shout-url <url>             # Custom Shout! URL (default: localhost:7109)
#
# Environment:
#   SHOUT_SLACK_API_URL - Override Slack API URL (set to mock-slack for mock mode)
#
# Prerequisites:
#   - Shout! running (e.g. sbcl --script run.lisp, or docker compose up)
#   - For mock mode: mock-slack running (python3 mock-slack/server.py)
#
set -euo pipefail

# ── Defaults ─────────────────────────────────────────────────────
SHOUT_URL="${SHOUT_URL:-http://localhost:7109}"
SHOUT_USER="${SHOUT_USER:-shout}"
SHOUT_PASS="${SHOUT_PASS:-shout}"
WEBHOOK="${WEBHOOK:-http://localhost:8080}"
SLACK_TOKEN="${SHOUT_SLACK_TOKEN:-}"
SLACK_CHANNEL="${SHOUT_SLACK_CHANNEL:-}"
MOCK_MODE=true
RUN_TESTS=""  # empty = all
PASSED=0
FAILED=0

# ── Parse args ───────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --test)       RUN_TESTS="$2"; shift 2 ;;
    --webhook)    WEBHOOK="$2"; MOCK_MODE=false; shift 2 ;;
    --token)      SLACK_TOKEN="$2"; shift 2 ;;
    --channel)    SLACK_CHANNEL="$2"; shift 2 ;;
    --shout-url)  SHOUT_URL="$2"; shift 2 ;;
    --user)       SHOUT_USER="$2"; shift 2 ;;
    --pass)       SHOUT_PASS="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,17p' "$0" | sed 's/^# \?//'
      exit 0 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

AUTH="${SHOUT_USER}:${SHOUT_PASS}"

# ── Test selection ───────────────────────────────────────────────
should_run() {
  [[ -z "$RUN_TESTS" ]] && return 0
  echo ",$RUN_TESTS," | grep -q ",$1,"
}

# ── Helpers ──────────────────────────────────────────────────────
bold()  { printf '\033[1m%s\033[0m\n' "$*"; }
green() { printf '\033[32m  PASS: %s\033[0m\n' "$*"; }
red()   { printf '\033[31m  FAIL: %s\033[0m\n' "$*"; }

pass() { green "$1"; PASSED=$((PASSED + 1)); }
fail() { red "$1"; FAILED=$((FAILED + 1)); }

assert_eq() {
  local desc="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    pass "$desc (got $actual)"
  else
    fail "$desc (expected $expected, got $actual)"
  fi
}

assert_contains() {
  local desc="$1" needle="$2" haystack="$3"
  if echo "$haystack" | grep -q "$needle"; then
    pass "$desc"
  else
    fail "$desc (expected to contain '$needle')"
  fi
}

assert_http() {
  local desc="$1" expected_code="$2" actual_code="$3"
  assert_eq "$desc" "$expected_code" "$actual_code"
}

mock_message_count() {
  curl -sf "${WEBHOOK}/messages" 2>/dev/null | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0"
}

mock_clear() {
  curl -sf -X DELETE "${WEBHOOK}/messages" >/dev/null 2>&1 || true
}

mock_last_message() {
  curl -sf "${WEBHOOK}/messages" 2>/dev/null | python3 -c "
import sys, json
msgs = json.load(sys.stdin)
if msgs:
    print(json.dumps(msgs[-1], indent=2))
else:
    print('{}')
" 2>/dev/null || echo "{}"
}

# ── Preflight ────────────────────────────────────────────────────
bold "Shout! Slack Integration Test"
echo "  Shout URL:  $SHOUT_URL"
echo "  Webhook:    $WEBHOOK"
echo "  Token:      ${SLACK_TOKEN:+set (${SLACK_TOKEN:0:12}...)}${SLACK_TOKEN:-not set}"
echo "  Channel:    ${SLACK_CHANNEL:-not set}"
echo "  Mock mode:  $MOCK_MODE"
echo "  Tests:      ${RUN_TESTS:-all}"
echo ""

bold "Preflight: checking Shout! is reachable..."
HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' "${SHOUT_URL}/info" 2>/dev/null || echo "000")
if [[ "$HTTP_CODE" != "200" ]]; then
  red "Shout! not reachable at ${SHOUT_URL}/info (HTTP $HTTP_CODE)"
  echo "  Start Shout! first: sbcl --script run.lisp"
  exit 1
fi
green "Shout! is up"

if $MOCK_MODE; then
  bold "Preflight: checking mock-slack is reachable..."
  HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' "${WEBHOOK}/messages" 2>/dev/null || echo "000")
  if [[ "$HTTP_CODE" != "200" ]]; then
    red "mock-slack not reachable at ${WEBHOOK}/messages (HTTP $HTTP_CODE)"
    echo "  Start it: python3 mock-slack/server.py &"
    exit 1
  fi
  green "mock-slack is up"
  mock_clear
fi

echo ""

# ── Test 1: GET /info ────────────────────────────────────────────
bold "Test 1: GET /info"
BODY=$(curl -sf "${SHOUT_URL}/info")
assert_contains "/info returns JSON with version" "version" "$BODY"

# ── Test 2: Auth required ────────────────────────────────────────
bold "Test 2: Auth required for /events"
HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' -X POST "${SHOUT_URL}/events" \
  -H 'Content-Type: application/json' \
  -d '{"topic":"auth-test","ok":true,"message":"no auth"}' 2>/dev/null)
assert_eq "POST /events without auth returns 401" "401" "$HTTP_CODE"

# ── Test 3: Bad credentials ─────────────────────────────────────
bold "Test 3: Bad credentials rejected"
HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' -u "wrong:creds" -X POST "${SHOUT_URL}/events" \
  -H 'Content-Type: application/json' \
  -d '{"topic":"auth-test","ok":true,"message":"bad creds"}' 2>/dev/null)
if [[ "$HTTP_CODE" == "401" || "$HTTP_CODE" == "403" ]]; then
  pass "POST /events with bad creds rejected (got $HTTP_CODE)"
else
  fail "POST /events with bad creds rejected (expected 401 or 403, got $HTTP_CODE)"
fi

# ── Detect Lisp vs Go ────────────────────────────────────────────
# Go /info returns "release":"Megaphone", Lisp returns "release":"Whisper"
INFO_BODY=$(curl -sf "${SHOUT_URL}/info")
if echo "$INFO_BODY" | grep -q "Megaphone"; then
  ENGINE="go"
else
  ENGINE="lisp"
fi
bold "Detected engine: $ENGINE"
echo ""

# ══════════════════════════════════════════════════════════════════
# PART A: Webhook-based slack handler (old method)
# ══════════════════════════════════════════════════════════════════
if should_run webhook; then
bold "══════════════════════════════════════════"
bold "Part A: Webhook-based slack handler"
bold "══════════════════════════════════════════"
echo ""

# ── Test 4: Load webhook rules ───────────────────────────────────
bold "Test 4: POST /rules (webhook)"
if [[ "$ENGINE" == "go" ]]; then
  RULES='rules:
  - for: "*"
    when:
      - match: "*"
        do:
          - slack:
              webhook: "'${WEBHOOK}'"
              text: "[WEBHOOK] {{ .Topic }} is {{ .Status }}"
              color: '"'"'{{ if .OK }}good{{ else }}danger{{ end }}'"'"'
              attach: "{{ .Message }}"'
  RULES_CHECK="rules"
else
  RULES='((for *
  (when *
    (slack :webhook "'${WEBHOOK}'"
           :text "[WEBHOOK] $topic is $status"
           :attach "$message"
           :color (if :ok? "good" "danger")))))'
  RULES_CHECK="for"
fi

HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' -u "$AUTH" -X POST "${SHOUT_URL}/rules" \
  --data-binary "$RULES")
assert_eq "POST /rules (webhook) succeeds" "200" "$HTTP_CODE"

# ── Test 5: GET /rules ──────────────────────────────────────────
bold "Test 5: GET /rules returns loaded rules"
RULES_BACK=$(curl -sf -u "$AUTH" "${SHOUT_URL}/rules")
assert_contains "GET /rules returns rule content" "$RULES_CHECK" "$RULES_BACK"

# ── Test 6: Baseline event (no notification) ─────────────────────
bold "Test 6: Baseline event (working, establishes state, no notification)"
HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' -u "$AUTH" -X POST "${SHOUT_URL}/events" \
  -H 'Content-Type: application/json' \
  -d '{"topic":"test/webhook","ok":true,"message":"build #1 passed","link":"http://ci.example.com/1"}')
assert_eq "POST /events baseline returns 200" "200" "$HTTP_CODE"

if $MOCK_MODE; then
  sleep 0.5
  COUNT=$(mock_message_count)
  assert_eq "Baseline event does NOT trigger notification" "0" "$COUNT"
fi

# ── Test 7: Broken event (working → broken transition) ──────────
bold "Test 7: Broken event (working → broken triggers notification)"
HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' -u "$AUTH" -X POST "${SHOUT_URL}/events" \
  -H 'Content-Type: application/json' \
  -d '{"topic":"test/webhook","ok":false,"message":"build #2 failed","link":"http://ci.example.com/2"}')
assert_eq "POST /events broken returns 200" "200" "$HTTP_CODE"

if $MOCK_MODE; then
  sleep 0.5
  COUNT=$(mock_message_count)
  assert_eq "Broken transition triggers notification" "1" "$COUNT"
  LAST=$(mock_last_message)
  assert_contains "Notification mentions topic" "test/webhook" "$LAST"
  assert_contains "Notification has danger color" "danger" "$LAST"
  assert_contains "Notification uses WEBHOOK tag" "WEBHOOK" "$LAST"
fi

# ── Test 8: Duplicate broken (no new notification) ──────────────
bold "Test 8: Duplicate broken event (no state change, no notification)"
curl -sf -u "$AUTH" -X POST "${SHOUT_URL}/events" \
  -H 'Content-Type: application/json' \
  -d '{"topic":"test/webhook","ok":false,"message":"build #3 also failed","link":"http://ci.example.com/3"}' >/dev/null

if $MOCK_MODE; then
  sleep 0.5
  COUNT=$(mock_message_count)
  assert_eq "Duplicate broken does NOT trigger notification" "1" "$COUNT"
fi

# ── Test 9: Fixed event (broken → fixed transition) ─────────────
bold "Test 9: Fixed event (broken → fixed triggers notification)"
HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' -u "$AUTH" -X POST "${SHOUT_URL}/events" \
  -H 'Content-Type: application/json' \
  -d '{"topic":"test/webhook","ok":true,"message":"build #4 passed","link":"http://ci.example.com/4"}')
assert_eq "POST /events fixed returns 200" "200" "$HTTP_CODE"

if $MOCK_MODE; then
  sleep 0.5
  COUNT=$(mock_message_count)
  assert_eq "Fixed transition triggers notification" "2" "$COUNT"
  LAST=$(mock_last_message)
  assert_contains "Notification mentions fixed" "fixed" "$LAST"
  assert_contains "Notification has good color" "good" "$LAST"
fi

# ── Test 10: Announcement ───────────────────────────────────────
bold "Test 10: POST /announcements (webhook)"
if $MOCK_MODE; then
  mock_clear
fi

HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' -u "$AUTH" -X POST "${SHOUT_URL}/announcements" \
  -H 'Content-Type: application/json' \
  -d '{"topic":"deploy/prod","ok":true,"message":"v2.0 deployed to production","link":"http://ci.example.com/deploy/42"}')
assert_eq "POST /announcements returns 200" "200" "$HTTP_CODE"

if $MOCK_MODE; then
  sleep 0.5
  COUNT=$(mock_message_count)
  assert_eq "Announcement triggers notification" "1" "$COUNT"
fi

# ── Test 11: Multiple topics ────────────────────────────────────
bold "Test 11: Independent topics have independent state"
if $MOCK_MODE; then
  mock_clear
fi

# Baseline for a new topic
curl -sf -u "$AUTH" -X POST "${SHOUT_URL}/events" \
  -H 'Content-Type: application/json' \
  -d '{"topic":"test/other","ok":true,"message":"other build ok"}' >/dev/null

# Break the new topic
curl -sf -u "$AUTH" -X POST "${SHOUT_URL}/events" \
  -H 'Content-Type: application/json' \
  -d '{"topic":"test/other","ok":false,"message":"other build broken"}' >/dev/null

if $MOCK_MODE; then
  sleep 0.5
  COUNT=$(mock_message_count)
  assert_eq "Second topic broken triggers its own notification" "1" "$COUNT"
  LAST=$(mock_last_message)
  assert_contains "Notification is for the right topic" "test/other" "$LAST"
fi

fi  # end webhook

# ══════════════════════════════════════════════════════════════════
# PART B: Web API slack-app handler (new method)
# ══════════════════════════════════════════════════════════════════

if should_run slack-app; then
# In mock mode, use mock-slack as the API endpoint.
# In real mode, need --token and --channel.
SKIP_SLACK_APP=false

if $MOCK_MODE; then
  # Mock mode: point slack-app at mock-slack
  SLACK_API_URL="${WEBHOOK}/api/chat.postMessage"
  SLACK_TOKEN="${SLACK_TOKEN:-mock-token}"
  SLACK_CHANNEL="${SLACK_CHANNEL:-#mock-channel}"
elif [[ -z "$SLACK_TOKEN" || -z "$SLACK_CHANNEL" ]]; then
  bold ""
  bold "Skipping Part B (slack-app): --token and --channel required for real mode"
  SKIP_SLACK_APP=true
else
  SLACK_API_URL="https://slack.com/api/chat.postMessage"
fi

if ! $SKIP_SLACK_APP; then
  echo ""
  bold "══════════════════════════════════════════"
  bold "Part B: Web API slack-app handler"
  bold "══════════════════════════════════════════"
  echo ""

  if $MOCK_MODE; then
    mock_clear
  fi

  # ── Test 12: Load slack-app rules ──────────────────────────────
  bold "Test 12: POST /rules (slack-app)"
  if [[ "$ENGINE" == "go" ]]; then
    RULES='rules:
  - for: "*"
    when:
      - match: "*"
        do:
          - slack-app:
              token: "'${SLACK_TOKEN}'"
              channel: "'${SLACK_CHANNEL}'"
              text: "[SLACK-APP] {{ .Topic }} is {{ .Status }}"
              color: '"'"'{{ if .OK }}good{{ else }}danger{{ end }}'"'"'
              attach: "{{ .Message }}"'
  else
    RULES='((for *
  (when *
    (slack-app :token "'${SLACK_TOKEN}'"
               :channel "'${SLACK_CHANNEL}'"
               :text "[SLACK-APP] $topic is $status"
               :attach "$message"
               :color (if :ok? "good" "danger")))))'
  fi

  HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' -u "$AUTH" -X POST "${SHOUT_URL}/rules" \
    --data-binary "$RULES")
  assert_eq "POST /rules (slack-app) succeeds" "200" "$HTTP_CODE"

  # ── Test 13: Baseline event (no notification) ──────────────────
  bold "Test 13: Baseline event (slack-app, establishes state)"
  HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' -u "$AUTH" -X POST "${SHOUT_URL}/events" \
    -H 'Content-Type: application/json' \
    -d '{"topic":"test/slackapp","ok":true,"message":"build #1 passed","link":"http://ci.example.com/sa/1"}')
  assert_eq "POST /events baseline returns 200" "200" "$HTTP_CODE"

  if $MOCK_MODE; then
    sleep 0.5
    COUNT=$(mock_message_count)
    assert_eq "Baseline does NOT trigger notification" "0" "$COUNT"
  fi

  # ── Test 14: Broken event ──────────────────────────────────────
  bold "Test 14: Broken event (slack-app, triggers notification)"
  HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' -u "$AUTH" -X POST "${SHOUT_URL}/events" \
    -H 'Content-Type: application/json' \
    -d '{"topic":"test/slackapp","ok":false,"message":"build #2 failed","link":"http://ci.example.com/sa/2"}')
  assert_eq "POST /events broken returns 200" "200" "$HTTP_CODE"

  if $MOCK_MODE; then
    sleep 0.5
    COUNT=$(mock_message_count)
    assert_eq "Broken transition triggers notification" "1" "$COUNT"
    LAST=$(mock_last_message)
    assert_contains "Notification mentions topic" "test/slackapp" "$LAST"
    assert_contains "Notification has danger color" "danger" "$LAST"
    assert_contains "Notification uses SLACK-APP tag" "SLACK-APP" "$LAST"
    assert_contains "Payload includes channel" "channel" "$LAST"
  fi

  # ── Test 15: Duplicate broken (no new notification) ────────────
  bold "Test 15: Duplicate broken (slack-app, no new notification)"
  curl -sf -u "$AUTH" -X POST "${SHOUT_URL}/events" \
    -H 'Content-Type: application/json' \
    -d '{"topic":"test/slackapp","ok":false,"message":"build #3 also failed"}' >/dev/null

  if $MOCK_MODE; then
    sleep 0.5
    COUNT=$(mock_message_count)
    assert_eq "Duplicate broken does NOT trigger notification" "1" "$COUNT"
  fi

  # ── Test 16: Fixed event ───────────────────────────────────────
  bold "Test 16: Fixed event (slack-app, triggers notification)"
  HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' -u "$AUTH" -X POST "${SHOUT_URL}/events" \
    -H 'Content-Type: application/json' \
    -d '{"topic":"test/slackapp","ok":true,"message":"build #4 passed","link":"http://ci.example.com/sa/4"}')
  assert_eq "POST /events fixed returns 200" "200" "$HTTP_CODE"

  if $MOCK_MODE; then
    sleep 0.5
    COUNT=$(mock_message_count)
    assert_eq "Fixed transition triggers notification" "2" "$COUNT"
    LAST=$(mock_last_message)
    assert_contains "Notification mentions fixed" "fixed" "$LAST"
    assert_contains "Notification has good color" "good" "$LAST"
  fi

  # ── Test 17: Announcement via slack-app ────────────────────────
  bold "Test 17: POST /announcements (slack-app)"
  if $MOCK_MODE; then
    mock_clear
  fi

  HTTP_CODE=$(curl -sf -o /dev/null -w '%{http_code}' -u "$AUTH" -X POST "${SHOUT_URL}/announcements" \
    -H 'Content-Type: application/json' \
    -d '{"topic":"deploy/staging","ok":true,"message":"v3.0 deployed to staging","link":"http://ci.example.com/deploy/99"}')
  assert_eq "POST /announcements (slack-app) returns 200" "200" "$HTTP_CODE"

  if $MOCK_MODE; then
    sleep 0.5
    COUNT=$(mock_message_count)
    assert_eq "Announcement triggers notification" "1" "$COUNT"
    LAST=$(mock_last_message)
    assert_contains "Announcement payload includes channel" "channel" "$LAST"
  fi
fi
fi  # end slack-app

# ── Results ──────────────────────────────────────────────────────
echo ""
bold "════════════════════════════════════════"
if [[ $FAILED -eq 0 ]]; then
  bold "  Results: $PASSED passed, 0 failed"
else
  bold "  Results: $PASSED passed, $FAILED failed"
fi
bold "════════════════════════════════════════"

if $MOCK_MODE; then
  echo ""
  bold "All mock-slack messages received:"
  curl -sf "${WEBHOOK}/messages" | python3 -m json.tool 2>/dev/null || echo "  (could not fetch)"
fi

exit $FAILED
