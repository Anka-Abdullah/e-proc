# Rencana pengerjaan

Disusun berurutan tanpa timeline: setiap fase diselesaikan sebelum lanjut ke
fase berikutnya. Fokusnya MVP yang jalan, terdokumentasi, dan bisa didemokan.

## Fase 0 — Persiapan & perencanaan ✅

- [x] Tujuan proyek & masalah yang diselesaikan → `docs/architecture.md`
- [x] Ruang lingkup MVP (vendor, tender/RFQ, perbandingan harga, PO, approval, audit, reporting)
- [x] Aktor & role (admin, procurement officer, approver, vendor)
- [x] Arsitektur tingkat tinggi (microservices, gateway, event-driven, DB per service, cache)
- [x] Tech stack final
- [x] Diagram arsitektur awal (mermaid, `docs/architecture.md`)
- [x] Daftar service (8 service: gateway + 7 domain)

## Fase 1 — Setup repository & tooling ✅

- [x] Repository monorepo (`git init`, branch `main`)
- [x] Go module: `github.com/ank/e-proc`
- [x] Struktur folder: `cmd/`, `internal/`, `proto/`, `pkg/`, `web/`, `deploy/`, `docs/`
- [x] Docker + docker-compose: MongoDB, Redis, RabbitMQ, backend services, frontend
- [x] Linter & formatter: golangci-lint (v2 config), ESLint, Prettier
- [x] Pre-commit hook (`.pre-commit-config.yaml`)
- [x] GitHub Actions CI: lint, test, build, docker smoke build
- [x] README awal + `.env.example`
- [x] Next.js + TypeScript + Tailwind di `web/`

## Fase 2 — Desain domain & data model ✅

- [x] Entity utama: Vendor, Tender, Bid, PurchaseOrder, Approval, AuditLog, User, Role
- [x] Skema MongoDB + index → `docs/data-model.md`
- [x] Kunci Redis (cache, session, idempotency, rate limit)
- [x] Katalog event RabbitMQ → `docs/events.md`
- [x] Protobuf untuk Vendor/Tender/Bid/PO/Approval service → `proto/**/*.proto`
- [x] Review desain (self-review: kontrak proto di-lint `buf lint`, index disesuaikan pola query)

## Fase 3 — Backend: service inti ✅

- [x] Vendor service (CRUD, validasi, status) — terverifikasi end-to-end
- [x] Tender service (CRUD, publish, close, auto-close scheduler) — terverifikasi end-to-end
- [x] Auth service (login, logout dengan pencabutan token, register, me) — terverifikasi end-to-end
- [x] Bid service (submit, list per tender, perbandingan harga) — terverifikasi end-to-end
- [x] Purchase Order service (buat dari bid, status machine) — terverifikasi end-to-end
- [x] Approval service (workflow, approve/reject, history) — terverifikasi end-to-end
- [x] Audit service (append-only, actor/action/entity/metadata) — terverifikasi end-to-end
- [x] Reporting service (PO per vendor, tender, approval) — proyeksi dari event, termasuk export CSV
- [x] API Gateway (routing REST -> gRPC, agregasi) — gateway HTTP + auth di edge (catatan: hop gateway→service memakai REST dengan JWT yang sama; gRPC dipakai untuk panggilan antar-service)
- [x] Middleware: JWT, RBAC, tenant isolation, request logging, error handling
- [x] Komunikasi: REST (external), gRPC (internal), RabbitMQ (event, exchange `eproc.events` terbentuk)
- [x] Redis: cache-through dokumen tunggal + invalidasi, idempotency claim, pencabutan token

## Fase 4 — Frontend

- [x] Layout dasar (sidebar, topbar, halaman login)
- [x] Halaman: dashboard, vendor, tender, bid, PO, approval, audit, reporting
- [x] Komponen reusable (table, form, modal, badge, card, filter, pagination)
- [x] Integrasi REST API (satu klien tip untuk seluruh gateway)
- [x] State management (React Query + context auth)
- [x] RBAC di UI (menu dan aksi disembunyikan sesuai role)
- [x] Form validation (zod + react-hook-form, server tetap otoritas)
- [ ] Notifikasi real-time (opsional — belum dikerjakan)
- [x] Responsive design dengan Tailwind (grid + nav mobile)
- [ ] Halaman dokumentasi API sederhana

## Fase 5 — Integrasi & workflow otomatis ✅

- [x] Integrasi antar service via gRPC (bid-service memvalidasi tender ke tender-service sebelum menerima penawaran; `TENDER_GRPC_ADDR`, dan peringatan di log bila tidak diset)
- [x] Event RabbitMQ: publish saat PO dibuat, consumer mengubah status (po-service menerapkan keputusan approval), plus event
- [ ] Consumer notifikasi ke kanal nyata (email/Telegram) — saat ini reminder hanya berupa event + audit log
- [x] Workflow otomatis: PO di atas threshold otomatis masuk approval (consumer po.created), reminder approval, auto-close tender
- [x] Reporting otomatis: export CSV, snapshot laporan terjadwal ke koleksi `reporting_snapshots`
- [x] Audit trail end-to-end (semua event otomatis tercatat, termasuk yang dihasilkan scheduler)

## Fase 6 — Testing & QA

- [x] Unit test service Go (`go test ./...`: vendor, bid, po, approval, pkg/auth + aturan status tender)
- [x] Integration test antar service — `scripts/e2e.sh`, 35 asersi lewat gateway (dijalankan terhadap stack container: 35 PASS / 0 FAIL)
- [ ] API test dengan Bruno/Postman — digantikan skrip e2e yang bisa dijalankan ulang dan exit non-zero saat gagal
- [ ] Frontend test (Jest + React Testing Library) — belum; UI diuji lewat Playwright
- [x] E2E test (Playwright) — 7 tes di browser sungguhan, dijalankan terhadap stack container: 7 PASS. Tes ini menemukan dua celah nyata: modal belum punya `role="dialog"` (sehingga pencarian label ambigu karena Next menyimpan segmen rute yang sudah dikunjungi di DOM) dan `getByText("pending")` cocok dengan `<option>` tersembunyi alih-alih badge status — keduanya diperbaiki di komponen/spec, bukan dengan melonggarkan asersi
- [x] Load test — `scripts/loadtest` (persentil latensi, throughput, sebaran status; terukur ~780 req/s p50 3,9 ms di gateway)
- [x] Security test dasar — `scripts/security-check.sh`, 16 asersi: token kedaluwarsa/diubah/ngawur, RBAC tidak bisa dilewati, validasi input, isolasi tenant, rate limit
- [x] Bug fix & refactor — bug yang ketemu lewat pengujian: rate limiter sliding-window (lockout tanpa batas), reminder approval tidak pernah terkirim (filter tenant kosong), event `po.issued`/`approval.decided` tidak diterbitkan, `budget` tidak ikut di `tender.published`, healthcheck kena rate limit

## Fase 7 — CI/CD & deployment ⏳

- [x] CI lengkap (build, test, lint, docker build smoke) — `.github/workflows/ci.yml`
- [x] Dockerfile untuk setiap service (`Dockerfile.service` dengan build arg SERVICE, `Dockerfile.web` untuk Next.js standalone)
- [x] docker-compose "production-like": satu perintah `docker compose up -d --build` menjalankan 13 container dengan healthcheck dan urutan start terjaga
- [x] Manifest Kubernetes (opsional) — `deploy/k8s/` (namespace, configmap, secret, 9 Deployment+Service, web, ingress, contoh infra)
- [ ] Environment dev/staging/prod terpisah (baru dev yang nyata)
- [ ] Monitoring dasar (health check + logging sudah; metrics Prometheus/Grafana belum)
- [ ] Deploy ke server / cloud
- [ ] Dokumentasi deployment (baru catatan di README + deploy/k8s/README.md)

## Fase 8 — Dokumentasi & portofolio

- [ ] README lengkap (deskripsi, fitur, arsitektur, cara jalan, screenshot, live demo)
- [ ] Diagram arsitektur final
- [ ] API documentation (OpenAPI/Swagger)
- [ ] Dokumentasi database & event flow
- [ ] Artikel teknis
- [ ] Video demo
- [ ] Case study portofolio

## Fase 9 — AI-assisted development workflow

- [ ] Boilerplate service dengan AI (Cursor/Claude Code)
- [ ] Unit test & komponen frontend dengan AI
- [ ] AI code review di PR
- [ ] Dokumentasi workflow AI di README
- [ ] Semua kode AI lewat review + testing

## Fase 10 — Finalisasi & persiapan lamaran

- [ ] Semua service jalan
- [ ] Semua test lulus
- [ ] Dokumentasi lengkap
- [ ] Live demo
- [ ] Update CV, LinkedIn, portofolio
- [ ] Siapkan jawaban teknis interview
- [ ] Kirim lamaran
