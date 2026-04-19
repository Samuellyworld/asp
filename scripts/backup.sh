#!/usr/bin/env bash
set -euo pipefail

# backup.sh — dumps PostgreSQL database, compresses, and optionally uploads to S3
# Usage: ./scripts/backup.sh [--upload s3://bucket/path] [--keep DAYS]

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_DIR"

source .env 2>/dev/null || true

POSTGRES_USER="${POSTGRES_USER:-trading_bot}"
POSTGRES_DB="${POSTGRES_DB:-trading_bot}"
BACKUP_DIR="${BACKUP_DIR:-$PROJECT_DIR/backups}"
KEEP_DAYS="${KEEP_DAYS:-7}"
UPLOAD_PATH=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --upload)  UPLOAD_PATH="$2"; shift 2 ;;
    --keep)    KEEP_DAYS="$2"; shift 2 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

mkdir -p "$BACKUP_DIR"

TIMESTAMP=$(date -u '+%Y%m%d_%H%M%S')
DUMP_FILE="$BACKUP_DIR/trading_bot_${TIMESTAMP}.sql"
COMPRESSED_FILE="${DUMP_FILE}.gz"

echo "=== Database Backup ==="
echo "Time: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo "Database: ${POSTGRES_DB}"

# 1. check postgres is running
if ! docker compose exec -T postgres pg_isready -U "$POSTGRES_USER" -q 2>/dev/null; then
  echo "ERROR: PostgreSQL is not running"
  exit 1
fi

# 2. dump the database
echo ""
echo "--- Dumping database ---"
docker compose exec -T postgres pg_dump \
  -U "$POSTGRES_USER" \
  -d "$POSTGRES_DB" \
  --no-owner \
  --no-privileges \
  --clean \
  --if-exists \
  > "$DUMP_FILE"

ROW_COUNT=$(wc -l < "$DUMP_FILE")
echo "  Dump complete: $ROW_COUNT lines"

# 3. compress
echo ""
echo "--- Compressing ---"
gzip -f "$DUMP_FILE"
SIZE=$(du -h "$COMPRESSED_FILE" | cut -f1)
echo "  Compressed: $COMPRESSED_FILE ($SIZE)"

# 4. verify backup integrity (decompress and check header)
echo ""
echo "--- Verifying backup ---"
if gzip -t "$COMPRESSED_FILE" 2>/dev/null; then
  echo "  Integrity check: ✓"
else
  echo "  Integrity check: ✗ CORRUPTED"
  exit 1
fi

# 5. upload to S3 (if requested)
if [ -n "$UPLOAD_PATH" ]; then
  echo ""
  echo "--- Uploading to $UPLOAD_PATH ---"
  if command -v aws &>/dev/null; then
    aws s3 cp "$COMPRESSED_FILE" "${UPLOAD_PATH}/$(basename "$COMPRESSED_FILE")"
    echo "  Upload complete"
  else
    echo "  WARNING: aws CLI not installed — skipping upload"
  fi
fi

# 6. clean old backups
echo ""
echo "--- Cleaning backups older than $KEEP_DAYS days ---"
REMOVED=$(find "$BACKUP_DIR" -name "trading_bot_*.sql.gz" -mtime +"$KEEP_DAYS" -delete -print | wc -l)
echo "  Removed: $REMOVED old backup(s)"

# 7. summary
echo ""
echo "=== Backup Summary ==="
echo "  File: $COMPRESSED_FILE"
echo "  Size: $SIZE"
REMAINING=$(find "$BACKUP_DIR" -name "trading_bot_*.sql.gz" | wc -l)
echo "  Total backups: $REMAINING"
echo "  Retention: $KEEP_DAYS days"
