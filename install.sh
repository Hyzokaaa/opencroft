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

# There is a login, but no TLS of its own yet. Serving plain http on a public
# interface would put a password on the wire, so default to localhost and let
# an SSH tunnel do the encryption.
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
  add_step "Install croft from $LOCAL_BINARY" "install -m 0755 '$LOCAL_BINARY' $PREFIX/bin/croft.new && mv $PREFIX/bin/croft.new $PREFIX/bin/croft"
else
  RELEASE_URL="https://github.com/$REPO/releases/$([ "$VERSION" = latest ] && echo latest/download || echo "download/$VERSION")/croft-linux-$ARCH"
  # Download beside the target and rename. Writing over a running binary fails
  # with ETXTBSY; renaming replaces the directory entry and always works.
  add_step "Download croft" "curl -fsSL '$RELEASE_URL' -o $PREFIX/bin/croft.new && chmod 0755 $PREFIX/bin/croft.new && mv $PREFIX/bin/croft.new $PREFIX/bin/croft"
fi

add_step "Create the croft system user" "groupadd --system croft; useradd --system -g croft -d /var/lib/croft -s /usr/sbin/nologin croft"
add_step "Create the vhost directory" "install -d $NGINX_CONF_DIR"
add_step "Wire it into nginx" "include $NGINX_CONF_DIR/*.conf; → the http block of $NGINX_CONF"

if [ "$(service_manager)" = "systemd" ]; then
  add_step "Install two services" "croft-agent (root, talks to the runtime) and croft (unprivileged, serves HTTP)"
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
    echo "  [WARNING] $ADDR is reachable from the network, and croft serves"
    echo "            plain http. Signing in would send the password in the"
    echo "            clear. Put a TLS proxy in front, or use an SSH tunnel:"
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
    "Wire it into nginx"*|"Install two services"*|"Create the croft system user"*) continue ;;
  esac
  run_step "${STEPS[$i]}" "${COMMANDS[$i]}"
done

# One managed line, one directory of our own. Uninstalling is removing both.
echo "── Wire it into nginx"
install -d "$NGINX_CONF_DIR"
if grep -qF "$NGINX_CONF_DIR/*.conf" "$NGINX_CONF" 2>/dev/null; then
  echo "[OK] Already included"
elif grep -qE '^[[:space:]]*http[[:space:]]*\{' "$NGINX_CONF" 2>/dev/null; then
  # Back up before touching it. This host may already be serving something,
  # and a broken nginx.conf takes that down with it.
  BACKUP="$NGINX_CONF.croft-backup-$(date +%Y%m%d%H%M%S)"
  cp -p "$NGINX_CONF" "$BACKUP"

  awk -v line="    include $NGINX_CONF_DIR/*.conf;" '
    !done && /^[[:space:]]*http[[:space:]]*\{/ { print; print line; done=1; next }
    { print }
  ' "$NGINX_CONF" > "$NGINX_CONF.croft-tmp" && mv "$NGINX_CONF.croft-tmp" "$NGINX_CONF"

  if nginx -t 2>/dev/null; then
    echo "[OK] Included $NGINX_CONF_DIR (previous file kept at $BACKUP)"
  else
    mv "$BACKUP" "$NGINX_CONF"
    echo "[WARN] nginx rejected the change, so it was reverted. Nothing was left broken."
    echo "       Add this line to the http block yourself:"
    echo "           include $NGINX_CONF_DIR/*.conf;"
  fi
else
  echo "[WARN] No http block found in $NGINX_CONF. Add this line yourself:"
  echo "           include $NGINX_CONF_DIR/*.conf;"
fi

# The half that serves HTTP to a browser has no business being root. The
# agent holds the privileges; the API reaches it over a socket its group owns.
echo "── Create the croft system user"
getent group croft >/dev/null || groupadd --system croft
getent passwd croft >/dev/null || useradd --system -g croft -d /var/lib/croft -s /usr/sbin/nologin croft
install -d -o croft -g croft -m 0750 /var/lib/croft
echo "[OK] user croft"

if [ "$(service_manager)" = "systemd" ]; then
  echo "── Install two services"

  cat > /etc/systemd/system/croft-agent.service <<EOF
[Unit]
Description=croft agent — the only half that talks to the container runtime
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
RuntimeDirectory=croft
RuntimeDirectoryMode=0755
ExecStart=$PREFIX/bin/croft agent --socket /run/croft/agent.sock --group croft
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

  cat > /etc/systemd/system/croft.service <<EOF
[Unit]
Description=croft — container control plane
After=croft-agent.service
Requires=croft-agent.service

[Service]
Type=simple
User=croft
Group=croft
StateDirectory=croft
ExecStart=$PREFIX/bin/croft serve --addr $ADDR
Restart=on-failure
RestartSec=3

# It needs no privileges, so it is given none.
NoNewPrivileges=yes
PrivateTmp=yes
ProtectHome=yes
ProtectSystem=strict
ProtectKernelTunables=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable --now croft-agent
  systemctl enable --now croft
  echo "[OK] croft-agent.service (root) and croft.service (unprivileged) running"
fi

# There is no sign-up in the panel on purpose: an account is created by
# somebody who already has a shell on this machine.
echo ""
echo "── Create the first user"

if [ "$ASSUME_YES" = false ] && [ -t 0 ]; then
  read -p "  Username (empty for a generated admin account): " FIRST_USER
  if [ -n "$FIRST_USER" ]; then
    "$PREFIX/bin/croft" user add "$FIRST_USER" || true
  else
    "$PREFIX/bin/croft" user bootstrap || true
  fi
else
  # Unattended. A fixed default password is the most exploited weakness in
  # self-hosted software, and "change it later" protects nobody, so this
  # generates one and prints it once.
  "$PREFIX/bin/croft" user bootstrap || true
fi

# The account was created by root, so the database belongs to root. The
# unprivileged half has to be able to read and write it.
chown -R croft:croft /var/lib/croft 2>/dev/null || true

echo ""
echo "  ╔══════════════════════════════════════════════════════╗"
echo "  ║           Installed                                  ║"
echo "  ╚══════════════════════════════════════════════════════╝"
echo ""
echo "    croft user add <name>      let somebody sign in"
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
echo "  Uninstall:"
echo "    systemctl disable --now croft croft-agent"
echo "    rm $PREFIX/bin/croft /etc/systemd/system/croft{,-agent}.service"
echo "    rm -rf $NGINX_CONF_DIR /var/lib/croft && userdel croft"
echo ""
