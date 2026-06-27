#!/usr/bin/env bash
# Record a real demo session with asciinema.
#
# Prerequisites:
#   - asciinema installed (pip install asciinema)
#   - sshforward binary in PATH
#   - ~/.sshforward/config.yaml configured
#   - SSH host accessible
#
# Usage:
#   chmod +x docs/demo/record.sh
#   docs/demo/record.sh <host> <service>
#
# Example:
#   docs/demo/record.sh myserver mysql
#
# The script records to docs/demo/demo-live.cast
# Upload: asciinema upload docs/demo/demo-live.cast

set -euo pipefail

HOST="${1:?Usage: $0 <host> <service>}"
SERVICE="${2:?Usage: $0 <host> <service>}"
CAST_FILE="docs/demo/demo-live.cast"

echo "Recording demo with host=$HOST service=$SERVICE"
echo "Press Ctrl+D when done."
echo ""

asciinema rec \
  --title "sshforward demo" \
  --cols 100 \
  --rows 30 \
  --command "bash -c '
    set -e
    echo \"# Show configuration\"
    sleep 1
    sshforward config
    sleep 2

    echo \"\"
    echo \"# Start tunnel\"
    sleep 1
    sshforward start $HOST $SERVICE
    sleep 2

    echo \"\"
    echo \"# List active tunnels\"
    sleep 1
    sshforward list
    sleep 3

    echo \"\"
    echo \"# View logs\"
    sleep 1
    sshforward logs $HOST $SERVICE
    sleep 2

    echo \"\"
    echo \"# Stop tunnel\"
    sleep 1
    sshforward stop $HOST $SERVICE
    sleep 2

    echo \"\"
    echo \"# Verify stopped\"
    sleep 1
    sshforward list
    sleep 2
  '" \
  "$CAST_FILE"

echo ""
echo "Saved to $CAST_FILE"
echo "Upload with: asciinema upload $CAST_FILE"
echo "Or embed with: https://asciinema.org"
