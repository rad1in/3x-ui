#!/bin/bash
# One-command node install. Installs 3x-ui without prompts, then registers this
# server on the master panel that issued the join command (Nodes > Add node >
# Install command). Needs XUI_JOIN_URL and XUI_JOIN_TOKEN; the master prints both.

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

fail() {
    echo -e "${red}$*${plain}" >&2
    exit 1
}

: "${XUI_JOIN_URL:?XUI_JOIN_URL is not set; copy the whole command from the master panel}"
: "${XUI_JOIN_TOKEN:?XUI_JOIN_TOKEN is not set; copy the whole command from the master panel}"

[[ $EUID -ne 0 ]] && fail "Please run this script as root."
command -v curl > /dev/null 2>&1 || fail "curl is required."

installer_url="${XUI_INSTALLER_URL:-https://raw.githubusercontent.com/rad1in/eternal/main/install.sh}"
result_file="${XUI_RESULT_FILE:-/etc/eternal/install-result.env}"
node_name="${XUI_NODE_NAME:-$(hostname)}"
tls_verify="${XUI_NODE_TLS_VERIFY:-verify}"
export XUI_NONINTERACTIVE=1

json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

if [[ "${XUI_SKIP_INSTALL:-0}" != "1" ]]; then
    installer="$(mktemp)" || fail "Cannot create a temporary file."
    trap 'rm -f "$installer"' EXIT
    echo "Downloading the installer from ${installer_url}"
    curl -fsSL "$installer_url" -o "$installer" || fail "Could not download the installer."
    bash "$installer" || fail "The installer failed; the server was not registered."
fi

[[ -r "$result_file" ]] || fail "No install result at ${result_file}; cannot tell the master how to reach this node."
# shellcheck disable=SC1090
source "$result_file"
[[ -n "${XUI_API_TOKEN:-}" ]] || fail "The install result has no API token."

re='^(https?)://(\[[^]]+\]|[^:/]+):([0-9]+)/(.*)$'
[[ "${XUI_ACCESS_URL:-}" =~ $re ]] || fail "Cannot read the panel address from '${XUI_ACCESS_URL:-}'."
scheme="${BASH_REMATCH[1]}"
host="${BASH_REMATCH[2]}"
port="${BASH_REMATCH[3]}"
base="/${BASH_REMATCH[4]#/}"
[[ "$base" == */ ]] || base="${base}/"

# An address the installer could not detect is left for the master to fill in
# from the connection this request arrives on.
address=""
if [[ "$host" != SERVER_IP_UNKNOWN ]]; then
    address="${host#[}"
    address="${address%]}"
fi

payload=$(printf '{"token":"%s","name":"%s","scheme":"%s","address":"%s","port":%s,"basePath":"%s","apiToken":"%s","tlsVerifyMode":"%s"}' \
    "$(json_escape "$XUI_JOIN_TOKEN")" "$(json_escape "$node_name")" "$scheme" "$(json_escape "$address")" \
    "$port" "$(json_escape "$base")" "$(json_escape "$XUI_API_TOKEN")" "$(json_escape "$tls_verify")")

curl_flags=(-sS -m 40 -H 'Content-Type: application/json')
[[ "${XUI_JOIN_INSECURE:-0}" == "1" ]] && curl_flags+=(-k)

echo "Registering ${node_name} on the master"
attempts="${XUI_JOIN_ATTEMPTS:-12}"
for ((i = 1; i <= attempts; i++)); do
    reply=$(curl "${curl_flags[@]}" -d "$payload" "$XUI_JOIN_URL" 2>&1)
    if [[ "$reply" == *'"success":true'* ]]; then
        echo -e "${green}Done: ${node_name} is now a node of the master panel.${plain}"
        exit 0
    fi
    echo -e "${yellow}Attempt ${i}/${attempts}: ${reply}${plain}"
    # A bad token never gets better; an unreachable node may once its panel is up.
    [[ "$reply" == *"invalid or expired"* ]] && fail "The join token was refused. Issue a new command on the master."
    sleep 5
done
fail "The master could not add this node. Check that the master can reach ${scheme}://${address:-this server}:${port}${base} and run the command again."
