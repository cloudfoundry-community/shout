# Setting Up Slack App Notifications

Shout! supports two methods for sending Slack notifications:

1. **Webhooks** (`slack` handler) — Uses incoming webhook URLs. Works with both legacy custom integrations and Slack App webhooks.
2. **Web API** (`slack-app` handler) — Uses `chat.postMessage` with Bearer token auth. Supports posting to multiple channels from a single bot.

This guide covers setting up the `slack-app` handler (method 2).

## Why Use the Web API?

- **Multiple channels**: A single bot token can post to any channel it's invited to. Webhooks are locked to one channel per URL.
- **Future-proof**: Slack recommends the Web API over legacy webhooks.
- **Richer features**: Thread replies, message updates, and other API capabilities (future).

## Step 1: Create a Slack App

1. Go to [api.slack.com/apps](https://api.slack.com/apps)
2. Click **Create New App** → **From scratch**
3. Name your app (e.g., `shout-bot`) and select your workspace
4. Click **Create App**

## Step 2: Configure Bot Token Scopes

1. In the app settings, go to **OAuth & Permissions**
2. Scroll to **Bot Token Scopes**
3. Add the following scopes:
   - `chat:write` — Send messages as the bot
   - `chat:write.customize` — Customize the bot's username and icon per message

> **Note:** You do not need the `incoming-webhook` scope for the `slack-app` handler. That scope is only for the webhook-based `slack` handler.

## Step 3: Install the App to Your Workspace

1. Scroll to the top of **OAuth & Permissions**
2. Click **Install to Workspace**
3. Review the permissions and click **Allow**
4. Copy the **Bot User OAuth Token** — it starts with `xoxb-`

> **Important:** Use the `xoxb-` Bot User OAuth Token, not the User OAuth Token (`xoxp-`) or App Configuration Token (`xoxe.xoxp-`). Bot tokens do not expire and are scoped to the permissions you configured.

## Step 4: Invite the Bot to Channels

The bot can only post to channels it has been invited to:

```
/invite @shout-bot
```

Run this in each channel where you want Shout! to send notifications.

## Step 5: Configure Shout!

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `SHOUT_SLACK_TOKEN` | Yes | Bot User OAuth Token (`xoxb-...`) |
| `SHOUT_SLACK_CHANNEL` | Yes | Default channel (e.g., `#ci-alerts` or `C0123ABCDE`) |
| `SHOUT_BOTNAME` | No | Display name (default: `shout!bot`) |
| `SHOUT_BOTICON` | No | Icon URL for the bot avatar |

Set these in your deployment environment (BOSH manifest, Docker Compose, shell, etc.):

```bash
export SHOUT_SLACK_TOKEN="xoxb-your-token-here"
export SHOUT_SLACK_CHANNEL="#ci-alerts"
```

### Rules Configuration

#### Go (YAML)

```yaml
- slack-app:
    text: "{{ .Topic }} is {{ .Status }}"
    color: '{{ if .OK }}good{{ else }}danger{{ end }}'
    attach: "{{ .Message }}"
```

To override the default channel or token per rule:

```yaml
- slack-app:
    channel: "#ops-critical"
    token: "xoxb-different-token"
    text: "{{ .Topic }} is {{ .Status }}"
```

#### Lisp (DSL)

```lisp
(slack-app :text "$topic is $status"
           :color (if ok? "good" "danger")
           :attach "$message")
```

With per-rule channel override:

```lisp
(slack-app :channel "#ops-critical"
           :text "$topic is $status"
           :color (if ok? "good" "danger")
           :attach "$message")
```

## Channels

The `channel` parameter accepts:
- Channel names: `#general`, `#ci-alerts`
- Channel IDs: `C0123ABCDE` (found in channel details → copy link)

Channel IDs are more reliable — they survive channel renames.

## Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `missing_scope` | Bot token lacks required permissions | Add `chat:write` scope, reinstall app |
| `not_in_channel` | Bot hasn't been invited to the channel | Run `/invite @shout-bot` in the channel |
| `channel_not_found` | Invalid channel name or ID | Verify the channel exists and name is correct |
| `invalid_auth` | Wrong token type or expired token | Use the `xoxb-` Bot User OAuth Token |

## Verifying the Setup

Send a test event to Shout! and confirm the message appears in Slack:

```bash
# Post a test event
curl -u shout:shout \
  -H "Content-Type: application/json" \
  -d '{"topic":"test/slack-app","ok":false,"message":"Testing slack-app handler"}' \
  http://localhost:7109/events

# Post a recovery to trigger the "fixed" notification
curl -u shout:shout \
  -H "Content-Type: application/json" \
  -d '{"topic":"test/slack-app","ok":true,"message":"All clear"}' \
  http://localhost:7109/events
```

> **Remember:** Shout! tracks state transitions. The first event on a new topic establishes a baseline without notifying. Only state changes (working→broken, broken→fixed) trigger notifications. Send two events to see a notification.
