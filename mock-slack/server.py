#!/usr/bin/env python3
"""Mock Slack webhook receiver for testing Shout! notifications.

Accepts POST requests (Slack-format JSON payloads), pretty-prints them
to stdout, and exposes GET /messages to retrieve all received messages
for programmatic verification.

POST /           - Receive a webhook payload (returns 200 OK)
GET  /messages   - Return JSON array of all received payloads
DELETE /messages - Clear all received payloads
"""

import json
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler
from datetime import datetime, timezone


messages = []


class SlackHandler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)

        try:
            payload = json.loads(body)
        except json.JSONDecodeError:
            payload = {"raw": body.decode("utf-8", errors="replace")}

        entry = {
            "received_at": datetime.now(timezone.utc).isoformat(),
            "path": self.path,
            "payload": payload,
        }
        messages.append(entry)

        print(f"\n{'='*60}", flush=True)
        print(f"[{entry['received_at']}] POST {self.path}", flush=True)
        print(json.dumps(payload, indent=2), flush=True)
        print(f"{'='*60}", flush=True)

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"ok":true}')

    def do_GET(self):
        if self.path == "/messages":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(messages).encode())
        elif self.path == "/health":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"ok":true}')
        else:
            self.send_response(404)
            self.end_headers()

    def do_DELETE(self):
        if self.path == "/messages":
            messages.clear()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"cleared":true}')
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        # Suppress default access log noise
        pass


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8080
    server = HTTPServer(("0.0.0.0", port), SlackHandler)
    print(f"Mock Slack listening on :{port}", flush=True)
    server.serve_forever()
