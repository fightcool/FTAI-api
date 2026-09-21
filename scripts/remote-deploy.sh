#!/usr/bin/env bash

set -Eeuo pipefail

APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="$APP_DIR/docker-compose.ft-api.yml"
ENV_FILE="$APP_DIR/.env"
CONTAINER_NAME="${CONTAINER_NAME:-ft-api}"
HEALTH_TIMEOUT_SECONDS="${HEALTH_TIMEOUT_SECONDS:-300}"
HEALTH_INTERVAL_SECONDS="${HEALTH_INTERVAL_SECONDS:-5}"

log() {
  printf '[remote-deploy] %s\n' "$*"
}

die() {
  printf '[remote-deploy] ERROR: %s\n' "$*" >&2
  exit 1
}

compose() {
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

check_env() {
  [[ -f "$ENV_FILE" ]] ||
    die "Missing $ENV_FILE. Run ./deploy-remote.sh bootstrap and fill it in first."

  local key
  local missing=0
  for key in SQL_DSN REDIS_CONN_STRING SESSION_SECRET; do
    if ! grep -Eq "^[[:space:]]*${key}=.+" "$ENV_FILE"; then
      log "Missing required key in .env: $key"
      missing=1
    fi
  done
  [[ "$missing" -eq 0 ]] || die "Complete $ENV_FILE before deploying."

  chmod 600 "$ENV_FILE"
}

preflight() {
  command -v docker >/dev/null 2>&1 || die "Docker is not installed on the server."
  docker info >/dev/null 2>&1 || die "The current user cannot access the Docker daemon."
  docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is not installed."
  [[ -f "$COMPOSE_FILE" ]] || die "Missing $COMPOSE_FILE"
  check_env
  mkdir -p "$APP_DIR/data" "$APP_DIR/logs"
  log "Preflight passed."
}

container_health() {
  docker inspect \
    --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' \
    "$CONTAINER_NAME" 2>/dev/null || true
}

health() {
  local deadline=$((SECONDS + HEALTH_TIMEOUT_SECONDS))
  local status

  while (( SECONDS < deadline )); do
    status="$(container_health)"
    case "$status" in
      healthy)
        log "Health check passed."
        return 0
        ;;
      unhealthy|exited|dead)
        log "Container entered state: $status"
        return 1
        ;;
      *)
        log "Waiting for application health: ${status:-not-created}"
        ;;
    esac
    sleep "$HEALTH_INTERVAL_SECONDS"
  done

  log "Health check timed out after ${HEALTH_TIMEOUT_SECONDS}s."
  return 1
}

show_logs() {
  compose logs --tail=200 new-api || true
}

status() {
  compose ps
  local status
  status="$(container_health)"
  [[ -n "$status" ]] && log "Container health: $status"
}

prepare_rollback_image() {
  if docker image inspect ft-api:current >/dev/null 2>&1; then
    docker tag ft-api:current ft-api:rollback
    log "Tagged the current healthy image as ft-api:rollback."
  fi
}

deploy() {
  preflight

  local tag="${DEPLOY_TAG:-manual-$(date +%Y%m%d%H%M%S)}"
  local candidate="ft-api:$tag"
  local current="ft-api:current"
  local rollback="ft-api:rollback"

  prepare_rollback_image

  log "Building $candidate"
  APP_IMAGE="$candidate" compose build --pull new-api

  log "Starting $candidate"
  APP_IMAGE="$candidate" compose up -d --remove-orphans new-api

  if health; then
    docker tag "$candidate" "$current"
    log "Deployment succeeded and ft-api:current now points to $candidate."
    status
    return 0
  fi

  show_logs

  if docker image inspect "$rollback" >/dev/null 2>&1; then
    log "Deployment failed; restoring the previous healthy image."
    APP_IMAGE="$rollback" compose up -d --remove-orphans new-api
    health || true
  else
    log "Deployment failed and no rollback image exists."
  fi

  return 1
}

build() {
  preflight

  local tag="${DEPLOY_TAG:-manual-$(date +%Y%m%d%H%M%S)}"
  local candidate="ft-api:$tag"

  log "Building $candidate"
  APP_IMAGE="$candidate" compose build --pull new-api
  log "Build completed: $candidate"
}

rollback() {
  preflight
  docker image inspect ft-api:rollback >/dev/null 2>&1 ||
    die "No rollback image exists."

  APP_IMAGE="ft-api:rollback" compose up -d --remove-orphans new-api
  health
  log "Rollback completed."
}

restart() {
  preflight
  compose restart new-api
  health
}

stop() {
  preflight
  compose down
}

main() {
  case "${1:-}" in
    preflight)
      preflight
      ;;
    deploy)
      deploy
      ;;
    build)
      build
      ;;
    status)
      status
      ;;
    health)
      preflight
      health
      ;;
    logs)
      compose logs --tail=200 -f new-api
      ;;
    restart)
      restart
      ;;
    stop)
      stop
      ;;
    rollback)
      rollback
      ;;
    *)
      die "Usage: $0 {preflight|build|deploy|status|health|logs|restart|stop|rollback}"
      ;;
  esac
}

main "$@"
