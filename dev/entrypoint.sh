#!/bin/sh
set -e

# The agent and the panel, in one container, talking over the socket exactly
# as they do on a server. The demo agent keeps its state in memory: restarting
# the container starts the demo over.
SOCKET=/run/croft/agent.sock
DB=/var/lib/croft/croft.db
mkdir -p /run/croft /var/lib/croft

croft agent --demo --socket "$SOCKET" --pause "${CROFT_PAUSE:-400ms}" &

for _ in $(seq 1 50); do [ -S "$SOCKET" ] && break; sleep 0.1; done

if ! croft user list --db "$DB" | grep -q '^demo '; then
  croft user add demo --db "$DB" --password demo-password >/dev/null
fi

echo ""
echo "  OpenCroft demo on http://localhost:${CROFT_PORT:-8080}"
echo "  user: demo   password: demo-password"
echo ""

exec croft serve --addr :8080 --agent "$SOCKET" --db "$DB"
