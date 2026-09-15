#!/usr/bin/env bash
# Installs the pi coding agent. Standalone by design: this file becomes the
# install.sh of a Dev Container Feature without modification.
set -euo pipefail

command -v npm >/dev/null || {
  echo "ERROR: node/npm not found. The node feature in devcontainer.json is required." >&2
  exit 1
}

npm install -g --ignore-scripts "@earendil-works/pi-coding-agent@${PI_VERSION:-latest}"
pi --version
