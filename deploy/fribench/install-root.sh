#!/usr/bin/env bash
set -Eeuo pipefail

PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'

readonly source_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly control_source="${source_dir}/fribench-codex-continuity"
readonly nginx_source="${source_dir}/codex-continuity.nginx.conf"
readonly control_target='/usr/local/sbin/fribench-codex-continuity'
readonly nginx_target='/usr/local/share/fribench/codex-continuity.nginx.conf'
readonly sudoers_target='/etc/sudoers.d/codex-continuity-deploy'
readonly compose_file='/srv/fribench/apps/internal/codex-continuity/current/compose.yaml'

[ "$(id -u)" -eq 0 ] || {
  echo 'ERROR: run this installer through sudo.' >&2
  exit 1
}

/usr/bin/test -f "$control_source"
/usr/bin/test -f "$nginx_source"
/usr/bin/install -o root -g root -m 0755 "$control_source" "$control_target"
/usr/bin/install -o root -g root -m 0644 "$nginx_source" "$nginx_target"

sudoers_temp="$(/usr/bin/mktemp)"
trap '/usr/bin/rm -f -- "$sudoers_temp"' EXIT

/usr/bin/cat >"$sudoers_temp" <<EOF
# Managed deployment permissions for Codex Continuity on fribench.
cpl ALL=(root) NOPASSWD: /usr/bin/docker compose -f ${compose_file} up -d --build
cpl ALL=(root) NOPASSWD: /usr/bin/docker compose -f ${compose_file} restart
cpl ALL=(root) NOPASSWD: /usr/bin/docker compose -f ${compose_file} ps
cpl ALL=(root) NOPASSWD: /usr/bin/docker compose -f ${compose_file} logs --tail=200
cpl ALL=(root) NOPASSWD: /usr/bin/docker compose -f ${compose_file} down
cpl ALL=(root) NOPASSWD: ${control_target} open, ${control_target} close, ${control_target} status
EOF

/usr/sbin/visudo -cf "$sudoers_temp"
/usr/bin/install -o root -g root -m 0440 "$sudoers_temp" "$sudoers_target"

echo 'Codex Continuity root controls installed.'
