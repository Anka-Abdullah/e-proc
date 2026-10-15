# Data model

Satu database MongoDB per service, dengan koleksi di bawah ini. Index ditulis
persis seperti yang dibuat service saat boot (`EnsureIndexes`).

## vendor service (`eproc_vendor`)

### `vendors`

| Field | Tipe | Catatan |
| --- | --- | --- |
| `_id` | ObjectID | |
| `tenant_id` | string | isolasi tenant |
| `code` | string | kode vendor, unik per tenant |
| `name` | string | |
| `email` | string | |
| `phone` | string | |
| `address` | string | |
| `status` | string | `pending` / `active` / `suspended` |
| `created_at`, `updated_at` | datetime | |
| `created_by` | string | actor id |

Index:

- `{tenant_id: 1, code: 1}` unique
- `{tenant_id: 1, name: 1}`
- `{tenant_id: 1, status: 1, updated_at: -1}`
- `{tenant_id: 1, email: 1}`

Pencarian teks (`?search=`) memakai regex case-insensitive pada `name`, `code`,
dan `email` — bukan text index, supaya pencarian sebagian kata (`contoh` cocok
dengan `PT Contoh Sejahtera`) langsung bekerja tanpa perlu full-text search.

## tender service (`eproc_tender`)

### `tenders`

Field inti: `tenant_id`, `code`, `title`, `description`, `items[]`
(`name`, `quantity`, `unit`, `specification`), `budget` (`amount`, `currency`),
`status`, `open_at`, `close_at`, `invited_vendor_ids[]`, `created_by`,
`created_at`, `updated_at`.

Index:

- `{tenant_id: 1, code: 1}` unique
- `{tenant_id: 1, status: 1, close_at: 1}`
- `{tenant_id: 1, invited_vendor_ids: 1}`
- text index `{title: "text", description: "text", code: "text"}`

## bid service (`eproc_bid`)

### `bids`

Field inti: `tenant_id`, `tender_id`, `vendor_id`, `items[]`
(`name`, `quantity`, `unit_price`, `total_price`), `total` (`amount`, `currency`),
`delivery_days`, `notes`, `status`, `submitted_by`, `submitted_at`, `updated_at`.

Index:

- `{tender_id: 1, vendor_id: 1}` unique — satu vendor satu penawaran per tender
- `{tender_id: 1, total.amount: 1}` — perbandingan harga
- `{vendor_id: 1, submitted_at: -1}`

## purchase order service (`eproc_po`)

### `purchase_orders`

Field inti: `tenant_id`, `number`, `tender_id`, `bid_id`, `vendor_id`,
`lines[]` (`name`, `quantity`, `unit_price`, `total_price`), `total`,
`status`, `approval_id`, `notes`, `created_by`, `created_at`, `updated_at`.

Index:

- `{tenant_id: 1, number: 1}` unique
- `{tenant_id: 1, vendor_id: 1, created_at: -1}`
- `{tenant_id: 1, status: 1, updated_at: -1}`
- `{approval_id: 1}`

## approval service (`eproc_approval`)

### `approvals`

Field inti: `tenant_id`, `entity_type`, `entity_id`, `amount`, `status`,
`requested_by`, `assigned_to`, `history[]` (`actor_id`, `decision`, `comment`,
`decided_at`), `comment`, `created_at`, `updated_at`.

Index:

- `{entity_type: 1, entity_id: 1}` unique — satu approval aktif per dokumen
- `{tenant_id: 1, status: 1, assigned_to: 1}`
- `{tenant_id: 1, created_at: -1}`

## audit service (`eproc_audit`)

### `audit_logs`

Field inti: `tenant_id`, `actor_id`, `actor_role`, `action`, `entity_type`,
`entity_id`, `before`, `after`, `metadata`, `request_id`, `ip`, `created_at`.

Koleksi ini **append-only**: service tidak menyediakan operasi update/delete.

Index:

- `{tenant_id: 1, created_at: -1}`
- `{entity_type: 1, entity_id: 1, created_at: -1}`
- `{actor_id: 1, created_at: -1}`
- TTL index opsional pada `created_at` untuk retensi (default nonaktif)

## Kenapa reporting memakai proyeksi, bukan query antar service

Database per service berarti reporting **tidak boleh** membaca koleksi milik
service lain. Jadi reporting-service berlangganan event yang sama dan
memelihara tabel baca sendiri (`reporting_purchase_orders`, `reporting_tenders`,
`reporting_approvals`). Semua handler adalah upsert berkunci id entitas, jadi
pengiriman ulang event aman. Konsekuensinya: laporan selalu eventual-consistent
— beberapa detik di belakang kebenaran — dan itu sifat yang disengaja, bukan bug.

## Kunci Redis

| Pola | Isi | TTL |
| --- | --- | --- |
| `cache:vendor:<id>` | dokumen vendor | 5 menit |
| `cache:tender:<id>` | dokumen tender | 5 menit |
| `cache:tender:list:<tenant>:<hash>` | halaman daftar tender | 60 detik |
| `cache:report:po-summary:<tenant>` | agregat laporan PO | 10 menit |
| `session:<token>` | subject pemilik sesi | sesuai TTL JWT |
| `idem:<key>` | penanda idempotency request | 24 jam |
| `ratelimit:<ip>` | counter request per menit | 60 detik |

Invalidasi: setiap operasi tulis menghapus kunci `cache:` yang relevan
(`Delete` dipanggil setelah write sukses), bukan menunggu TTL.
