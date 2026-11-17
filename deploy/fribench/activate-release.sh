#!/usr/bin/env bash
set -Eeuo pipefail

PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'

readonly release_id="${1:-}"
readonly app_root='/srv/fribench/tools/codex-continuity'
readonly release_dir="${app_root}/releases/${release_id}"
readonly shared_dir="${app_root}/shared"
readonly web_archive="/tmp/codex-continuity-web-${release_id}.tar.gz"

[[ "$release_id" =~ ^[0-9]{8}-[0-9]{6}$ ]] || {
  echo 'ERROR: release id must use YYYYMMDD-HHMMSS.' >&2
  exit 1
}
[ -d "$release_dir" ] || {
  echo "ERROR: release directory does not exist: ${release_dir}" >&2
  exit 1
}
[ -f "$web_archive" ] || {
  echo "ERROR: web archive does not exist: ${web_archive}" >&2
  exit 1
}

if [ -f "${release_dir}/continuity-server-linux-amd64" ]; then
  /usr/bin/mv \
    "${release_dir}/continuity-server-linux-amd64" \
    "${release_dir}/continuity-server"
fi
/usr/bin/chmod 0555 "${release_dir}/continuity-server"
/usr/bin/mkdir -p "${release_dir}/web" "${shared_dir}/data" "${shared_dir}/downloads"
/usr/bin/tar -xzf "$web_archive" -C "${release_dir}/web"

if [ ! -f "${shared_dir}/.env" ]; then
  umask 077
  admin_password="$(/usr/bin/openssl rand -hex 24)"
  env_temp="$(/usr/bin/mktemp "${TMPDIR:-/tmp}/codex-continuity-env.XXXXXX")"
  trap '/usr/bin/rm -f -- "${env_temp:-}"' EXIT
  /usr/bin/printf '%s\n' \
    'CONTINUITY_PUBLIC_URL=https://continuity.fribench.cn' \
    'CONTINUITY_ADMIN_EMAIL=admin@fribench.local' \
    'CONTINUITY_ADMIN_NAME=系统管理员' \
    "CONTINUITY_ADMIN_PASSWORD=${admin_password}" \
    'CONTINUITY_COOKIE_SECURE=true' \
    'CONTINUITY_TRUST_PROXY=true' \
    'CONTINUITY_MAX_UPLOAD_MIB=500' \
    'CONTINUITY_HANDOFF_RETENTION_DAYS=14' \
    'CONTINUITY_HANDOFF_VERSIONS=2' \
    'CONTINUITY_USER_STORAGE_MIB=2048' \
    'CONTINUITY_TOTAL_STORAGE_MIB=10240' \
    >"$env_temp"
  /usr/bin/mv "$env_temp" "${shared_dir}/.env"
  trap - EXIT
else
  /usr/bin/sed -i \
    -e 's|^CONTINUITY_PUBLIC_URL=http://1\.14\.72\.50:24001$|CONTINUITY_PUBLIC_URL=https://continuity.fribench.cn|' \
    -e 's|^CONTINUITY_COOKIE_SECURE=false$|CONTINUITY_COOKIE_SECURE=true|' \
    "${shared_dir}/.env"
fi

for setting in \
  'CONTINUITY_HANDOFF_RETENTION_DAYS=14' \
  'CONTINUITY_HANDOFF_VERSIONS=2' \
  'CONTINUITY_USER_STORAGE_MIB=2048' \
  'CONTINUITY_TOTAL_STORAGE_MIB=10240'
do
  key="${setting%%=*}"
  /usr/bin/grep -q "^${key}=" "${shared_dir}/.env" ||
    /usr/bin/printf '%s\n' "$setting" >>"${shared_dir}/.env"
done
/usr/bin/chmod 0600 "${shared_dir}/.env"

next_link="${app_root}/current.next"
/usr/bin/ln -sfn "$release_dir" "$next_link"
/usr/bin/mv -Tf "$next_link" "${app_root}/current"

echo "Activated Codex Continuity release: ${release_id}"
