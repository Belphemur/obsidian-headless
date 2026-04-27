#!/bin/sh
# Interactive helper: logs in to Obsidian as the obsidian user.
# The auth token is stored in the OS keyring / encrypted store and cannot
# be retrieved as a plain file.
#
# Usage (override entrypoint so s6-overlay services are not started):
#   docker run --rm -it --entrypoint get-token <image>

set -e

# HOME is set by the Dockerfile ENV; this fallback ensures the script works
# when run outside the container (e.g. during development).
export HOME="${HOME:-/home/obsidian}"

echo ""
echo "=== obsidian-headless: Login ==="
echo ""
echo "Log in to your Obsidian account."
echo "You will be prompted for your email, password, and MFA code (if enabled)."
echo ""

# Run login as the obsidian user so the token is stored in the right place
s6-setuidgid obsidian ob login

echo ""
echo "Login successful. The auth token has been stored securely."
echo "You can now run the container normally with your credentials persisted in the config volume."
echo ""
