#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SSH_TARGET="${SSH_TARGET:-ftapi@ftai.cc}"
REMOTE_DIR="${REMOTE_DIR:-/opt/ft-api}"

SSH_OPTIONS=(
  -o BatchMode=yes
  -o ConnectTimeout=10
  -o ServerAliveInterval=30
  -o ServerAliveCountMax=4
)

log() {
  printf '[deploy] %s\n' "$*"
}

die() {
  printf '[deploy] ERROR: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "Missing local command: $1"
}

remote() {
  ssh "${SSH_OPTIONS[@]}" "$SSH_TARGET" "$@"
}

remote_tty() {
  ssh -tt "${SSH_OPTIONS[@]}" "$SSH_TARGET" "$@"
}

check_local_repo() {
  git -C "$ROOT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1 ||
    die "$ROOT_DIR is not a Git repository"

  if [[ "${ALLOW_DIRTY:-0}" != "1" ]] && [[ -n "$(git -C "$ROOT_DIR" status --porcelain)" ]]; then
    die "Working tree is dirty. Commit changes or run with ALLOW_DIRTY=1."
  fi
}

sync_code() {
  require_command rsync
  require_command ssh

  log "Ensuring remote directory exists: $SSH_TARGET:$REMOTE_DIR"
  remote "mkdir -p '$REMOTE_DIR'"

  log "Syncing source to $SSH_TARGET:$REMOTE_DIR"
  rsync -az --delete \
    -e "ssh ${SSH_OPTIONS[*]}" \
    --exclude='.git/' \
    --exclude='.env' \
    --exclude='.env.*' \
    --exclude='data/' \
    --exclude='logs/' \
    --exclude='backups/' \
    --exclude='node_modules/' \
    --exclude='web/node_modules/' \
    --exclude='web/dist/' \
    --exclude='tiktoken_cache/' \
    --exclude='.DS_Store' \
    "$ROOT_DIR/" "$SSH_TARGET:$REMOTE_DIR/"
}

remote_script() {
  remote "cd '$REMOTE_DIR' && bash scripts/remote-deploy.sh $*"
}

initialize_server_dir() {
  remote "mkdir -p '$REMOTE_DIR/data' '$REMOTE_DIR/logs'"
}

initialize_env() {
  remote "cd '$REMOTE_DIR' && if [ ! -f .env ]; then cp deploy/ft-api.env.example .env && chmod 600 .env; fi"
}

deploy() {
  check_local_repo
  sync_code
  initialize_server_dir
  initialize_env

  local deploy_tag
  deploy_tag="$(git -C "$ROOT_DIR" rev-parse --short=12 HEAD)"

  log "Deploying revision $deploy_tag"
  remote "cd '$REMOTE_DIR' && DEPLOY_TAG='$deploy_tag' bash scripts/remote-deploy.sh deploy"
}

build() {
  check_local_repo
  sync_code
  initialize_server_dir

  local deploy_tag
  deploy_tag="$(git -C "$ROOT_DIR" rev-parse --short=12 HEAD)"

  log "Building revision $deploy_tag without changing the running container"
  remote "cd '$REMOTE_DIR' && DEPLOY_TAG='$deploy_tag' bash scripts/remote-deploy.sh build"
}

show_help() {
  cat <<'EOF'
Usage: ./deploy-remote.sh <command>

Commands:
  bootstrap   Create remote runtime directories and a private .env from the example
  build       Build the current revision without changing the running container
  deploy      Sync the current revision and deploy it (default)
  sync        Sync source only
  preflight   Check SSH, Docker, Compose, permissions, and required .env keys
  status      Show container and health status
  health      Wait for the application health check
  logs        Follow application logs
  restart     Restart the application
  stop        Stop the application
  rollback    Restore the previous healthy image
  shell       Open a shell in the server project directory

Environment overrides:
  SSH_TARGET   SSH destination, default: ftapi@ftai.cc
  REMOTE_DIR   Remote project directory, default: /opt/ft-api
  ALLOW_DIRTY  Set to 1 to deploy an uncommitted working tree
EOF
}

main() {
  local command="${1:-deploy}"

  case "$command" in
    bootstrap)
      check_local_repo
      sync_code
      initialize_server_dir
      initialize_env
      log "Bootstrap complete. Edit $REMOTE_DIR/.env on the server before deploying."
      ;;
    deploy)
      deploy
      ;;
    build)
      build
      ;;
    sync)
      check_local_repo
      sync_code
      ;;
    preflight)
      remote_script "preflight"
      ;;
    status)
      remote_script "status"
      ;;
    health)
      remote_script "health"
      ;;
    logs)
      remote_tty "cd '$REMOTE_DIR' && bash scripts/remote-deploy.sh logs"
      ;;
    restart)
      remote_script "restart"
      ;;
    stop)
      remote_script "stop"
      ;;
    rollback)
      remote_script "rollback"
      ;;
    shell)
      remote_tty "cd '$REMOTE_DIR' && exec \"\${SHELL:-/bin/bash}\" -l"
      ;;
    help|--help|-h)
      show_help
      ;;
    *)
      show_help
      die "Unknown command: $command"
      ;;
  esac
}

main "$@"
