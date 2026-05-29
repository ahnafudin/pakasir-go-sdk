# Pakasir Go SDK

> Klien Go yang ringan, idiomatis, dan tanpa dependensi eksternal untuk gerbang pembayaran [Pakasir](https://pakasir.com).

[![Go Reference](https://pkg.go.dev/badge/github.com/ahnafudin/pakasir-go-sdk.svg)](https://pkg.go.dev/github.com/ahnafudin/pakasir-go-sdk)
[![Go Report Card](https://goreportcard.com/badge/github.com/ahnafudin/pakasir-go-sdk)](https://goreportcard.com/report/github.com/ahnafudin/pakasir-go-sdk)
[![Go Version](https://img.shields.io/badge/go-1.26.3%2B-00ADD8?logo=go)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](./LICENSE)

SDK ini mendukung QRIS dan Virtual Account multi-bank (BNI, BRI, CIMB Niaga,
Maybank, Permata, BNC, Sampoerna, ATM Bersama, Artha Graha) dengan API yang
type-safe, helper polling status real-time (`Watch`), dan validator webhook yang
mengikuti panduan keamanan resmi Pakasir — semuanya dalam satu paket tanpa
dependensi pihak ketiga.

---

## ⚡ Fitur Unggulan

- **Tanpa dependensi non-stdlib.** Hanya pustaka standar Go. Tidak ada yang perlu
  diaudit, tidak ada risiko rantai pasok (supply chain).
- **Satu paket, satu import.** Seluruh permukaan API ada di `package pakasir` —
  `godoc` muat dalam satu halaman.
- **Type-safe & IDE-friendly.** Setiap respons API dipetakan ke struct `Payment`
  dan `Event` yang bertipe; metode pembayaran dan status berupa enum.
- **Helper `Watch` real-time.** Pantau status transaksi lewat channel Go untuk
  UI "menunggu pembayaran" — dengan dedup status dan pembatalan via `context`.
- **Proteksi webhook.** `Event.Match` / `Event.Verify` memvalidasi `order_id`
  dan `amount` sesuai panduan resmi Pakasir; tolak yang mencurigakan dengan 403.
- **Idiomatis.** `context.Context` di parameter pertama, functional options,
  error bertipe (`errors.Is` / `errors.As`), dan injeksi `http.Client` kustom.
- **Deterministik.** Tanpa retry tersembunyi; pasang `http.RoundTripper` sendiri
  bila butuh retry/backoff.

## 📦 Instalasi

Pastikan Anda menggunakan **Go 1.26.3 atau lebih baru**, lalu jalankan:

```bash
go get github.com/ahnafudin/pakasir-go-sdk
```

Impor paketnya:

```go
import pakasir "github.com/ahnafudin/pakasir-go-sdk"
```

## 🛠️ Konfigurasi

Buat klien dengan project slug dan API key dari dashboard Pakasir Anda:

```go
client := pakasir.New("slug-proyek-anda", "api-key-rahasia-anda")
```

Klien aman digunakan secara konkuren oleh banyak goroutine. Sesuaikan perilaku
lewat functional options:

```go
client := pakasir.New("slug-proyek-anda", "api-key-rahasia-anda",
    pakasir.WithTimeout(15*time.Second),        // timeout HTTP (default 30s)
    pakasir.WithUserAgent("aplikasi-saya/1.0"), // User-Agent kustom
    pakasir.WithLogger(slog.Default()),         // logging non-fatal (default: senyap)
    pakasir.WithHTTPClient(customClient),        // *http.Client kustom (retry/proxy/tracing)
    pakasir.WithBaseURL("http://localhost:8080"),// untuk mock server / proxy (BUKAN sandbox)
)
```

> **Sandbox vs Produksi:** Pakasir tidak menyediakan host sandbox terpisah.
> Sandbox diaktifkan per-proyek di dashboard, dan payload webhook membawa flag
> `IsSandbox`. Gunakan proyek sandbox dengan API key-nya sendiri — bukan
> `WithBaseURL`.

> ⚠️ **Keamanan:** `DetailPayment` mengirim `api_key` sebagai query parameter URL
> sesuai spesifikasi Pakasir. Key bisa muncul di log akses HTTP — panggil hanya
> dari kode sisi server, jangan dari frontend.

## 🚀 Penggunaan Cepat

### 1. Membuat Transaksi Pembayaran

Gunakan konstanta `Method` untuk validasi dan autocompletion:

```go
ctx := context.Background()

payment, err := client.CreatePayment(
    ctx,
    pakasir.MethodQRIS,        // metode pembayaran
    "INV-1700000000",          // order_id unik milik Anda
    50_000,                    // nominal (IDR)
)
if err != nil {
    log.Fatal(err)
}

fmt.Println(payment.PaymentNumber) // kode QRIS atau nomor VA
fmt.Println(payment.Fee)           // biaya gateway (dari respons API)
fmt.Println(payment.TotalPayment)  // nominal + biaya
if payment.ExpiredAt != nil {
    fmt.Println(payment.ExpiredAt)  // batas waktu pembayaran
}
```

### 2. Membangun URL Pembayaran (tanpa panggilan API)

`GetPaymentURL` menyusun URL halaman pembayaran ter-host tanpa request jaringan:

```go
url := client.GetPaymentURL(
    pakasir.MethodQRIS,
    "INV-1700000000",
    50_000,
    pakasir.WithRedirectURL("https://situs-anda.com/invoice/selesai"), // opsional
    pakasir.WithQRISOnly(true),                                        // opsional
)
// https://app.pakasir.com/pay/{slug}/50000?order_id=INV-1700000000&...
```

### 3. Memantau Status Secara Real-time (`Watch`)

`Watch` melakukan polling sampai status final, `context` dibatalkan, atau terjadi
error permanen. Cocok untuk UI "menunggu pembayaran":

```go
watchCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
defer cancel()

for evt := range client.Watch(watchCtx, "INV-1700000000", 50_000) {
    if evt.Err != nil {
        log.Printf("watch error: %v", evt.Err)
        break
    }
    fmt.Println("status:", evt.Payment.Status)
    if evt.Payment.Status.IsTerminal() {
        break
    }
}
```

Channel ditutup tepat satu kali. Untuk membatasi durasi, bungkus `context`
dengan `context.WithTimeout` — tidak ada opsi timeout terpisah.

### 4. Memproses & Memverifikasi Webhook Secara Aman

Pakasir tidak menandatangani webhook. Validasi setiap callback dengan `Verify`
(versi yang mengembalikan error) atau `Match` (versi boolean):

```go
func handleWebhook(w http.ResponseWriter, r *http.Request) {
    event, err := pakasir.ParseWebhookRequest(r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Ambil nominal asli dari database Anda berdasarkan event.OrderID.
    expectedAmount := lookupAmount(event.OrderID)

    if err := event.Verify(event.OrderID, expectedAmount); err != nil {
        // order_id / amount tidak cocok → tolak dengan 403.
        http.Error(w, err.Error(), http.StatusForbidden)
        return
    }

    // Lolos verifikasi → proses efek samping secara idempoten pada event.Status.
    switch event.Status {
    case pakasir.StatusCompleted:
        markOrderPaid(event.OrderID, event.CompletedAt)
    }

    w.WriteHeader(http.StatusOK)
}
```

## 💳 Metode Pembayaran

SDK menyediakan konstanta `Method` bertipe. `Method.IsValid()` memeriksa nilai
yang tidak dikenal, dan `AllMethods()` mengembalikan seluruh daftar:

| Metode          | Konstanta Go                 | Kode Wire        | Biaya¹           |
|-----------------|------------------------------|------------------|------------------|
| QRIS            | `pakasir.MethodQRIS`         | `qris`           | 0,7% + Rp 310²   |
| BNI VA          | `pakasir.MethodBNIVA`        | `bni_va`         | Rp 3.500         |
| BRI VA          | `pakasir.MethodBRIVA`        | `bri_va`         | Rp 3.500         |
| CIMB Niaga VA   | `pakasir.MethodCIMBVA`       | `cimb_niaga_va`  | Rp 3.500         |
| Maybank VA      | `pakasir.MethodMaybankVA`    | `maybank_va`     | Rp 3.500         |
| Permata VA      | `pakasir.MethodPermataVA`    | `permata_va`     | Rp 3.500         |
| BNC VA          | `pakasir.MethodBNCVA`        | `bnc_va`         | Rp 3.500         |
| ATM Bersama VA  | `pakasir.MethodATMBersamaVA` | `atm_bersama_va` | Rp 3.500         |
| Sampoerna VA    | `pakasir.MethodSampoernaVA`  | `sampoerna_va`   | Rp 2.000         |
| Artha Graha VA  | `pakasir.MethodArthaGrahaVA` | `artha_graha_va` | Rp 2.000         |

> ¹ Biaya per [pricing resmi Pakasir](https://pakasir.com/p/pricing)
> (diperbarui 22 Mei 2026); dapat berubah sewaktu-waktu — selalu rujuk sumber
> resmi. SDK tidak menghitung biaya: `Fee` dan `TotalPayment` diambil dari
> respons `CreatePayment` yang dikembalikan server Pakasir.
> ² Untuk nominal di atas Rp 105.000, biaya QRIS menjadi 1% + Rp 0.

## 🔄 Status Transaksi

| Status            | Konstanta Go             | Final? |
|-------------------|--------------------------|--------|
| Menunggu bayar    | `pakasir.StatusPending`      | tidak  |
| Selesai           | `pakasir.StatusCompleted`    | ya     |
| Dibatalkan        | `pakasir.StatusCancelled`    | ya     |
| Kedaluwarsa       | `pakasir.StatusExpired`      | ya     |

- `Status.IsTerminal()` — `true` bila tidak ada perubahan lagi yang diharapkan.
- `Status.IsKnown()` — `false` untuk status yang mungkin ditambahkan Pakasir di
  masa depan, sehingga Anda dapat menanganinya secara defensif.

## 📖 API Reference

```go
// Konstruktor
func New(slug, apiKey string, opts ...Option) *Client

// Operasi pembayaran
func (c *Client) CreatePayment(ctx context.Context, method Method, orderID string, amount int64, opts ...PaymentOption) (*Payment, error)
func (c *Client) GetPaymentURL(method Method, orderID string, amount int64, opts ...PaymentOption) string
func (c *Client) DetailPayment(ctx context.Context, orderID string, amount int64) (*Payment, error)
func (c *Client) CancelPayment(ctx context.Context, orderID string, amount int64) (*Payment, error)
func (c *Client) SimulatePayment(ctx context.Context, orderID string, amount int64) (*Payment, error)

// Polling real-time
func (c *Client) Watch(ctx context.Context, orderID string, amount int64, opts ...WatchOption) <-chan WatchEvent

// Webhook
func ParseWebhook(body []byte) (*Event, error)
func ParseWebhookRequest(r *http.Request, opts ...WebhookOption) (*Event, error)
func (e *Event) Match(orderID string, expectedAmount int64) bool
func (e *Event) Verify(orderID string, expectedAmount int64) error
```

| Fungsi              | Keterangan |
|---------------------|------------|
| `CreatePayment`     | Membuat transaksi baru secara real-time. Mengembalikan `*Payment`. |
| `GetPaymentURL`     | Menyusun URL pembayaran ter-host (tanpa panggilan API). |
| `DetailPayment`     | Mengambil status transaksi terkini. **Server-side saja** (api_key di URL). |
| `CancelPayment`     | Membatalkan transaksi yang masih pending. |
| `SimulatePayment`   | Mensimulasikan pembayaran sukses (khusus mode sandbox). |
| `Watch`             | Polling status sampai final / `context` batal; streaming via channel. |
| `ParseWebhook`      | Mem-parse body webhook (`[]byte`) menjadi `*Event`. |
| `ParseWebhookRequest` | Mem-parse dari `*http.Request` dengan batas ukuran body (default 1 MiB). |
| `Event.Match`       | `true` jika `order_id` dan `amount` cocok. |
| `Event.Verify`      | `nil` jika cocok; `ErrOrderIDMismatch` / `ErrAmountMismatch` jika tidak. |

Dokumentasi lengkap tiap simbol tersedia di
[pkg.go.dev](https://pkg.go.dev/github.com/ahnafudin/pakasir-go-sdk).

## ⚠️ Penanganan Error

Error HTTP non-2xx dikembalikan sebagai `*APIError` (gunakan `errors.As`):

```go
payment, err := client.DetailPayment(ctx, "INV-1", 50_000)
var apiErr *pakasir.APIError
if errors.As(err, &apiErr) {
    log.Printf("HTTP %d: %s", apiErr.StatusCode, apiErr.Message)
}
```

Error sentinel yang dapat dicocokkan dengan `errors.Is`:

| Sentinel               | Penyebab |
|------------------------|----------|
| `ErrInvalidOrderID`    | `order_id` kosong sebelum request dikirim. |
| `ErrInvalidAmount`     | `amount` bukan bilangan positif. |
| `ErrInvalidMethod`     | Metode pembayaran tidak dikenal. |
| `ErrEmptyBody`         | Body webhook kosong. |
| `ErrBodyTooLarge`      | Body webhook melebihi batas ukuran. |
| `ErrOrderIDMismatch`   | `order_id` webhook tidak cocok (`Event.Verify`). |
| `ErrAmountMismatch`    | `amount` webhook tidak cocok (`Event.Verify`). |

## 🔐 Keamanan

Pakasir **tidak** menandatangani webhook. SDK menyediakan `Event.Match` /
`Event.Verify` sesuai panduan resmi Pakasir ("pastikan amount dan order_id
sesuai") — validasi setiap webhook terhadap catatan Anda sendiri dan tolak yang
tidak cocok dengan HTTP 403. Untuk pertahanan berlapis terhadap replay atau
pemalsuan: batasi endpoint webhook di edge (CDN / reverse proxy) ke IP sumber
Pakasir, dan terapkan idempotensi pada `order_id` di database Anda.

`DetailPayment` mengirim `api_key` sebagai query parameter URL (sesuai spesifikasi
Pakasir). Key dapat muncul di log akses HTTP — panggil hanya dari kode sisi
server tepercaya, jangan dari frontend.

## 🤝 Kontribusi

Kontribusi disambut baik. Silakan buka issue untuk mendiskusikan perubahan besar
sebelum mengirim pull request. Pastikan `go vet`, `go test -race ./...`, dan
`staticcheck` lolos.

## 📜 Lisensi

Didistribusikan di bawah [MIT License](./LICENSE) — Copyright (c) 2026 Ahnafudin.
