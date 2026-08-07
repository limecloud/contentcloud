#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ -f "${ROOT_DIR}/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "${ROOT_DIR}/.env"
  set +a
fi

BACKEND_HOST="${CONTENTCLOUD_DEV_BACKEND_HOST:-127.0.0.1}"
BACKEND_PORT="${CONTENTCLOUD_DEV_BACKEND_PORT:-8080}"
FRONTEND_HOST="${CONTENTCLOUD_DEV_FRONTEND_HOST:-0.0.0.0}"
FRONTEND_PORT="${CONTENTCLOUD_DEV_FRONTEND_PORT:-5173}"
KILL_OCCUPIED="${CONTENTCLOUD_DEV_KILL_OCCUPIED:-0}"
ISOLATED_MEMORY="${CONTENTCLOUD_DEV_ISOLATED_MEMORY:-1}"
PIDS=()

usage() {
  cat <<'EOF'
Usage: ./scripts/dev.sh [options]

Options:
  --backend-port PORT   Set the Go API port (default: 8080).
  --frontend-port PORT  Set the Vite port (default: 5173).
  --memory              Use isolated in-memory data and local blob storage (default).
  --with-database       Use database and object-storage settings loaded from the environment.
  --kill-occupied       Gracefully stop listeners on the selected ports before starting.
  --no-kill-occupied    Refuse to start when a selected port is occupied (default).
  -h, --help            Show this help message.

Environment:
  CONTENTCLOUD_DEV_BACKEND_HOST=127.0.0.1
  CONTENTCLOUD_DEV_BACKEND_PORT=8080
  CONTENTCLOUD_DEV_FRONTEND_HOST=0.0.0.0
  CONTENTCLOUD_DEV_FRONTEND_PORT=5173
  CONTENTCLOUD_DEV_ISOLATED_MEMORY=1
  CONTENTCLOUD_DEV_KILL_OCCUPIED=0
  CONTENTCLOUD_DEV_DATA_DIR=./var/dev-data
EOF
}

require_option_value() {
  local option="$1"
  local value="${2:-}"
  if [ -z "${value}" ]; then
    echo "Missing value for ${option}." >&2
    usage >&2
    exit 2
  fi
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --backend-port)
      require_option_value "$@"
      BACKEND_PORT="$2"
      shift 2
      ;;
    --frontend-port)
      require_option_value "$@"
      FRONTEND_PORT="$2"
      shift 2
      ;;
    --memory)
      ISOLATED_MEMORY=1
      shift
      ;;
    --with-database)
      ISOLATED_MEMORY=0
      shift
      ;;
    --kill-occupied)
      KILL_OCCUPIED=1
      shift
      ;;
    --no-kill-occupied)
      KILL_OCCUPIED=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

normalize_boolean() {
  local name="$1"
  local value="$2"
  case "${value}" in
    1|true|TRUE|yes|YES)
      printf '1'
      ;;
    0|false|FALSE|no|NO)
      printf '0'
      ;;
    *)
      echo "${name} must be a boolean value, got: ${value}" >&2
      exit 2
      ;;
  esac
}

validate_port() {
  local name="$1"
  local port="$2"
  case "${port}" in
    ''|*[!0-9]*)
      echo "${name} must be an integer between 1 and 65535, got: ${port}" >&2
      exit 2
      ;;
  esac
  if [ "${port}" -lt 1 ] || [ "${port}" -gt 65535 ]; then
    echo "${name} must be between 1 and 65535, got: ${port}" >&2
    exit 2
  fi
}

KILL_OCCUPIED="$(normalize_boolean "CONTENTCLOUD_DEV_KILL_OCCUPIED" "${KILL_OCCUPIED}")"
ISOLATED_MEMORY="$(normalize_boolean "CONTENTCLOUD_DEV_ISOLATED_MEMORY" "${ISOLATED_MEMORY}")"
validate_port "Backend port" "${BACKEND_PORT}"
validate_port "Frontend port" "${FRONTEND_PORT}"

if [ "${BACKEND_PORT}" = "${FRONTEND_PORT}" ]; then
  echo "Backend and frontend ports must be different." >&2
  exit 2
fi

require_command() {
  local command_name="$1"
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "Required command not found: ${command_name}" >&2
    exit 1
  fi
}

require_command go
require_command pnpm
require_command curl

if [ ! -d "${ROOT_DIR}/web/node_modules" ]; then
  echo "Frontend dependencies are missing. Run 'pnpm install' from ${ROOT_DIR}." >&2
  exit 1
fi

port_in_use() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1
    return $?
  fi
  if command -v nc >/dev/null 2>&1; then
    nc -z 127.0.0.1 "${port}" >/dev/null 2>&1
    return $?
  fi
  return 1
}

require_free_port() {
  local name="$1"
  local port="$2"
  if ! port_in_use "${port}"; then
    return
  fi

  if [ "${KILL_OCCUPIED}" != "1" ]; then
    echo "${name} port ${port} is already in use." >&2
    echo "Stop that listener, choose another port, or explicitly pass --kill-occupied." >&2
    exit 1
  fi

  if ! command -v lsof >/dev/null 2>&1; then
    echo "Cannot identify the listener on ${name} port ${port}: lsof is unavailable." >&2
    exit 1
  fi

  local listeners=()
  local pid
  while IFS= read -r pid; do
    if [ -n "${pid}" ]; then
      listeners+=("${pid}")
    fi
  done < <(lsof -nP -tiTCP:"${port}" -sTCP:LISTEN 2>/dev/null | sort -u)

  if [ "${#listeners[@]}" -eq 0 ]; then
    echo "Cannot identify the listener on ${name} port ${port}." >&2
    exit 1
  fi

  echo "${name} port ${port} is occupied; sending SIGTERM to:"
  for pid in "${listeners[@]}"; do
    ps -p "${pid}" -o pid=,command= 2>/dev/null || echo "  PID ${pid}"
    kill -TERM "${pid}" 2>/dev/null || true
  done

  local attempt
  for ((attempt = 0; attempt < 50; attempt++)); do
    if ! port_in_use "${port}"; then
      echo "${name} port ${port} is now available."
      return
    fi
    sleep 0.1
  done

  echo "${name} port ${port} is still occupied after 5 seconds; refusing to send SIGKILL." >&2
  exit 1
}

terminate_tree() {
  local parent_pid="$1"
  local child_pid
  if command -v pgrep >/dev/null 2>&1; then
    while IFS= read -r child_pid; do
      if [ -n "${child_pid}" ]; then
        terminate_tree "${child_pid}"
      fi
    done < <(pgrep -P "${parent_pid}" 2>/dev/null || true)
  fi
  kill -TERM "${parent_pid}" >/dev/null 2>&1 || true
}

cleanup() {
  local status="$?"
  local pid
  local attempt
  trap - EXIT INT TERM
  set +e

  for pid in "${PIDS[@]}"; do
    if kill -0 "${pid}" >/dev/null 2>&1; then
      terminate_tree "${pid}"
    fi
  done

  for ((attempt = 0; attempt < 50; attempt++)); do
    local running=0
    for pid in "${PIDS[@]}"; do
      if kill -0 "${pid}" >/dev/null 2>&1; then
        running=1
      fi
    done
    if [ "${running}" -eq 0 ]; then
      break
    fi
    sleep 0.1
  done

  for pid in "${PIDS[@]}"; do
    if kill -0 "${pid}" >/dev/null 2>&1; then
      echo "Process ${pid} did not stop after SIGTERM; it was not force-killed." >&2
    else
      wait "${pid}" >/dev/null 2>&1 || true
    fi
  done

  exit "${status}"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

require_free_port "Backend" "${BACKEND_PORT}"
require_free_port "Frontend" "${FRONTEND_PORT}"

BACKEND_URL="http://${BACKEND_HOST}:${BACKEND_PORT}"
FRONTEND_DISPLAY_HOST="${FRONTEND_HOST}"
if [ "${FRONTEND_DISPLAY_HOST}" = "0.0.0.0" ]; then
  FRONTEND_DISPLAY_HOST="localhost"
fi

if [ "${ISOLATED_MEMORY}" = "1" ]; then
  export CONTENTCLOUD_DATABASE_URL=""
  export CONTENTCLOUD_AUTO_MIGRATE=0
  export CONTENTCLOUD_S3_BUCKET=""
  export CONTENTCLOUD_DATA_DIR="${CONTENTCLOUD_DEV_DATA_DIR:-${ROOT_DIR}/var/dev-data}"
fi

echo "ContentCloud API: ${BACKEND_URL}"
echo "ContentCloud UI:  http://${FRONTEND_DISPLAY_HOST}:${FRONTEND_PORT}"
if [ "${ISOLATED_MEMORY}" = "1" ]; then
  echo "Storage:          isolated memory + ${CONTENTCLOUD_DATA_DIR}"
else
  echo "Storage:          environment configuration"
fi

(
  cd "${ROOT_DIR}"
  exec env \
    CONTENTCLOUD_ADDR="${BACKEND_HOST}:${BACKEND_PORT}" \
    CONTENTCLOUD_DEV_MODE=1 \
    CONTENTCLOUD_WEB_DIST="${CONTENTCLOUD_WEB_DIST:-web/dist}" \
    go run ./cmd/contentcloud-server
) &
BACKEND_PID="$!"
PIDS+=("${BACKEND_PID}")

wait_for_backend() {
  local attempt
  for ((attempt = 0; attempt < 100; attempt++)); do
    if curl --fail --silent --show-error --max-time 1 "${BACKEND_URL}/healthz" >/dev/null 2>&1; then
      return
    fi
    if ! kill -0 "${BACKEND_PID}" >/dev/null 2>&1; then
      echo "ContentCloud API exited before becoming ready." >&2
      wait "${BACKEND_PID}" || true
      exit 1
    fi
    sleep 0.1
  done
  echo "ContentCloud API did not become ready within 10 seconds: ${BACKEND_URL}/healthz" >&2
  exit 1
}

wait_for_backend

(
  cd "${ROOT_DIR}/web"
  exec env \
    VITE_DEV_PROXY_TARGET="${BACKEND_URL}" \
    VITE_DEV_PORT="${FRONTEND_PORT}" \
    pnpm exec vite --config vite.config.ts --host "${FRONTEND_HOST}" --port "${FRONTEND_PORT}" --strictPort
) &
PIDS+=("$!")

while true; do
  for pid in "${PIDS[@]}"; do
    if ! kill -0 "${pid}" >/dev/null 2>&1; then
      if wait "${pid}"; then
        exit 0
      else
        exit "$?"
      fi
    fi
  done
  sleep 1
done
