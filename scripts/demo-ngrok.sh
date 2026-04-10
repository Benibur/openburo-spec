#!/usr/bin/env bash
# demo-ngrok.sh — expose the OpenBuro server via ngrok for a public demo.
#
# Prerequisites:
#   1. ngrok installed (see below)
#   2. ngrok authtoken configured (ngrok config add-authtoken YOUR_TOKEN)
#   3. config.yaml and credentials.yaml exist at repo root
#   4. Binary built: make build
#
# Install ngrok (Ubuntu/Debian):
#   curl -sSL https://ngrok-agent.s3.amazonaws.com/ngrok.asc \
#     | sudo tee /etc/apt/trusted.gpg.d/ngrok.asc >/dev/null \
#   && echo "deb https://ngrok-agent.s3.amazonaws.com buster main" \
#     | sudo tee /etc/apt/sources.list.d/ngrok.list \
#   && sudo apt update && sudo apt install ngrok
#
# Or via snap:
#   sudo snap install ngrok
#
# Or direct download (linux-amd64):
#   curl -sSL https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-linux-amd64.tgz \
#     | tar xz -C ~/.local/bin
#
# Authtoken (free tier is fine):
#   Sign up at https://dashboard.ngrok.com/signup
#   Get your token from https://dashboard.ngrok.com/get-started/your-authtoken
#   Run: ngrok config add-authtoken YOUR_TOKEN
#
# Usage:
#   ./scripts/demo-ngrok.sh

set -euo pipefail

PORT=8090
CONFIG="./config.yaml"
CREDS="./credentials.yaml"
BIN="./bin/openburo-server"

# Sanity checks
if ! command -v ngrok >/dev/null 2>&1; then
  echo "ERROR: ngrok is not installed. See the header of this script for install instructions." >&2
  exit 1
fi

if [ ! -f "$CONFIG" ]; then
  echo "ERROR: $CONFIG not found. Copy config.example.yaml to config.yaml and edit it." >&2
  exit 1
fi

if [ ! -f "$CREDS" ]; then
  echo "ERROR: $CREDS not found. Copy credentials.example.yaml to credentials.yaml and generate a bcrypt hash." >&2
  exit 1
fi

if [ ! -x "$BIN" ]; then
  echo "Building openburo-server..."
  make build
fi

# Start the server in the background
echo ">>> Starting OpenBuro server on :$PORT..."
"$BIN" -config "$CONFIG" > /tmp/openburo-demo.log 2>&1 &
SERVER_PID=$!

# Clean up on exit
cleanup() {
  echo ""
  echo ">>> Stopping server (PID $SERVER_PID)..."
  kill -TERM "$SERVER_PID" 2>/dev/null || true
  wait "$SERVER_PID" 2>/dev/null || true
  echo ">>> Server logs: /tmp/openburo-demo.log"
}
trap cleanup EXIT

# Wait for the server to bind
for i in 1 2 3 4 5 6 7 8 9 10; do
  if curl -sSf "http://localhost:$PORT/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done

if ! curl -sSf "http://localhost:$PORT/health" >/dev/null 2>&1; then
  echo "ERROR: Server never became healthy. Check /tmp/openburo-demo.log" >&2
  exit 1
fi

echo ">>> Server healthy. Starting ngrok tunnel..."
echo ""
echo "============================================================"
echo "  Once ngrok is connected:"
echo "    - Your public URL will be shown in the ngrok dashboard"
echo "      (look for 'Forwarding https://XXXX.ngrok-free.app')"
echo "    - Test with:"
echo "        curl https://XXXX.ngrok-free.app/health"
echo "        curl -u admin:demo-admin-2026 https://XXXX.ngrok-free.app/api/v1/registry"
echo "    - Press Ctrl+C to stop both ngrok and the server."
echo "============================================================"
echo ""

# Run ngrok in the foreground — Ctrl+C will trigger the trap and stop the server
ngrok http "$PORT"
