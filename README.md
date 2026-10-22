# Kerykeion Terminal

Aplikasi chat dan transfer berkas P2P untuk jaringan lokal kantor (LAN), berjalan langsung antar-laptop tanpa server pusat atau koneksi internet.

## Cara Pasang

### Menggunakan Installer (Rekomendasi)

Jalankan script installer di root direktori:

```bash
git clone https://github.com/your-org/kerykeion_terminal.git
cd kerykeion_terminal
./install.sh
```

Script ini mendeteksi distro Linux kamu (CachyOS, Fedora, Ubuntu, dll.), mengompilasi biner `kerxt`, menyalinnya ke `~/.local/bin/kerxt`, dan memastikan direktori tersebut sudah masuk ke `$PATH`.

Jika biner `kerxt` sudah ada di sistem, kamu juga bisa menjalankan:
```bash
kerxt install
```

### Manual per Distro

**CachyOS / Arch Linux**
```bash
sudo pacman -S go git
go build -ldflags="-s -w" -o ~/.local/bin/kerxt ./cmd/kerxt
```

**Fedora**
```bash
sudo dnf install -y golang git
go build -ldflags="-s -w" -o ~/.local/bin/kerxt ./cmd/kerxt
```

**Kompilasi Silang (Windows & macOS)**
```bash
# Windows (64-bit)
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o kerxt.exe ./cmd/kerxt

# macOS (Apple Silicon arm64)
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o kerxt-darwin-arm64 ./cmd/kerxt
```

### Menghapus Instalasi (Uninstall)

```bash
# Hapus biner saja
kerxt uninstall

# Hapus biner beserta database riwayat dan cache
kerxt uninstall --purge
```

## Penggunaan

Jalankan antarmuka visual (TUI):
```bash
kerxt
```

Pilihan perintah lain:
```bash
kerxt doctor        # Cek interface, port binding, dan saran firewall
kerxt join ngobs    # Langsung masuk atau buat room tertentu
kerxt --repl        # Mode baris perintah sederhana tanpa TUI
```

## Perintah di Dalam Aplikasi

| Perintah | Keterangan | Contoh |
|---|---|---|
| `/help` | Menampilkan panduan perintah | `/help` atau `/help join` |
| `/who` | Menampilkan rekan yang online di LAN | `/who` |
| `/dm <rekan>` | Membuka pesan pribadi ke rekan | `/dm aria` |
| `/join <room>` | Masuk ke room atau buat baru bila belum ada | `/join ngobs` |
| `/room new <nama>` | Membuat room baru secara eksplisit | `/room new proyek-a` |
| `/room list` | Menampilkan daftar room yang diikuti | `/room list` |
| `/room info <nama>` | Menampilkan detail anggota room | `/room info ngobs` |
| `/room transfer <nama> <rekan>` | Mengalihkan peran master room ke rekan lain | `/room transfer ngobs budi` |
| `/go <target>` | Berpindah percakapan aktif | `/go ngobs` |
| `/send <path>` | Mengirim berkas atau gambar ke percakapan aktif | `/send ~/foto.png` |
| `/get <no>` | Mengunduh berkas dari daftar room | `/get 1` |
| `/leave [room]` | Keluar dari room saat ini | `/leave ngobs` |
| `/disband [room]` | Membubarkan room permanen (khusus master) | `/disband ngobs` |
| `/name <baru>` | Mengubah nama tampilan kamu di jaringan | `/name Budi` |
| `/open <no>` | Membuka tautan web di browser default | `/open 1` |
| `/doctor` | Cek status interface, port, dan firewall | `/doctor` |
| `/add <ip:port>` | Hubungkan manual bila broadcast diblokir access point | `/add 10.10.40.50:42100` |
| `/quit` | Keluar dari aplikasi | `/quit` |

## Karakteristik & Protokol

- **Room Tanpa Host**: Room tetap aktif selama minimal satu anggota terhubung. Setiap peristiwa room ditandatangani secara digital oleh pembuatnya sehingga riwayat valid dapat diteruskan oleh anggota mana pun.
- **Resume Transfer File**: Berkas dipecah menjadi potongan 256 KB dengan verifikasi hash SHA-256 dan bitmap di disk. Transfer otomatis melanjutkan bagian yang tersisa saat koneksi WiFi pulih setelah berpindah access point.
- **Model Media Room**: Gambar diumumkan beserta thumbnail ringan (≤ 8 KB). Anggota lain mengunduh dengan `/get <nomor>`, dan siapa pun yang sudah selesai mengunduh otomatis menjadi sumber tambahan (*multi-source*).
- **Keamanan & Privasi**: Kunci privat Ed25519 tersimpan dengan izin `0600`. Koneksi antar-laptop dienkripsi TLS 1.3 dengan fingerprint pinning, dan `peer_id` dihitung dari SHA-256 public key.
- **Penyimpanan Lokal**: Database SQLite berbasis pure Go (`modernc.org/sqlite`) beroperasi dalam mode WAL dengan pemangkasan otomatis data yang berumur lebih dari 30 hari.

## Lokasi Data

- **Konfigurasi & Database**:
  - Linux: `~/.config/kerykeion/` (`identity.key`, `config.json`, `history.db`)
  - Windows: `%APPDATA%\kerykeion\`
  - macOS: `~/Library/Application Support/kerykeion/`

- **Cache & Unduhan**:
  - Linux: `~/.cache/kerykeion/downloads/`
  - Windows: `%LOCALAPPDATA%\kerykeion\downloads\`
  - macOS: `~/Library/Caches/kerykeion/downloads/`

## Testing

Jalankan seluruh pengujian unit dan integrasi end-to-end:
```bash
go test -v ./...
```
