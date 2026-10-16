# E-Procurement Platform

Platform pengadaan barang/jasa berbasis microservices: manajemen vendor,
tender/RFQ, penawaran harga, purchase order, approval workflow, audit log, dan
reporting — dalam satu alur yang terekam jejaknya.

**Stack:** Golang · gRPC + Protobuf · REST · MongoDB · Redis · RabbitMQ ·
Next.js · TypeScript · Tailwind CSS · Docker · GitHub Actions

## Fitur (MVP)

- **Vendor** — CRUD supplier, status aktif/nonaktif, pencarian & filter
- **Tender / RFQ** — buat, publish, tutup (termasuk auto-close saat `close_at` lewat)
- **Bid** — vendor mengirim penawaran, perbandingan harga antar vendor
- **Purchase Order** — PO dibuat dari bid terpilih, status machine lengkap
- **Approval** — PO di atas threshold otomatis masuk approval, dengan history keputusan
- **Audit log** — append-only, mencatat actor/action/entity/before-after
- **Reporting** — laporan PO per vendor, tender, dan approval + export CSV

## Arsitektur

Diagram komponen, alasan pemilihan microservices/MongoDB/RabbitMQ, dan matriks
role ada di [`docs/architecture.md`](docs/architecture.md).

```mermaid
flowchart LR
  WEB[Next.js] --> GW[API Gateway<br/>REST + JWT + RBAC]
  GW --> S[7 domain services<br/>via gRPC]
  S --> M[(MongoDB)]
  S --> RD[(Redis)]
  S <--> MQ{{RabbitMQ}}
```

Data model dan index: [`docs/data-model.md`](docs/data-model.md).
Katalog event: [`docs/events.md`](docs/events.md).
Rencana pengerjaan 10 fase: [`docs/plan.md`](docs/plan.md).

## Menjalankan semuanya dengan satu perintah

Cara tercepat mencoba seluruh platform (13 container: 3 infrastruktur, 8 service,
gateway, dan web):

```bash
docker compose up -d --build          # dari akar repo
docker compose ps                     # tunggu sampai semuanya "healthy"
docker compose logs -f api-gateway    # ikuti log gateway
docker compose down                   # hentikan (volume tetap)
docker compose down -v                # hentikan dan hapus data
```

Yang perlu diketahui tentang perintah itu:

- Definisi aslinya di `deploy/docker/docker-compose.yml`; `compose.yaml` di akar
  repo hanya `include` file itu, sehingga `docker compose` bekerja tanpa `-f`.
- Urutan start dijaga `depends_on: condition: service_healthy`: gateway baru
  menyala setelah 8 upstream menjawab `/healthz`, dan web setelah gateway sehat.
  Jadi begitu `ps` hijau, API-nya memang siap — bukan sekadar container "Up".
- Alamat antar-service memakai nama compose (`http://tender-service:8082`),
  bukan `localhost`. Yang tetap `localhost` hanya `NEXT_PUBLIC_API_URL`, karena
  nilai itu dipakai **browser**, bukan container.
- Port host dipilih agar tidak bentrok dengan layanan umum: gateway `8000`,
  service `8001`–`8008`, web `3000`, Redis `6380` (6379 sering sudah dipakai).
- `JWT_SECRET` default adalah nilai pengembangan. Untuk pemakaian nyata, salin
  `.env.example` → `.env`, isi nilai asli, dan `APP_ENV=production` akan menolak
  boot bila `JWT_SECRET` masih default.

## Menjalankan untuk pengembangan (log ke terminal, hot reload frontend)

```bash
cp .env.example .env          # sesuaikan bila perlu
./scripts/dev-up.sh           # build + jalankan 8 service + gateway, log di .dev-logs/
./scripts/dev-down.sh         # hentikan semuanya (container infra tetap jalan)
```

Beberapa perintah lain:

```bash
make infra-up                       # hanya Mongo + Redis + RabbitMQ
make run SERVICE=vendor-service     # satu service saja, log ke terminal
make up / make down                 # alias untuk docker compose
make help                           # daftar lengkap
```

Frontend secara terpisah (hot reload):

```bash
cd web
pnpm install
NEXT_PUBLIC_API_URL=http://localhost:8000 pnpm dev   # http://localhost:3000
```

Halaman: `/login`, `/dashboard`, `/vendors`, `/tenders`, `/bids`,
`/purchase-orders`, `/approvals`, `/audit-logs`, `/reports`. Menu dan tombol
disaring mengikuti role (matriks izin di `web/src/lib/rbac.ts` mirror dari
`pkg/auth/auth.go`).

Catatan autentikasi UI: token disimpan di `localStorage` untuk MVP. Cookie
httpOnly yang di-set gateway adalah langkah pengerasan berikutnya, dan sudah
dicatat sebagai utang teknis di `web/src/lib/api.ts`.

## Pengujian

```bash
go test ./...                    # unit test semua paket Go
cd web && pnpm lint && pnpm exec tsc --noEmit

./scripts/e2e.sh                 # 35 asersi rantai penuh lewat gateway (stack harus jalan)
./scripts/security-check.sh      # 16 asersi keamanan (token, RBAC, input, tenant, rate limit)
go run ./scripts/loadtest -n 500 -c 25   # latensi & throughput

cd web && pnpm exec playwright test      # 7 tes UI di browser sungguhan
```

## Struktur repositori

```
cmd/            entrypoint setiap service
internal/       kode domain per service (model, repository, service, handler)
pkg/            library bersama: config, logger, db, cache, broker, auth, httpx, middleware, validator
proto/          definisi gRPC (sumber kebenaran kontrak antar service)
proto/gen/      hasil generate (jangan diedit manual)
web/            Next.js + TypeScript + Tailwind
deploy/docker/  Dockerfile + docker-compose
deploy/k8s/     manifest Kubernetes (opsional)
docs/           arsitektur, data model, event, rencana
scripts/        alat bantu (seed, migrasi)
```

## Peta port

Compose memetakan port **host** yang menghindari bentrokan dengan container lain
di mesin ini (6379 dipakai Redis lain, 8086 dipakai Tiny RDM).

| Service | HTTP (host→container) | gRPC |
| --- | --- | --- |
| api-gateway | 8000→8080 | — |
| vendor-service | 8001→8081 | 9081 |
| tender-service | 8002→8082 | 9082 |
| bid-service | 8003→8083 | 9083 |
| po-service | 8004→8084 | 9084 |
| approval-service | 8005→8085 | 9085 |
| audit-service | 8006→8086 | 9086 |
| reporting-service | 8007→8087 | 9087 |
| auth-service | 8008→8088 | 9098 |
| web | 3000 | — |
| mongo | 27017 | — |
| redis | **6380→6379** | — |
| rabbitmq | 5672, 15672 (UI) | — |

## Menjalankan satu service secara lokal

```bash
export MONGO_URI=mongodb://localhost:27017 MONGO_DATABASE=eproc \
       REDIS_ADDR=localhost:6380 RABBITMQ_URL=amqp://eproc:eproc@localhost:5672/ \
       HTTP_ADDR=:8001 GRPC_ADDR=:9081
go run ./cmd/vendor-service
```

Token untuk pengujian (belum ada UI login):

```bash
TOKEN=$(go run ./scripts/mint-token -role admin -tenant demo -user u-admin)
curl -sS localhost:8001/readyz
curl -sS -X POST localhost:8001/vendors -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"code":"vnd-001","name":"PT Contoh","email":"sales@contoh.co.id"}'
```

Cek jalur gRPC internal: `go run ./scripts/grpc-smoke -addr localhost:9081 -tenant demo`

## API Gateway

Klien hanya bicara ke gateway (`:8000`). Gateway memverifikasi JWT di edge,
menerapkan CORS + rate limit, lalu meneruskan permintaan ke service pemilik
prefix. Service tetap memverifikasi token dan otorisasinya sendiri (defence in
depth), jadi RBAC tidak bisa dilewati dengan menembak service langsung.

| Prefix | Service |
| --- | --- |
| `POST /api/v1/auth/login` (publik) | auth-service |
| `/api/v1/auth/*` | auth-service |
| `/api/v1/vendors` | vendor-service |
| `/api/v1/tenders` | tender-service |
| `/api/v1/bids` | bid-service |
| `/api/v1/purchase-orders` | po-service |
| `/api/v1/approvals` | approval-service |
| `/api/v1/audit-logs` | audit-service |
| `/api/v1/reports` | reporting-service |

`GET /healthz` (liveness) dan `GET /readyz` (kesiapan + status tiap upstream)
tidak butuh token, dan tidak dihitung oleh rate limiter karena dipakai probe.

Catatan transport: hop gateway → service memakai REST dengan JWT yang sama.
gRPC dipakai untuk pemanggilan antar-service (mis. `scripts/grpc-smoke`), bukan
untuk hop ini — jadi jangan mengeklaim gateway berbicara gRPC ke service.

## Menyalakan seluruh stack untuk development

```bash
docker compose -f deploy/docker/docker-compose.yml up -d mongo redis rabbitmq
./scripts/dev-up.sh          # build + jalankan 8 service + gateway, log di .dev-logs/
./scripts/dev-down.sh        # hentikan semua yang dijalankan dev-up
```

Login pertama (belum ada UI): lihat bagian "Menjalankan satu service" di atas
untuk membuat token, atau daftarkan user lewat `POST /api/v1/auth/register`
dengan token admin.

## Alur otomatis (tanpa tambalan dari UI)

Setelah Fase 5, rantai pengadaan berjalan sendiri lewat event:

```mermaid
sequenceDiagram
  participant PO as po-service
  participant MQ as RabbitMQ
  participant AP as approval-service
  participant PO2 as po-service (consumer)
  participant AU as audit-service

  PO->>MQ: po.created (requires_approval=true)
  MQ->>AP: buat approval otomatis (assignee dari APPROVAL_ASSIGNEE)
  AP->>MQ: approval.requested
  Note over AP: scheduler mengirim approval.reminder bila menunggu terlalu lama
  AP->>MQ: approval.decided
  MQ->>PO2: po.approved / po.rejected
  PO2->>PO2: status PO berubah sendiri (tanpa PATCH dari UI)
  MQ->>AU: seluruh kejadian tercatat append-only
```

