FROM mcr.microsoft.com/devcontainers/go:1.27-bookworm

# A named volume whose target path does not exist in the image is created root-owned,
# and no-new-privileges leaves no sudo to repair it afterwards. Every volume mount point
# in devcontainer.json must appear here. See docs/findings.md, finding F8.
RUN mkdir -p /go/pkg/mod \
             /home/vscode/.cache/go-build \
             /home/vscode/.local/share/mise \
             /home/vscode/.pi/agent/npm \
             /home/vscode/.pi/agent/pi-claude-marketplace \
             /home/vscode/.config \
             /home/vscode/.history \
 && chown -R vscode:vscode /go/pkg /home/vscode

# Shims first, so tools are visible to NON-INTERACTIVE shells — which is how pi's bash
# tool runs commands, and where shell-init hooks such as `mise activate` never apply.
# This belongs here rather than in containerEnv: ${containerEnv:PATH} is not resolved
# inside containerEnv itself, and a value set here also survives a plain docker exec.
ENV PATH="/home/vscode/.local/share/mise/shims:${PATH}"
