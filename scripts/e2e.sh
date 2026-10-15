#!/usr/bin/env bash
# End-to-end test of the whole procurement chain against a running stack.
#
#   ./scripts/dev-up.sh          # in another terminal
#   ./scripts/e2e.sh
#
# Every step asserts the expected HTTP status, so a broken contract fails the
# run instead of printing a green-looking summary. Exits non-zero on first
# failure.
set -uo pipefail

cd "$(dirname "$0")/.."

GATEWAY="${GATEWAY:-http://localhost:8000}"
PASS=0
FAIL=0

pass() { PASS=$((PASS + 1)); printf '  \033[32mPASS\033[0m %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf '  \033[31mFAIL\033[0m %s\n' "$1"; }

# expect asserts an expected HTTP status and prints the body on mismatch.
expect() {
  local label="$1" want="$2" got="$3" body="$4"
  if [ "$want" = "$got" ]; then
    pass "$label (HTTP $got)"
  else
    fail "$label: want HTTP $want, got $got — body: $(printf '%s' "$body" | head -c 200)"
  fi
}

json() { python3 -c "import json,sys;print($1)"; }

TOKEN() { go run ./scripts/mint-token -role "$1" -tenant "${2:-demo}" -user "${3:-u-$1}"; }

echo "== 0. gateway readiness =="
ready=$(curl -sS --max-time 20 -o /tmp/e2e_ready.json -w '%{http_code}' "$GATEWAY/readyz")
expect "gateway /readyz" 200 "$ready" "$(cat /tmp/e2e_ready.json)"

echo "== 1. authentication =="
ADMIN=$(TOKEN admin)
VENDOR=$(TOKEN vendor demo v-e2e)
ANON=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$GATEWAY/api/v1/vendors")
expect "list tanpa token ditolak" 401 "$ANON" ""

# /auth/me needs a real account: a minted token carries a synthetic user id that
# has no document in the users collection, so the honest path is register + login.
stamp_auth=$(date +%s)
email="e2e${stamp_auth}@contoh.co.id"
reg=$(curl -sS --max-time 10 -o /tmp/e2e_reg.json -w '%{http_code}' -X POST "$GATEWAY/api/v1/auth/register" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$email\",\"name\":\"User E2E\",\"password\":\"rahasia-kuat-123\",\"role\":\"procurement_officer\"}")
expect "POST /auth/register" 201 "$reg" "$(cat /tmp/e2e_reg.json)"

weak=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/auth/register" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d "{\"email\":\"weak${stamp_auth}@contoh.co.id\",\"name\":\"Weak\",\"password\":\"123\",\"role\":\"admin\"}")
expect "password lemah ditolak" 422 "$weak" ""

login=$(curl -sS --max-time 10 -o /tmp/e2e_login.json -w '%{http_code}' -X POST "$GATEWAY/api/v1/auth/login" \
  -H 'Content-Type: application/json' -d "{\"tenant_id\":\"demo\",\"email\":\"$email\",\"password\":\"rahasia-kuat-123\"}")
expect "POST /auth/login" 200 "$login" "$(cat /tmp/e2e_login.json)"
USER_TOKEN=$(cat /tmp/e2e_login.json | json "json.load(sys.stdin).get('data',{}).get('token','')" 2>/dev/null)

bad_login=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/auth/login" \
  -H 'Content-Type: application/json' -d "{\"tenant_id\":\"demo\",\"email\":\"$email\",\"password\":\"salah-sekali\"}")
expect "password salah ditolak" 403 "$bad_login" ""

me=$(curl -sS --max-time 10 -o /tmp/e2e_me.json -w '%{http_code}' "$GATEWAY/api/v1/auth/me" -H "Authorization: Bearer $USER_TOKEN")
expect "GET /auth/me dengan token hasil login" 200 "$me" "$(cat /tmp/e2e_me.json)"

logout=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/auth/logout" -H "Authorization: Bearer $USER_TOKEN")
expect "POST /auth/logout" 204 "$logout" ""

revoked=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$GATEWAY/api/v1/auth/me" -H "Authorization: Bearer $USER_TOKEN")
expect "token yang sudah logout ditolak" 401 "$revoked" ""

echo "== 2. vendor =="
stamp=$(date +%s)
code="VND-E2E-${stamp: -5}"
vbody=$(curl -sS --max-time 10 -w '\n%{http_code}' -X POST "$GATEWAY/api/v1/vendors" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d "{\"code\":\"$code\",\"name\":\"PT E2E ${stamp}\",\"email\":\"e2e${stamp}@contoh.co.id\"}")
vstatus=$(printf '%s' "$vbody" | tail -1)
expect "POST /vendors" 201 "$vstatus" "$vbody"

dup=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/vendors" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d "{\"code\":\"$code\",\"name\":\"Duplikat\",\"email\":\"dup${stamp}@contoh.co.id\"}")
expect "kode vendor duplikat" 409 "$dup" ""

invalid=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/vendors" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"code":"","name":"","email":"bukan-email"}')
expect "validasi payload vendor" 422 "$invalid" ""

rbac=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/vendors" \
  -H "Authorization: Bearer $VENDOR" -H 'Content-Type: application/json' \
  -d "{\"code\":\"VND-DENY-$stamp\",\"name\":\"X\",\"email\":\"x@y.id\"}")
expect "role vendor dilarang menulis vendor" 403 "$rbac" ""

echo "== 3. tender =="
close_at=$(python3 -c "import datetime;print((datetime.datetime.utcnow()+datetime.timedelta(days=2)).strftime('%Y-%m-%dT%H:%M:%SZ'))")
tender_json=$(curl -sS --max-time 10 -X POST "$GATEWAY/api/v1/tenders" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d "{\"code\":\"TND-E2E-$stamp\",\"title\":\"Tender E2E $stamp\",\"items\":[{\"name\":\"Laptop\",\"quantity\":10,\"unit\":\"unit\"}],\"budget\":{\"amount\":120000000,\"currency\":\"IDR\"},\"close_at\":\"$close_at\"}")
TENDER_ID=$(printf '%s' "$tender_json" | json "json.load(sys.stdin)['data']['id']" 2>/dev/null)
if [ -n "${TENDER_ID:-}" ]; then pass "POST /tenders"; else fail "POST /tenders: $tender_json"; fi

close_draft=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/tenders/$TENDER_ID/close" -H "Authorization: Bearer $ADMIN")
expect "tutup tender yang masih draft" 409 "$close_draft" ""

pub=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/tenders/$TENDER_ID/publish" -H "Authorization: Bearer $ADMIN")
expect "publish tender" 200 "$pub" ""

pub2=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/tenders/$TENDER_ID/publish" -H "Authorization: Bearer $ADMIN")
expect "publish dua kali ditolak" 409 "$pub2" ""

echo "== 4. penawaran =="
bid_json=$(curl -sS --max-time 15 -X POST "$GATEWAY/api/v1/bids" \
  -H "Authorization: Bearer $VENDOR" -H 'Content-Type: application/json' \
  -d "{\"tender_id\":\"$TENDER_ID\",\"vendor_id\":\"v-e2e\",\"delivery_days\":14,\"currency\":\"IDR\",\"items\":[{\"name\":\"Laptop\",\"quantity\":10,\"unit_price\":{\"amount\":11000000,\"currency\":\"IDR\"}}]}")
BID_ID=$(printf '%s' "$bid_json" | json "json.load(sys.stdin)['data']['id']" 2>/dev/null)
BID_TOTAL=$(printf '%s' "$bid_json" | json "json.load(sys.stdin)['data']['total']['amount']" 2>/dev/null)
if [ "${BID_TOTAL:-0}" = "110000000" ]; then
  pass "total penawaran dihitung server (110000000)"
else
  fail "total penawaran: want 110000000, got ${BID_TOTAL:-none} — $bid_json"
fi

bid_dup=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/bids" \
  -H "Authorization: Bearer $VENDOR" -H 'Content-Type: application/json' \
  -d "{\"tender_id\":\"$TENDER_ID\",\"vendor_id\":\"v-e2e\",\"currency\":\"IDR\",\"items\":[{\"name\":\"Laptop\",\"quantity\":1,\"unit_price\":{\"amount\":1000,\"currency\":\"IDR\"}}]}")
expect "vendor sama mengirim penawaran dua kali" 409 "$bid_dup" ""

cmp=$(curl -sS --max-time 10 -o /tmp/e2e_cmp.json -w '%{http_code}' "$GATEWAY/api/v1/bids/compare/$TENDER_ID" -H "Authorization: Bearer $ADMIN")
expect "perbandingan harga" 200 "$cmp" "$(cat /tmp/e2e_cmp.json)"

sel=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/bids/$BID_ID/select" -H "Authorization: Bearer $ADMIN")
expect "pilih penawaran" 200 "$sel" ""

echo "== 5. purchase order + approval otomatis =="
po_json=$(curl -sS --max-time 15 -X POST "$GATEWAY/api/v1/purchase-orders" \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d "{\"tender_id\":\"$TENDER_ID\",\"bid_id\":\"$BID_ID\",\"vendor_id\":\"v-e2e\",\"currency\":\"IDR\",\"lines\":[{\"name\":\"Laptop\",\"quantity\":10,\"unit_price\":{\"amount\":11000000,\"currency\":\"IDR\"}}]}")
PO_ID=$(printf '%s' "$po_json" | json "json.load(sys.stdin)['data']['purchase_order']['id']" 2>/dev/null)
PO_STATUS=$(printf '%s' "$po_json" | json "json.load(sys.stdin)['data']['purchase_order']['status']" 2>/dev/null)
if [ "$PO_STATUS" = "pending_approval" ]; then
  pass "PO di atas threshold masuk pending_approval"
else
  fail "status PO: want pending_approval, got ${PO_STATUS:-none} — $po_json"
fi

# The approval is created by the consumer, so it needs a moment to appear.
APPROVAL_ID=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  sleep 2
  APPROVAL_ID=$(curl -sS --max-time 10 "$GATEWAY/api/v1/approvals?entity_id=$PO_ID" -H "Authorization: Bearer $ADMIN" \
    | json "(json.load(sys.stdin)['data'] or [{}])[0].get('id','')" 2>/dev/null)
  [ -n "$APPROVAL_ID" ] && break
done
if [ -n "$APPROVAL_ID" ]; then
  pass "approval dibuat otomatis oleh consumer (po.created)"
else
  fail "approval otomatis tidak muncul untuk PO $PO_ID"
fi

if [ -n "$APPROVAL_ID" ]; then
  APPROVER=$(TOKEN approver demo u-approver)
  wrong_approver=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/approvals/$APPROVAL_ID/decide" \
    -H "Authorization: Bearer $(TOKEN approver demo u-approver-lain)" -H 'Content-Type: application/json' -d '{"approve":true}')
  expect "approver lain tidak boleh memutuskan" 403 "$wrong_approver" ""

  reject_no_reason=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/approvals/$APPROVAL_ID/decide" \
    -H "Authorization: Bearer $APPROVER" -H 'Content-Type: application/json' -d '{"approve":false}')
  expect "penolakan tanpa alasan" 422 "$reject_no_reason" ""

  decide=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -X POST "$GATEWAY/api/v1/approvals/$APPROVAL_ID/decide" \
    -H "Authorization: Bearer $APPROVER" -H 'Content-Type: application/json' \
    -d '{"approve":true,"comment":"disetujui oleh e2e"}')
  expect "approver memutuskan" 200 "$decide" ""

  # The PO status must change on its own: no PATCH is issued here on purpose.
  PO_STATUS_AFTER=""
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    sleep 2
    PO_STATUS_AFTER=$(curl -sS --max-time 10 "$GATEWAY/api/v1/purchase-orders/$PO_ID" -H "Authorization: Bearer $ADMIN" \
      | json "json.load(sys.stdin)['data']['status']" 2>/dev/null)
    [ "$PO_STATUS_AFTER" = "approved" ] && break
  done
  if [ "$PO_STATUS_AFTER" = "approved" ]; then
    pass "status PO berubah sendiri setelah keputusan (approved)"
  else
    fail "status PO setelah keputusan: want approved, got ${PO_STATUS_AFTER:-none}"
  fi
fi

echo "== 6. audit & laporan =="
audit=$(curl -sS --max-time 10 -o /tmp/e2e_audit.json -w '%{http_code}' "$GATEWAY/api/v1/audit-logs?per_page=5" -H "Authorization: Bearer $ADMIN")
expect "audit log" 200 "$audit" "$(cat /tmp/e2e_audit.json)"

audit_denied=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$GATEWAY/api/v1/audit-logs" -H "Authorization: Bearer $VENDOR")
expect "role vendor dilarang membaca audit" 403 "$audit_denied" ""

for report in po-by-vendor tender-summary approval-summary snapshot; do
  code=$(curl -sS --max-time 15 -o /dev/null -w '%{http_code}' "$GATEWAY/api/v1/reports/$report" -H "Authorization: Bearer $ADMIN")
  expect "laporan /$report" 200 "$code" ""
done

csv=$(curl -sS --max-time 15 -o /tmp/e2e_report.csv -w '%{http_code}' "$GATEWAY/api/v1/reports/po-by-vendor?format=csv" -H "Authorization: Bearer $ADMIN")
expect "export CSV" 200 "$csv" ""
if head -1 /tmp/e2e_report.csv | grep -q "vendor_id,order_count,total,currency"; then
  pass "header CSV sesuai"
else
  fail "header CSV tidak sesuai: $(head -1 /tmp/e2e_report.csv)"
fi

echo
printf 'hasil: \033[32m%d PASS\033[0m, \033[31m%d FAIL\033[0m\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
