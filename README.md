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

## Menjalankan

Butuh Go 1.26+, Node 24+, pnpm, Docker.

```bash
cp .env.example .env          # sesuaikan bila perlu

make infra-up                 # Mongo + Redis + RabbitMQ
make run SERVICE=vendor-service   # jalankan satu service (terminal terpisah)
make up                       # atau seluruh stack via Docker Compose
```

Frontend:

```bash
cd web && pnpm install && pnpm dev   # http://localhost:3000
```

Perintah lain: `make help`.

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

## Status

- [x] Fase 0 — perencanaan, ruang lingkup, aktor, arsitektur
- [x] Fase 1 — repository, tooling, Docker, CI, linter
- [ ] Fase 2 — domain & data model final, protobuf
- [ ] Fase 3 — service inti
- [ ] Fase 4 — frontend
- [ ] Fase 5 — integrasi & workflow otomatis
- [ ] Fase 6 — testing & QA
- [ ] Fase 7 — CI/CD & deployment
- [ ] Fase 8 — dokumentasi & portofolio
- [ ] Fase 9 — AI-assisted workflow
- [ ] Fase 10 — finalisasi

## AI-assisted workflow

Bagian ini diisi selama pengerjaan (Fase 9): prompt yang dipakai, apa yang
divalidasi manual, dan bagaimana hasil AI diuji. Aturannya satu: **tidak ada
kode hasil AI yang masuk tanpa review dan test yang lulus.**

## Lisensi

Belum ditentukan.
