#!/usr/bin/env bash
set -euo pipefail

# deploy.sh — pulls latest code, rebuilds and deploys all services
# Usage: ./scripts/deploy.sh [--no-build] [--service SERVICE]

COMPOSE_FILE="docker-compose.yml"
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_DIR"

NO_BUILD=false
SERVICE=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --no-build) NO_BUILD=true; shift ;;
    --service)  SERVICE="$2"; shift 2 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

echo "=== Trading Bot Deployment ==="
echo "Directory: $PROJECT_DIR"
echo "Time: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"

# 1. pull latest code (skip if in CI)
if [ -d .git ]; then
  echo ""
  echo "--- Pulling latest code ---"
  git pull --ff-only || { echo "git pull failed — resolve conflicts first"; exit 1; }
fi

# 2. validate .env
if [ ! -f .env ]; then
  echo "ERROR: .env file not found. Copy .env.example and fill in secrets."
  exit 1
fi

source .env

if [ -z "${SECURITY_MASTER_KEY:-}" ]; then
  echo "ERROR: SECURITY_MASTER_KEY not set in .env"
  exit 1
fi

if [ -z "${TELEGRAM_BOT_TOKEN:-}" ] && [ -z "${DISCORD_BOT_TOKEN:-}" ]; then
  echo "ERROR: At least one of TELEGRAM_BOT_TOKEN or DISCORD_BOT_TOKEN is required"
  exit 1
fi

# 3. run migrations (applied via docker-entrypoint-initdb.d on first boot,
#    but re-running is idempotent due to IF NOT EXISTS)
echo ""
echo "--- Checking database ---"
docker compose up -d postgres
echo "Waiting for postgres health..."
for i in $(seq 1 30); do
  if docker compose exec -T postgres pg_isready -U "${POSTGRES_USER:-trading_bot}" -q 2>/dev/null; then
    echo "Postgres is ready"
    break
  fi
  sleep 1
done

# apply migrations
for f in migrations/*.sql; do
  echo "Applying migration: $f"
	  docker compose exec -T postgres psql \
	    -U "${POSTGRES_USER:-trading_bot}" \
	    -d "${POSTGRES_DB:-trading_bot}" \
	    -f "/docker-entrypoint-initdb.d/$(basename "$f")" \
	    --set ON_ERROR_STOP=1 \
	    --quiet
done

for f in go-bot/migrations/*.sql; do
  if [ -f "$f" ]; then
    echo "Applying migration: $f"
    docker compose cp "$f" postgres:/tmp/
	    docker compose exec -T postgres psql \
	      -U "${POSTGRES_USER:-trading_bot}" \
	      -d "${POSTGRES_DB:-trading_bot}" \
	      -f "/tmp/$(basename "$f")" \
	      --set ON_ERROR_STOP=1 \
	      --quiet
  fi
done

# 4. build and deploy
echo ""
echo "--- Building and deploying services ---"

BUILD_FLAG=""
if [ "$NO_BUILD" = false ]; then
  BUILD_FLAG="--build"
fi

if [ -n "$SERVICE" ]; then
  docker compose up -d $BUILD_FLAG "$SERVICE"
else
  docker compose up -d $BUILD_FLAG
fi

# 5. wait for health checks
echo ""
echo "--- Waiting for services to be healthy ---"
SERVICES=("postgres" "redis" "rust-engine" "ml-service" "go-bot")

for svc in "${SERVICES[@]}"; do
  echo -n "  $svc: "
  for i in $(seq 1 60); do
    STATUS=$(docker compose ps --format json "$svc" 2>/dev/null | grep -o '"Health":"[^"]*"' | head -1 || echo "")
    RUNNING=$(docker compose ps --format json "$svc" 2>/dev/null | grep -o '"State":"running"' || echo "")
    if [[ "$STATUS" == *'"Health":"healthy"'* ]] || [[ -n "$RUNNING" && "$svc" == "go-bot" ]]; then
      echo "✓"
      break
    fi
    if [ "$i" -eq 60 ]; then
      echo "✗ (timeout)"
      echo "  Check logs: docker compose logs $svc"
    fi
    sleep 2
  done
done

# 6. verify endpoints
echo ""
echo "--- Verifying endpoints ---"
sleep 3

HEALTH=$(curl -sf http://localhost:8080/health 2>/dev/null || echo "FAIL")
echo "  go-bot /health: $HEALTH"

PROM=$(curl -sf http://localhost:9090/-/ready 2>/dev/null || echo "FAIL")
echo "  prometheus: $PROM"

GRAF=$(curl -sf http://localhost:3000/api/health 2>/dev/null | grep -o '"database":"ok"' || echo "FAIL")
echo "  grafana: $GRAF"

echo ""
echo "=== Deployment complete ==="
docker compose ps --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}"
