#!/bin/bash
set -e

# install.sh — Install croft and the dependencies it needs.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Hyzokaaa/opencroft/main/install.sh -o install.sh
#   sudo bash install.sh
#
#   sudo bash install.sh --dry-run          print the plan and exit
#   sudo bash install.sh --yes              no questions (for automation)
#   sudo bash install.sh --binary ./croft   install a binary you already have
#   sudo bash install.sh --addr :8080       listen elsewhere (see the warning below)
#
# Nothing is installed before you have seen the exact commands.

VERSION="${VERSION:-latest}"
REPO="${REPO:-Hyzokaaa/opencroft}"
PREFIX="${PREFIX:-/usr/local}"
NGINX_CONF="${NGINX_CONF:-/etc/nginx/nginx.conf}"
NGINX_CONF_DIR="${NGINX_CONF_DIR:-/etc/nginx/croft.d}"

# Until authentication lands, the daemon must not be reachable from the network.
# Reach it over an SSH tunnel: ssh -L 8080:127.0.0.1:8080 you@server
ADDR="${ADDR:-127.0.0.1:8080}"

DRY_RUN=false
ASSUME_YES=false
LOCAL_BINARY=""

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=true ;;
    --yes|-y)  ASSUME_YES=true ;;
    --binary)  LOCAL_BINARY="$2"; shift ;;
    --addr)    ADDR="$2"; shift ;;
    *) echo "[ERROR] Unknown option: $1" >&2; exit 1 ;;
  esac
  shift
done

# ── Host detection ────────────────────────────────────────────────────────────

distro_family() {
  [ -f /etc/os-release ] || { echo "unknown"; return; }
  # shellcheck disable=SC1091
  . /etc/os-release

  case "${ID:-}" in
    debian|ubuntu|linuxmint|pop|raspbian) echo "debian" ;;
    fedora|rhel|centos|rocky|almalinux)   echo "rhel" ;;
    arch|manjaro|endeavouros)             echo "arch" ;;
    alpine)                               echo "alpine" ;;
    opensuse*|sles)                       echo "suse" ;;
    *)
      case "${ID_LIKE:-}" in
        *debian*) echo "debian" ;;
        *rhel*|*fedora*) echo "rhel" ;;
        *arch*)   echo "arch" ;;
        *suse*)   echo "suse" ;;
        *)        echo "unknown" ;;
      esac
      ;;
  esac
}

distro_id() {
  [ -f /etc/os-release ] || { echo "unknown"; return; }
  # shellcheck disable=SC1091
  . /etc/os-release
  echo "${ID:-unknown}"
}

distro_version() {
  [ -f /etc/os-release ] || return 0
  # shellcheck disable=SC1091
  . /etc/os-release
  echo "${VERSION_ID:-}"
}

install_command() {
  case "$(distro_family)" in
    debian) echo "apt-get install -y $*" ;;
    rhel)   command -v dnf >/dev/null && echo "dnf install -y $*" || echo "yum install -y $*" ;;
    arch)   echo "pacman -S --noconfirm $*" ;;
    alpine) echo "apk add $*" ;;
    suse)   echo "zypper install -y $*" ;;
    *)      echo "" ;;
  esac
}

refresh_command() {
  case "$(distro_family)" in
    debian) echo "apt-get update" ;;
    arch)   echo "pacman -Sy" ;;
    alpine) echo "apk update" ;;
    suse)   echo "zypper refresh" ;;
    *)      echo "" ;;
  esac
}

service_manager() {
  command -v systemctl >/dev/null && { echo "systemd"; return; }
  echo "openrc"
}

arch_suffix() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) echo "" ;;
  esac
}

# Incus is packaged natively almost everywhere. LXD needs snap, which is
# awkward outside Ubuntu, so it is only the fallback on older Ubuntu.
runtime_plan() {
  command -v incus >/dev/null && { echo "incus:present"; return; }
  command -v lxc >/dev/null && { echo "lxd:present"; return; }
  [ -x /snap/bin/lxc ] && { echo "lxd:present"; return; }

  local version
  version=$(distro_version)
  if [ "$(distro_id)" = "ubuntu" ] && [ "${version%%.*}" -lt 24 ] 2>/dev/null; then
    echo "lxd:snap"
  else
    echo "incus:package"
  fi
}

# ── Plan ──────────────────────────────────────────────────────────────────────

echo ""
echo "  ╔══════════════════════════════════════╗"
echo "  ║       Install croft                  ║"
echo "  ╚══════════════════════════════════════╝"
echo ""

FAMILY=$(distro_family)
ARCH=$(arch_suffix)

echo "  Host:    $(distro_id) $(distro_version) ($FAMILY, $(uname -m))"
echo "  Listen:  $ADDR"
echo ""

if [ -z "$(install_command true)" ]; then
  echo "[ERROR] Unsupported distribution. Install Incus and nginx by hand, then re-run." >&2
  exit 1
fi

if [ -z "$ARCH" ] && [ -z "$LOCAL_BINARY" ]; then
  echo "[ERROR] No published binary for $(uname -m). Build from source and pass --binary." >&2
  exit 1
fi

STEPS=()
COMMANDS=()
add_step() { STEPS+=("$1"); COMMANDS+=("$2"); }

RUNTIME_PLAN=$(runtime_plan)
RUNTIME="${RUNTIME_PLAN%%:*}"
RUNTIME_HOW="${RUNTIME_PLAN##*:}"

NGINX_MISSING=false
command -v nginx >/dev/null || NGINX_MISSING=true

if [ "$RUNTIME_HOW" != "present" ] || [ "$NGINX_MISSING" = true ]; then
  REFRESH=$(refresh_command)
  if [ -n "$REFRESH" ]; then
    add_step "Refresh package lists" "$REFRESH"
  fi
fi

case "$RUNTIME_HOW" in
  present)
    echo "  ✓ $RUNTIME is already installed"
    ;;
  package)
    add_step "Install Incus (container runtime)" "$(install_command incus)"
    add_step "Initialise Incus" "incus admin init --minimal"
    ;;
  snap)
    add_step "Install snapd" "$(install_command snapd)"
    add_step "Install LXD" "snap install lxd"
    add_step "Initialise LXD" "lxd init --auto"
    ;;
esac

if [ "$NGINX_MISSING" = false ]; then
  echo "  ✓ nginx is already installed"
else
  add_step "Install nginx (reverse proxy)" "$(install_command nginx)"
fi

if [ -n "$LOCAL_BINARY" ]; then
  add_step "Install croft from $LOCAL_BINARY" "install -m 0755 '$LOCAL_BINARY' $PREFIX/bin/croft"
else
  RELEASE_URL="https://github.com/$REPO/releases/$([ "$VERSION" = latest ] && echo latest/download || echo "download/$VERSION")/croft-linux-$ARCH"
  add_step "Download croft" "curl -fsSL '$RELEASE_URL' -o $PREFIX/bin/croft && chmod 0755 $PREFIX/bin/croft"
fi

add_step "Create the vhost directory" "install -d $NGINX_CONF_DIR"
add_step "Wire it into nginx" "include $NGINX_CONF_DIR/*.conf; → the http block of $NGINX_CONF"

if [ "$(service_manager)" = "systemd" ]; then
  add_step "Install the croft service" "write /etc/systemd/system/croft.service, then systemctl enable --now croft"
fi

echo ""
echo "── Plan ──"
echo ""
for i in "${!STEPS[@]}"; do
  printf "  %d. %s\n" "$((i + 1))" "${STEPS[$i]}"
  printf "     \$ %s\n" "${COMMANDS[$i]}"
done

echo ""
echo "  Installing a container runtime creates a network bridge and firewall"
echo "  rules on this host. Nothing above runs until you say so."
echo ""

case "$ADDR" in
  127.0.0.1:*|localhost:*) ;;
  *)
    echo "  [WARNING] $ADDR is reachable from the network, and croft has no"
    echo "            authentication yet. Anyone who can reach that port can"
    echo "            create and destroy containers. Use an SSH tunnel instead:"
    echo "                ssh -L 8080:127.0.0.1:8080 you@this-host"
    echo ""
    ;;
esac

if [ "$DRY_RUN" = true ]; then
  echo "  Dry run — nothing was executed."
  echo ""
  exit 0
fi

if [ "$EUID" -ne 0 ]; then
  echo "[ERROR] Installing requires root. Re-run with sudo, or use --dry-run." >&2
  exit 1
fi

if [ "$ASSUME_YES" = false ]; then
  if [ ! -t 0 ]; then
    echo "[ERROR] No terminal to confirm on. Download and run it, or pass --yes." >&2
    exit 1
  fi
  read -p "  Proceed? (y/N): " CONFIRM
  if [ "${CONFIRM,,}" != "y" ]; then
    echo "  Aborted."
    exit 0
  fi
  echo ""
fi

# ── Run ───────────────────────────────────────────────────────────────────────

run_step() {
  echo "── $1"
  eval "$2"
}

for i in "${!STEPS[@]}"; do
  case "${STEPS[$i]}" in
    "Wire it into nginx"*|"Install the croft service"*) continue ;;
  esac
  run_step "${STEPS[$i]}" "${COMMANDS[$i]}"
done

# One managed line, one directory of our own. Uninstalling is removing both.
echo "── Wire it into nginx"
install -d "$NGINX_CONF_DIR"
if grep -qF "$NGINX_CONF_DIR/*.conf" "$NGINX_CONF" 2>/dev/null; then
  echo "[OK] Already included"
elif grep -qE '^[[:space:]]*http[[:space:]]*\{' "$NGINX_CONF" 2>/dev/null; then
  awk -v line="    include $NGINX_CONF_DIR/*.conf;" '
    !done && /^[[:space:]]*http[[:space:]]*\{/ { print; print line; done=1; next }
    { print }
  ' "$NGINX_CONF" > "$NGINX_CONF.croft-tmp" && mv "$NGINX_CONF.croft-tmp" "$NGINX_CONF"
  nginx -t && echo "[OK] Included $NGINX_CONF_DIR"
else
  echo "[WARN] No http block found in $NGINX_CONF. Add this line yourself:"
  echo "           include $NGINX_CONF_DIR/*.conf;"
fi

if [ "$(service_manager)" = "systemd" ]; then
  echo "── Install the croft service"
  cat > /etc/systemd/system/croft.service <<EOF
[Unit]
Description=croft — container control plane
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$PREFIX/bin/croft serve --addr $ADDR
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now croft
  echo "[OK] croft.service running"
fi

echo ""
echo "  ╔══════════════════════════════════════════════════════╗"
echo "  ║           Installed                                  ║"
echo "  ╚══════════════════════════════════════════════════════╝"
echo ""
echo "    croft list                 show what is running"
echo "    croft create <name>        create a container"
echo "    croft destroy <name>       remove one"
echo ""
echo "  The panel is on http://$ADDR"
case "$ADDR" in
  127.0.0.1:*|localhost:*)
    echo "  Reach it from your machine with:"
    echo "      ssh -L 8080:$ADDR you@$(hostname -f 2>/dev/null || hostname)"
    ;;
esac
echo ""
echo "  Uninstall: systemctl disable --now croft; rm $PREFIX/bin/croft"
echo "             /etc/systemd/system/croft.service; rm -rf $NGINX_CONF_DIR"
echo ""
