# Deployment

Dua jalur yang sudah disiapkan: Docker Compose (cara utama) dan Kubernetes
(opsional). Keduanya memakai image yang sama dan sinyal kesehatan yang sama.

## 1. Docker Compose — satu perintah

```bash
docker compose up -d --build          # 13 container: infra, 8 service, gateway, web
docker compose ps                     # tunggu sampai semua "healthy"
docker compose logs -f api-gateway
docker compose down                   # hentikan, volume tetap
docker compose down -v                # hentikan dan hapus data
```

Urutan start dijaga `depends_on: condition: service_healthy`: gateway baru
menyala setelah 8 upstream menjawab `/healthz`, dan web setelah gateway sehat.
Konsekuensinya `ps` yang hijau berarti API memang siap.

### Port di host

| Layanan | Port |
| --- | --- |
| Gateway API | 8000 |
| Service domain | 8001–8008 (masing-masing juga membuka gRPC 9081–9087, 9098, 9099) |
| Web | 3000 |
| MongoDB | 27017 |
| Redis | **6380** (6379 sering sudah dipakai Redis lain di mesin developer) |
| RabbitMQ | 5672, UI 15672 |

### Sebelum dipakai serius

1. `cp .env.example .env`, isi nilai sebenarnya.
2. **`JWT_SECRET` wajib diganti.** Dengan `APP_ENV=production`, service menolak
   boot selama nilainya masih default pengembangan — jadi salah konfigurasi
   ketahuan saat start, bukan setelah data bocor.
3. Ganti kredensial RabbitMQ dan MongoDB.
4. Batasi CORS: `FRONTEND_ORIGIN` hanya boleh origin web yang sebenarnya.
5. Letakkan gateway di belakang TLS. Gateway sudah meneruskan `X-Forwarded-*`
   (proxy menulisnya lewat `SetXForwarded`), jadi rate limiter membaca IP klien
   yang benar, bukan IP reverse proxy.

## 2. Kubernetes — opsional

Lihat [`deploy/k8s/README.md`](../deploy/k8s/README.md) untuk urutan apply dan
daftar yang harus diisi. Poin pentingnya:

- Hanya `api-gateway` dan `web` yang diekspos lewat ingress; service domain tetap
  internal. Kalau service domain dibuka langsung, RBAC di gateway kehilangan arti.
- `tender-service`, `approval-service`, `reporting-service` **1 replika** karena
  scheduler-nya berjalan di dalam proses. Menaikkan replika tanpa lease membuat
  sweep ganda (tender tertutup dua kali, reminder terkirim dua kali).
- Probe memakai `/healthz` (liveness) dan `/readyz` (readiness) yang sama dengan
  compose, termasuk `/readyz` gateway yang melaporkan status tiap upstream.

## Ukuran image

Service Go dibangun sebagai binary statis di atas base `scratch` — tanpa shell,
tanpa package manager, berjalan sebagai uid 10001.

| Service | Ukuran |
| --- | --- |
| api-gateway | 30,7 MB |
| auth-service | 33,1 MB |
| audit-service | 33,0 MB |
| reporting-service | 33,1 MB |
| vendor, tender, po, approval, bid | 34,0–34,2 MB |

Isinya hanya tiga hal: binary (24 MB — sisa terbesar, konsekuensi dari gRPC +
driver MongoDB yang di-link statis), database zona waktu (1,5 MB), dan berkas
sertifikat TLS (184 kB).

Karena tidak ada `wget`/`curl` di dalam image, healthcheck container dijalankan
oleh binary-nya sendiri: `["CMD", "/service", "-healthcheck"]`. Mode itu
memeriksa `/healthz` milik service dan keluar 0/1 — lihat `pkg/app/healthcheck.go`.

Catatan kalau ukuran jadi masalah: 24 MB itu memang binary-nya. Menekannya lebih
jauh berarti memakai UPX (kompresi, menambah waktu start dan sering ditandai
pemindai keamanan) atau melepas dependensi — keduanya keputusan sadar, bukan
sesuatu yang pantas dilakukan diam-diam.

## Pemecahan masalah

### Build berhenti dengan `rpc error: ... EOF`, `database not open`, atau container tiba-tiba `Up N seconds`

Itu tanda **VM Docker-nya dimatikan di tengah build**, bukan Dockerfile yang salah.
Di mesin ini (RAM 8 GB) membangun sepuluh image secara paralel menghabiskan
memori, VM OrbStack dimatikan, dan BuildKit melaporkan error transportasi — lalu
semua container (termasuk infrastruktur) restart bersamaan. Ciri khasnya: beberapa
`buildkit` target dibatalkan (`CANCELED`) tanpa error kompilasi.

Bangun berurutan, lalu jalankan:

```bash
COMPOSE_PARALLEL_LIMIT=1 docker compose build    # satu image pada satu waktu
docker compose up -d                             # hanya membuat container, tidak build
```

Pemeriksaan cepat kalau gejalanya muncul lagi:

```bash
docker info --format '{{.ServerVersion}} running={{.ContainersRunning}}'   # daemon hidup?
docker run --rm alpine df -h /                                             # ruang di dalam VM
docker system df                                                           # image + cache
memory_pressure | tail -3                                                  # tekanan memori host
```

### Unduhan lambat atau gagal di tengah (`stream error`, `UND_ERR_SOCKET`)

Kedua Dockerfile sudah memakai BuildKit cache mount (module cache Go dan pnpm
store) plus retry terbatas, sehingga percobaan ulang memakai yang sudah terunduh.
Kalau jaringan sangat lambat, selesaikan per service:

```bash
COMPOSE_PARALLEL_LIMIT=1 docker compose build web     # image terberat, sekali saja
```

### Port sudah dipakai

Gejalanya `Bind for 0.0.0.0:XXXX failed: port is already allocated`. Lihat tabel
port di atas; Redis sengaja di `6380` dan audit-service di `8006` karena `6379`
dan `8086` sering sudah dipakai layanan lain.

## Rahasia dan konfigurasi

Semua konfigurasi lewat environment variable (`pkg/config`). Yang tidak boleh
masuk repo: `JWT_SECRET`, kredensial RabbitMQ/MongoDB, dan token apa pun.

Di Kubernetes, lebih baik membuat secret secara imperatif supaya nilainya tidak
pernah singgah di disk:

```bash
kubectl -n eproc create secret generic eproc-secrets \
  --from-literal=JWT_SECRET="$(openssl rand -base64 48)"
```

## Data dan backup

- MongoDB menyimpan data utama. Backup: `docker compose exec mongo mongodump
  --archive=/tmp/dump.gz --gzip` lalu salin keluar, atau `mongodump` dari host ke
  `localhost:27017`.
- Redis menyimpan cache, sesi, penanda idempotency, dan counter rate limit —
  **boleh hilang**: semua bisa dibangun ulang. Yang perlu diperhatikan: sesi yang
  hilang berarti pengguna harus login ulang, dan penanda idempotency yang hilang
  membuka peluang pemrosesan ulang event (index unik di MongoDB tetap menahan
  duplikasi).
- RabbitMQ: queue bersifat durable. Pesan yang gagal di-`Nack` tanpa requeue dan
  tercatat di log — **bukan** masuk dead-letter queue otomatis, jadi bila butuh
  DLQ itu langkah berikutnya yang belum dikerjakan.

## Kesehatan dan pengamatan

- `GET /healthz` — liveness, tidak menyentuh dependensi.
- `GET /readyz` — readiness: memeriksa Mongo/Redis, dan di gateway memeriksa
  8 upstream sekaligus. Endpoint ini tidak dihitung rate limiter.
- Log terstruktur (`slog`): JSON di `APP_ENV=production`, teks di development.
  Setiap permintaan punya `request_id` yang diteruskan ke service di belakang
  gateway (`X-Request-Id`).
- Audit trail: setiap aksi penting tercatat append-only di `audit_logs`, termasuk
  aksi yang dilakukan scheduler.

Yang belum ada: metrik Prometheus/Grafana, tracing terdistribusi, dan notifikasi
ke kanal nyata (email/Telegram). Ketiganya sudah ditandai di `docs/plan.md`.
