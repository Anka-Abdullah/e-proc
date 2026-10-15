#!/usr/bin/env bash
# Start every service locally for development, plus the API gateway.
#
#   ./scripts/dev-up.sh          # start in the background, logs in .dev-logs/
#   ./scripts/dev-down.sh        # stop everything started here
#
# Infra (Mongo, Redis, RabbitMQ) is expected to be running:
#   docker compose -f deploy/docker/docker-compose.yml up -d mongo redis rabbitmq
set -euo pipefail

cd "$(dirname "$0")/.."

LOG_DIR=".dev-logs"
PID_DIR=".dev-pids"
mkdir -p "$LOG_DIR" "$PID_DIR"

# Shared environment. Redis is on 6380 on this machine because 6379 is taken.
export APP_ENV="${APP_ENV:-development}"
export LOG_LEVEL="${LOG_LEVEL:-info}"
export MONGO_URI="${MONGO_URI:-mongodb://localhost:27017}"
export MONGO_DATABASE="${MONGO_DATABASE:-eproc}"
export REDIS_ADDR="${REDIS_ADDR:-localhost:6380}"
export RABBITMQ_URL="${RABBITMQ_URL:-amqp://eproc:eproc@localhost:5672/}"
export JWT_SECRET="${JWT_SECRET:-dev-secret-change-me}"
export JWT_TTL="${JWT_TTL:-12h}"
export APPROVAL_THRESHOLD="${APPROVAL_THRESHOLD:-50000000}"
export AUTO_CLOSE_INTERVAL="${AUTO_CLOSE_INTERVAL:-1m}"
export FRONTEND_ORIGIN="${FRONTEND_ORIGIN:-http://localhost:3000}"
# The edge rate limiter counts per IP per minute. Development and the E2E script
# legitimately send more than the production default, so the dev stack raises it.
# scripts/security-check.sh verifies the limiter still engages at this value.
export RATE_LIMIT_RPS="${RATE_LIMIT_RPS:-300}"
# Cross-service validation: bid-service checks the tender over gRPC.
export TENDER_GRPC_ADDR="${TENDER_GRPC_ADDR:-localhost:9082}"
# Who receives approvals created automatically from events.
export APPROVAL_ASSIGNEE="${APPROVAL_ASSIGNEE:-u-approver}"
export APPROVAL_REMINDER_AFTER="${APPROVAL_REMINDER_AFTER:-24h}"
export REMINDER_INTERVAL="${REMINDER_INTERVAL:-1h}"
export REPORT_INTERVAL="${REPORT_INTERVAL:-24h}"

# name:http_port:grpc_port
SERVICES=(
  "auth-service:8008:9098"
  "vendor-service:8001:9081"
  "tender-service:8002:9082"
  "bid-service:8003:9083"
  "po-service:8004:9084"
  "approval-service:8005:9085"
  "audit-service:8006:9086"
  "reporting-service:8007:9087"
)

echo "==> building binaries"
mkdir -p bin
for entry in "${SERVICES[@]}"; do
  name="${entry%%:*}"
  go build -o "bin/$name" "./cmd/$name"
done
go build -o bin/api-gateway ./cmd/api-gateway

echo "==> starting services"
for entry in "${SERVICES[@]}"; do
  IFS=":" read -r name http_port grpc_port <<< "$entry"
  HTTP_ADDR=":$http_port" GRPC_ADDR=":$grpc_port" \
    "./bin/$name" > "$LOG_DIR/$name.log" 2>&1 &
  echo $! > "$PID_DIR/$name.pid"
  echo "    $name http=:$http_port grpc=:$grpc_port pid=$(cat "$PID_DIR/$name.pid")"
done

HTTP_ADDR=":8000" GRPC_ADDR=":9099" \
  ./bin/api-gateway > "$LOG_DIR/api-gateway.log" 2>&1 &
echo $! > "$PID_DIR/api-gateway.pid"
echo "    api-gateway http=:8000 pid=$(cat "$PID_DIR/api-gateway.pid")"

echo
echo "waiting for readiness..."
sleep 4
for port in 8000 8001 8002 8003 8004 8005 8006 8007 8008; do
  if curl -fsS --max-time 3 "http://localhost:$port/healthz" >/dev/null 2>&1; then
    echo "    :$port ok"
  else
    echo "    :$port NOT responding (see $LOG_DIR/*.log)"
  fi
done

echo
echo "gateway:    http://localhost:8000"
echo "login:      POST http://localhost:8000/api/v1/auth/login"
echo "logs:       tail -f $LOG_DIR/api-gateway.log"
echo "stop:       ./scripts/dev-down.sh"
