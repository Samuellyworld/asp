#!/bin/bash
# infrastructure test script
# tests: postgresql, redis, database migrations

set -e

echo "trading bot - infrastructure test"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if docker is running
if ! docker info > /dev/null 2>&1; then
    echo -e "${RED}Error: Docker is not running${NC}"
    exit 1
fi

# Load environment variables if .env exists
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Set defaults
POSTGRES_USER=${POSTGRES_USER:-trading_bot}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-trading_bot_secret}
POSTGRES_DB=${POSTGRES_DB:-trading_bot}
REDIS_PASSWORD=${REDIS_PASSWORD:-redis_secret}

echo "1. starting infrastructure containers..."
docker-compose up -d postgres redis

echo ""
echo "2. waiting for services to be healthy..."

# Wait for PostgreSQL
echo -n "   PostgreSQL: "
for i in {1..30}; do
    if docker exec trading-bot-postgres pg_isready -U $POSTGRES_USER -d $POSTGRES_DB > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Ready${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}✗ Timeout${NC}"
        exit 1
    fi
    sleep 1
done

# Wait for Redis
echo -n "   Redis:      "
for i in {1..30}; do
    if docker exec trading-bot-redis redis-cli -a $REDIS_PASSWORD ping > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Ready${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}✗ Timeout${NC}"
        exit 1
    fi
    sleep 1
done

echo ""
echo "3. verifying postgresql database..."

# Count tables
TABLE_COUNT=$(docker exec trading-bot-postgres psql -U $POSTGRES_USER -d $POSTGRES_DB -t -c "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE';" | xargs)

echo "   Tables found: $TABLE_COUNT"

if [ "$TABLE_COUNT" -eq "15" ]; then
    echo -e "   ${GREEN}✓ All 15 tables created successfully${NC}"
else
    echo -e "   ${YELLOW}⚠ Expected 15 tables, found $TABLE_COUNT${NC}"
fi

# List all tables
echo ""
echo "   Table listing:"
docker exec trading-bot-postgres psql -U $POSTGRES_USER -d $POSTGRES_DB -c "\dt" | grep -E "^\s+public\s+\|" | while read line; do
    TABLE=$(echo $line | awk -F'|' '{print $2}' | xargs)
    echo "   ✓ $TABLE"
done

# Test a query
echo ""
echo -n "   Empty users check: "
USER_COUNT=$(docker exec trading-bot-postgres psql -U $POSTGRES_USER -d $POSTGRES_DB -t -c "SELECT count(*) FROM users;" | xargs)
if [ "$USER_COUNT" -eq "0" ]; then
    echo -e "${GREEN}✓ 0 rows (as expected)${NC}"
else
    echo -e "${YELLOW}Found $USER_COUNT rows${NC}"
fi

echo ""
echo "4. verifying redis..."

echo -n "   ping test: "
PONG=$(docker exec trading-bot-redis redis-cli -a $REDIS_PASSWORD PING 2>/dev/null)
if [ "$PONG" = "PONG" ]; then
    echo -e "${GREEN}$PONG${NC}"
else
    echo -e "${RED}Failed${NC}"
    exit 1
fi

echo ""
echo "5. checking container logs for errors..."

PG_ERRORS=$(docker-compose logs postgres 2>&1 | grep -i "error\|fatal" | wc -l | xargs)
REDIS_ERRORS=$(docker-compose logs redis 2>&1 | grep -i "error\|fatal" | wc -l | xargs)

echo -n "   PostgreSQL log errors: "
if [ "$PG_ERRORS" -eq "0" ]; then
    echo -e "${GREEN}0${NC}"
else
    echo -e "${YELLOW}$PG_ERRORS (review with 'docker-compose logs postgres')${NC}"
fi

echo -n "   Redis log errors:      "
if [ "$REDIS_ERRORS" -eq "0" ]; then
    echo -e "${GREEN}0${NC}"
else
    echo -e "${YELLOW}$REDIS_ERRORS (review with 'docker-compose logs redis')${NC}"
fi

echo ""
echo -e "${GREEN}infrastructure test complete!${NC}"
echo ""
echo "next steps:"
echo "  • copy .env.example to .env and fill in your secrets"
echo ""
