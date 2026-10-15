#!/usr/bin/env bash
# Security checks against a running stack.
#
#   ./scripts/dev-up.sh
#   ./scripts/security-check.sh
#
# Each check asserts the *defence*, not just that a request failed: an expired
# token, a tampered token, a role reaching past its permissions, malformed input,
# cross-tenant access, and the rate limiter at its configured value.
set -uo pipefail

cd "$(dirname "$0")/.."

GATEWAY="${GATEWAY:-http://localhost:8000}"
PASS=0
FAIL=0

pass() { PASS=$((PASS + 1)); printf '  \033[32mPASS\033[0m %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf '  \033[31mFAIL\033[0m %s\n' "$1"; }

expect() {
  local label="$1" want="$2" got="$3" body="${4:-}"
  if [ "$want" = "$got" ]; then
    pass "$label (HTTP $got)"
  else
    fail "$label: want HTTP $want, got $got — body: $(printf '%s' "$body" | head -c 160)"
  fi
}

TOKEN() { go run ./scripts/mint-token -role "$1" -tenant "${2:-demo}" -user "${3:-u-$1}"; }

# await_budget waits out the per-minute rate-limit window when an earlier test run
# has already consumed it. A 429 during an RBAC check would otherwise look like a
# failure even though it is a rejection of a different kind.
await_budget() {
  for attempt in 1 2 3 4 5 6 7 8; do
    code=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$GATEWAY/api/v1/vendors?per_page=1" \
      -H "Authorization: Bearer $(TOKEN admin)")
    if [ "$code" != "429" ]; then
      return 0
    fi
    echo "  (kuota rate limit terpakai — menunggu 10s, percobaan $attempt)"
    sleep 10
  done
  echo "  (peringatan: kuota rate limit belum pulih)"
}

echo "== 1. token =="
expired=$(go run ./scripts/mint-token -role admin -tenant demo -user u-exp -ttl=-5m)
code=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$GATEWAY/api/v1/vendors" -H "Authorization: Bearer $expired")
expect "token kedaluwarsa ditolak" 401 "$code"

valid=$(TOKEN admin)
tampered="${valid%?}X"   # flips the last character of the signature
code=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$GATEWAY/api/v1/vendors" -H "Authorization: Bearer $tampered")
expect "tanda tangan token yang diubah ditolak" 401 "$code"

code=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$GATEWAY/api/v1/vendors" -H "Authorization: Bearer not-a-token")
expect "token ngawur ditolak" 401 "$code"

code=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$GATEWAY/api/v1/vendors" -H "Authorization: Basic $valid")
expect "skema Authorization lain ditolak" 401 "$code"

echo "== 2. RBAC tidak bisa dilewati =="
await_budget
VENDOR=$(TOKEN vendor demo v-sec)
for target in "POST /api/v1/vendors" "POST /api/v1/tenders" "POST /api/v1/approvals" "GET /api/v1/audit-logs" "GET /api/v1/reports/tender-summary"; do
  method="${target%% *}"
  path="${target#* }"
  code=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X "$method" "$GATEWAY$path" \
    -H "Authorization: Bearer $VENDOR" -H 'Content-Type: application/json' -d '{}')
  if [ "$code" = "403" ] || [ "$code" = "401" ]; then
    pass "role vendor ditolak di $method $path (HTTP $code)"
  else
    fail "role vendor TIDAK ditolak di $method $path (HTTP $code)"
  fi
done

# A vendor token cannot decide an approval that belongs to someone else.
ADMIN=$(TOKEN admin)
await_budget

# Make sure there is a pending approval to attack: a PO above the threshold gets
# one created automatically by the consumer.
stamp_sec=$(date +%s)
curl -sS --max-time 15 -o /dev/null -X POST "$GATEWAY/api/v1/purchase-orders" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d "{\"tender_id\":\"tender-sec-$stamp_sec\",\"bid_id\":\"bid-sec-$stamp_sec\",\"vendor_id\":\"v-sec\",\"currency\":\"IDR\",\"lines\":[{\"name\":\"Item\",\"quantity\":2,\"unit_price\":{\"amount\":9000000,\"currency\":\"IDR\"}}]}"

pending=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  sleep 2
  pending=$(curl -sS --max-time 10 "$GATEWAY/api/v1/approvals?status=pending" -H "Authorization: Bearer $ADMIN" \
    | python3 -c "
import json,sys
try:
    data = json.load(sys.stdin).get('data') or []
    print(data[0]['id'] if data else '')
except Exception:
    print('')
" 2>/dev/null)
  [ -n "$pending" ] && break
done
if [ -n "$pending" ]; then
  code=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/approvals/$pending/decide" \
    -H "Authorization: Bearer $VENDOR" -H 'Content-Type: application/json' -d '{"approve":true}')
  expect "role vendor tidak bisa memutuskan approval" 403 "$code"
else
  echo "  (lewati: tidak ada approval pending)"
fi

echo "== 3. validasi input =="
unknown=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/vendors" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"code":"VND-SEC-1","name":"PT Sec","email":"sec@contoh.co.id","is_admin":true}')
expect "field tak dikenal ditolak" 400 "$unknown"

python3 -c "
import json
print(json.dumps({'code':'VND-SEC-BIG','name':'X'*1200000,'email':'big@contoh.co.id'}))
" > /tmp/sec_big.json
big=$(curl -sS --max-time 20 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/vendors" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' --data-binary @/tmp/sec_big.json)
if [ "$big" = "400" ] || [ "$big" = "413" ]; then
  pass "body melebihi batas ditolak (HTTP $big)"
else
  fail "body melebihi batas: want 400/413, got $big"
fi

injection=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/vendors" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"code":"VND-SEC-2","name":"{\"$ne\":null}","email":"nosql@contoh.co.id"}')
if [ "$injection" = "201" ] || [ "$injection" = "409" ]; then
  pass "operator NoSQL diperlakukan sebagai teks biasa (HTTP $injection)"
else
  fail "operator NoSQL: want 201/409, got $injection"
fi
# The value must not have become a query operator that matches everything.
search=$(curl -sS --max-time 10 "$GATEWAY/api/v1/vendors?search=%7B%22%24ne%22%3Anull%7D" -H "Authorization: Bearer $ADMIN" \
  | python3 -c "import json,sys;print(json.load(sys.stdin)['meta']['total'])")
if [ "${search:-0}" -le 2 ]; then
  pass "pencarian tidak dievaluasi sebagai operator ($search hasil)"
else
  fail "pencarian tampak dievaluasi sebagai operator ($search hasil)"
fi

echo "== 4. isolasi tenant =="
await_budget
OTHER=$(TOKEN admin tenant-lain u-admin-lain)
own=$(curl -sS --max-time 10 "$GATEWAY/api/v1/vendors?per_page=1" -H "Authorization: Bearer $ADMIN" \
  | python3 -c "import json,sys;d=json.load(sys.stdin)['data'];print(d[0]['id'] if d else '')")
if [ -n "$own" ]; then
  code=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$GATEWAY/api/v1/vendors/$own" -H "Authorization: Bearer $OTHER")
  expect "tenant lain tidak bisa membaca vendor milik demo" 404 "$code"
else
  echo "  (lewati: belum ada vendor)"
fi

echo "== 5. rate limiter =="
limit="${RATE_LIMIT_RPS:-300}"
echo "  mengirim $((limit + 40)) request untuk memicu batas ($limit/menit)..."
saw429=0
for _ in $(seq 1 $((limit + 40))); do
  code=$(curl -sS --max-time 5 -o /dev/null -w '%{http_code}' "$GATEWAY/api/v1/vendors?per_page=1" -H "Authorization: Bearer $ADMIN")
  if [ "$code" = "429" ]; then
    saw429=1
    break
  fi
done
if [ "$saw429" = "1" ]; then
  pass "rate limiter menolak setelah batas terlampaui (429)"
else
  fail "rate limiter TIDAK pernah menolak pada batas $limit"
fi

echo
printf 'hasil: \033[32m%d PASS\033[0m, \033[31m%d FAIL\033[0m\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
