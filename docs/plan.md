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

## Fase 3 — Backend: service inti ⏳

- [x] Vendor service (CRUD, validasi, status) — terverifikasi end-to-end
- [ ] Tender service (CRUD, publish, close, auto-close scheduler)
- [ ] Bid service (submit, list per tender, perbandingan harga)
- [ ] Purchase Order service (buat dari bid, status machine)
- [ ] Approval service (workflow, approve/reject, history)
- [ ] Audit service (append-only, actor/action/entity/metadata)
- [ ] Reporting service (PO per vendor, tender, approval)
- [ ] Auth service (login, refresh, logout, manajemen user) — tambahan di luar daftar awal
- [ ] API Gateway (routing REST -> gRPC, agregasi)
- [x] Middleware: JWT, RBAC, tenant isolation, request logging, error handling
- [x] Komunikasi: REST (external), gRPC (internal), RabbitMQ (event, exchange `eproc.events` terbentuk)
- [x] Redis: cache-through dokumen tunggal + invalidasi, idempotency claim, pencabutan token

## Fase 4 — Frontend

- [ ] Layout dasar (sidebar, topbar, halaman login)
- [ ] Halaman: dashboard, vendor, tender, bid, PO, approval, audit, reporting
- [ ] Komponen reusable (table, form, modal, badge, card, filter, pagination)
- [ ] Integrasi REST API
- [ ] State management (React Query + context auth)
- [ ] RBAC di UI (sembunyikan menu & aksi)
- [ ] Form validation
- [ ] Notifikasi real-time (opsional)
- [ ] Responsive (Tailwind)
- [ ] Halaman dokumentasi API

## Fase 5 — Integrasi & workflow otomatis

- [ ] Integrasi antar service via gRPC
- [ ] Event RabbitMQ (publish, consumer status, consumer notifikasi)
- [ ] Workflow otomatis (PO → approval di atas threshold, reminder, auto-close tender)
- [ ] Reporting otomatis (export CSV/PDF, scheduled report)
- [ ] Audit trail end-to-end

## Fase 6 — Testing & QA

- [ ] Unit test service Go
- [ ] Integration test antar service
- [ ] API test (Bruno/Postman collection di repo)
- [ ] Frontend test (Jest + React Testing Library)
- [ ] E2E test (Playwright)
- [ ] Load test (k6)
- [ ] Security test (JWT expiry, RBAC bypass, input validation, rate limit)
- [ ] Bug fix & refactor

## Fase 7 — CI/CD & deployment

- [ ] CI lengkap (build, test, lint, docker build, push image)
- [ ] Dockerfile per service
- [ ] docker-compose production
- [ ] Manifest Kubernetes (opsional)
- [ ] Environment dev/staging/prod
- [ ] Monitoring dasar (health check, logging, metrics)
- [ ] Deploy + dokumentasi deployment

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
