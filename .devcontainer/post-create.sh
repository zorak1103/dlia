#!/usr/bin/env bash
# Applies the personal configuration layer and installs declared tools.
set -euo pipefail

P=".devcontainer/.personal"

mkdir -p ~/.pi/agent ~/.pi/agent/skills ~/.config/mise "$PI_CODING_AGENT_SESSION_DIR"

# Copied, never mounted: the container keeps a writable copy and the host stays untouched.
# Refreshed on every create, so edits to the personal layer take effect on rebuild.
[ -f "$P/settings.json" ] && cp    "$P/settings.json" ~/.pi/agent/settings.json
[ -f "$P/models.json"   ] && cp    "$P/models.json"   ~/.pi/agent/models.json
[ -f "$P/mcp.json"      ] && cp    "$P/mcp.json"      ~/.pi/agent/mcp.json
[ -f "$P/claude-plugins.json" ] && cp "$P/claude-plugins.json" ~/.pi/agent/claude-plugins.json
[ -f "$P/AGENTS.md"     ] && cp    "$P/AGENTS.md"     ~/.pi/agent/AGENTS.md
[ -f "$P/mise.toml"     ] && cp    "$P/mise.toml"     ~/.config/mise/config.toml
[ -d "$P/skills"        ] && cp -r "$P/skills/."      ~/.pi/agent/skills/

# The copy above does not fetch anything it declares. pi only checks for missing packages
# at its own startup, so without this, packages in the personal settings.json would first
# get fetched whenever the developer types the first prompt (see findings.md, finding F14).
# No --approve needed: global packages carry no project-trust gate.
[ -f "$P/settings.json" ] && \
  { pi update --extensions || echo "WARNING: 'pi update --extensions' failed — packages from the personal layer may be missing"; }

# claude-plugins.json's marketplace/plugin clones happen at pi's own session-start hook,
# not at `pi update --extensions` above (see findings.md, finding F16). A throwaway
# non-interactive run triggers that hook eagerly; the model call itself is expected to
# fail here (no credentials needed for this) and is discarded. --offline skips pi's own
# startup network checks but does not block the hook's git clone; --no-session avoids
# leaving an empty session file behind.
[ -f "$P/claude-plugins.json" ] && \
  { pi --offline --no-session -p "noop" >/dev/null 2>&1 || true; }

[ -n "${ANTHROPIC_API_KEY:-}" ] || \
  echo "WARNING: ANTHROPIC_API_KEY is empty — see docs/setup-windows.md"

# Bind-mounted files appear as root-owned to the container user, so git refuses to touch
# the workspace ("dubious ownership"). That breaks language-level VCS stamping (Go's `go
# build`, for example) and the VS Code git integration. Register the workspace and its
# ancestors up to /workspaces — narrower than the usual wildcard, and it covers the case
# where the mount root is the repo root.
d="$PWD"
while [ "$d" != "/" ] && [ "$d" != "/workspaces" ]; do
  git config --global --add safe.directory "$d"
  d="$(dirname "$d")"
done

# A single unresolvable tool must not brick the container: the personal layer is
# hand-edited, and a typo there would otherwise leave no usable environment to fix it
# from. Fail loudly, carry on.
mise install || echo "WARNING: 'mise install' reported failures — see the output above; other tools are unaffected"
mise reshim
