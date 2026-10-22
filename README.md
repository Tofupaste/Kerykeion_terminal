# KantorChat (P2P LAN)

KantorChat adalah aplikasi obrolan (*chat room*) dan transfer file terdistribusi (P2P) berbasis terminal yang beroperasi langsung antar-laptop di jaringan lokal kantor (LAN), **tanpa koneksi internet dan tanpa server pusat atau VPS**.

---

## Fitur Utama

- **Pesan Pribadi (DM) & Room Tanpa Host**: Masuk room cukup mengetik `/join <nama_room>` (misal `/join ngobs`). Room tetap aktif selama minimal 1 anggota terhubung.
- **Kirim Berkas & Media dengan Resume Otomatis**: Berkas dipecah menjadi *chunk* 256 KB dengan verifikasi integritas penuh SHA-256 dan pelacakan bitmap di disk. Jika koneksi WiFi terputus (saat berpindah Access Point), pengiriman akan otomatis melanjutkan (*resume*) tanpa mengulang dari awal.
- **Model Media di Room (Umumkan lalu Tarik)**: Gambar diumumkan beserta *thumbnail* ringan (≤ 8 KB). Berkas diunduh dengan `/get <nomor>`, mendukung multi-sumber dari anggota room lain saat pengirim asli sedang luring (*offline*).
- **Keamanan & Kriptografi**:
  - Identitas persisten berbasis kunci **Ed25519** tersimpan di direktori pengguna (`os.UserConfigDir`) dengan hak akses `0600`.
  - `peer_id` = sidik jari SHA-256 dari kunci publik.
  - Setiap pesan dan peristiwa room ditandatangani secara digital (*cryptographically signed*).
  - Enkripsi transport **TLS 1.3** dengan sertifikat *self-signed* dan penyematan sidik jari (*fingerprint pinning* / TOFU).
- **Penyimpanan Lokal & Retensi 30 Hari**: Basis data lokal SQLite (*pure Go* tanpa CGO melalui `modernc.org/sqlite`) beroperasi dalam mode WAL dengan pemangkasan otomatis riwayat lebih dari 30 hari.
- **Diagnosa Mandiri (`/doctor`)**: Alat pengecekan interface jaringan, port binding TCP/UDP, broadcast, serta petunjuk konfigurasi firewall Windows/Linux/macOS dalam bahasa yang mudah dipahami.
- **Antarmuka Ganda (TUI & REPL)**: Antarmuka visual penuh terminal (*Bubble Tea TUI*) dengan sidebar room dan rekan, serta mode REPL baris perintah untuk pengujian otomatis atau terminal minimalis.
- **Distribusi Tunggal Tanpa Ketergantungan**: Satu file biner mandiri (*single static binary* < 20 MB) untuk Windows, Linux, dan macOS.

---

## Struktur Direktori Proyek

```
.
├── PRD.md                       # Dokumen Persyaratan Produk
├── README.md                    # Dokumentasi Penggunaan
├── cmd/
│   └── kantorchat/              # Titik masuk program utama (main.go & e2e_test.go)
└── pkg/
    ├── config/                  # Pengaturan direktori & konfigurasi aplikasi
    ├── db/                      # SQLite (modernc), skema, indeks, & retensi 30 hari
    ├── discovery/               # Penemuan peer berbasis broadcast UDP, multicast, & deteksi interface
    ├── doctor/                  # Diagnosa jaringan LAN, status port, & saran firewall
    ├── identity/                # Kunci Ed25519, peer_id, tanda tangan digital, & verifikasi
    ├── messaging/               # Pesan teks DM, fan-out room, outbox dengan backoff, & ekstraksi tautan
    ├── protocol/                # Framing biner, tipe payload, & serialisasi
    ├── room/                    # Siklus hidup room tanpa host, sinkronisasi riwayat, & tombstone
    ├── transfer/                # Transfer file per-chunk (256 KB), bitmap resume, sanitasi nama, & thumbnail
    ├── transport/               # Koneksi TCP + TLS 1.3 dengan fingerprint pinning
    └── ui/                      # TUI Bubble Tea, REPL baris perintah, katalog /help, & autokompleksi Tab
```

---

## Cara Kompilasi

Pastikan telah terpasang compiler Go (versi 1.22 atau yang lebih baru).

### 1. Kompilasi untuk Sistem Saat Ini (Linux)
```bash
go build -ldflags="-s -w" -o kantorchat ./cmd/kantorchat
```

### 2. Kompilasi Silang untuk Windows (64-bit)
```bash
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o kantorchat.exe ./cmd/kantorchat
```

### 3. Kompilasi Silang untuk macOS (Apple Silicon arm64)
```bash
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o kantorchat-darwin-arm64 ./cmd/kantorchat
```

---

## Cara Menjalankan

### Menjalankan Antarmuka Visual (TUI)
```bash
./kantorchat
```

### Menjalankan Diagnosa Jaringan
```bash
./kantorchat doctor
```

### Langsung Masuk ke Room Tertentu
```bash
./kantorchat join ngobs
```

### Menjalankan Mode REPL (Baris Perintah)
```bash
./kantorchat --repl
```

---

## Perintah di Dalam Aplikasi

| Perintah | Fungsi | Contoh |
|---|---|---|
| `/help` | Menampilkan panduan bantuan perintah | `/help` atau `/help join` |
| `/who` | Menampilkan rekan yang sedang online di LAN | `/who` |
| `/dm <rekan>` | Beralih ke pesan pribadi dengan rekan | `/dm aria` |
| `/join <room>` | Masuk ke room (buat baru bila belum ada) | `/join ngobs` |
| `/room new <room>` | Membuat room baru secara eksplisit | `/room new proyek-a` |
| `/room list` | Melihat daftar room yang diikuti | `/room list` |
| `/room info <room>` | Melihat informasi detail dan anggota room | `/room info ngobs` |
| `/room transfer <room> <rekan>` | Mengalihkan peran master room ke rekan lain | `/room transfer ngobs budi` |
| `/go <target>` | Berpindah fokus percakapan aktif | `/go ngobs` |
| `/send <path_file>` | Mengirim file atau gambar ke percakapan aktif | `/send /home/user/foto.jpg` |
| `/get <no>` | Mengunduh file dari pengumuman room | `/get 1` |
| `/leave [room]` | Keluar dari room saat ini (hanya diri sendiri) | `/leave ngobs` |
| `/disband [room]` | Membubarkan room secara permanen (khusus master) | `/disband ngobs` |
| `/name <baru>` | Mengubah nama tampilan kamu di jaringan | `/name Budi` |
| `/open <no>` | Membuka tautan web yang diterima di peramban | `/open 1` |
| `/doctor` | Memeriksa status port, jaringan, dan firewall | `/doctor` |
| `/add <ip:port>` | Menambahkan IP rekan secara manual bila broadcast dibatasi | `/add 10.10.40.50:42100` |
| `/quit` | Keluar dari aplikasi KantorChat | `/quit` |

---

## Menjalankan Pengujian Otomatis

Seluruh pengujian unit dan pengujian integrasi *End-to-End* (E2E) dapat dijalankan dengan:

```bash
go test -v ./...
```
Semua modul mencakup pengujian kunci kriptografi, *framing protocol*, integritas SQLite, sanitasi nama berkas, mekanisme *resume* berkas dari disk bitmap, serta komunikasi pertukaran pesan TLS dua arah.
