#!/usr/bin/env sh
# install.sh — Install glitch and its core provider plugins from a release archive.
#
# Run from the directory where the archive was extracted:
#   ./install.sh
#
# Installs:
#   Binaries → ~/.local/bin  (or /usr/local/bin if writable)
#   Sidecar YAMLs → ~/.config/glitch/wrappers/

set -e

WRAPPERS_DIR="${HOME}/.config/glitch/wrappers"

# Determine binary install destination.
if [ -w /usr/local/bin ]; then
    BIN_DIR="/usr/local/bin"
else
    BIN_DIR="${HOME}/.local/bin"
fi

mkdir -p "${BIN_DIR}" "${WRAPPERS_DIR}"

# Install all plugin binaries found alongside this script.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

for binary in \
    glitch \
    glitch-claude \
    glitch-github-copilot \
    glitch-codex \
    glitch-gemini \
    glitch-opencode \
    glitch-ollama
do
    src="${SCRIPT_DIR}/${binary}"
    # On Windows the binary has a .exe extension; skip gracefully if absent.
    if [ ! -f "${src}" ] && [ -f "${src}.exe" ]; then
        src="${src}.exe"
        binary="${binary}.exe"
    fi
    if [ -f "${src}" ]; then
        install -m 0755 "${src}" "${BIN_DIR}/${binary}"
        echo "Installed ${binary} → ${BIN_DIR}/${binary}"
    fi
done

# Install sidecar YAML files from the wrappers/ subdirectory.
WRAPPERS_SRC="${SCRIPT_DIR}/wrappers"
if [ -d "${WRAPPERS_SRC}" ]; then
    for yaml in "${WRAPPERS_SRC}"/*.yaml; do
        [ -f "${yaml}" ] || continue
        name="$(basename "${yaml}")"
        install -m 0644 "${yaml}" "${WRAPPERS_DIR}/${name}"
        echo "Installed ${name} → ${WRAPPERS_DIR}/${name}"
    done
fi

echo ""
echo "Done. Make sure ${BIN_DIR} is on your PATH."
