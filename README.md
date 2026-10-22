# Kerykeion Terminal (`kerxt`)

Kerykeion Terminal adalah aplikasi obrolan (*chat room*) dan transfer berkas P2P (*peer-to-peer*) berbasis terminal untuk jaringan lokal kantor (LAN), **tanpa internet dan tanpa server pusat atau VPS**.

Cukup ketik `kerxt` di terminal, aplikasi langsung aktif dan siap digunakan.

---

## Fitur Utama

- **Aktivasi Cepat**: Ketik `kerxt` di terminal untuk langsung masuk ke antarmuka visual (TUI).
- **Pesan Pribadi (DM) & Room Tanpa Host**: Masuk room cukup mengetik `/join <nama_room>` (misal `/join ngobs`). Room tetap hidup selama minimal 1 anggota terhubung.
- **Kirim Berkas & Media dengan Resume Otomatis**: Berkas dipecah menjadi *chunk* 256 KB dengan verifikasi integritas penuh SHA-256 dan pelacakan bitmap di disk. Jika koneksi WiFi terputus saat berpindah Access Point (AP), pengiriman akan otomatis melanjutkan (*resume*) tanpa mengulang dari awal.
- **Model Media di Room (Umumkan lalu Tarik)**: Gambar diumumkan beserta thumbnail ringan (≤ 8 KB). Berkas diunduh dengan perintah `/get <nomor>`, mendukung multi-sumber dari anggota room lain saat pengirim asli sedang luring (*offline*).
- **Keamanan & Kriptografi**:
  - Identitas persisten berbasis pasangan kunci **Ed25519** tersimpan di direktori pengguna (`~/.config/kerykeion/identity.key`) dengan hak akses ketat `0600`.
  - `peer_id` merupakan sidik jari SHA-256 dari kunci publik.
  - Setiap pesan dan peristiwa room ditandatangani secara digital (*cryptographically signed*).
  - Enkripsi transport **TLS 1.3** dengan sertifikat *self-signed* dan penyematan sidik jari (*fingerprint pinning* / TOFU).
- **Penyimpanan Lokal & Retensi 30 Hari**: Basis data lokal SQLite (*pure Go* tanpa CGO melalui `modernc.org/sqlite`) beroperasi dalam mode WAL dengan pemangkasan otomatis riwayat lebih dari 30 hari.
- **Diagnosa Mandiri (`kerxt doctor`)**: Alat pengecekan interface jaringan, status bind port TCP/UDP, broadcast, serta petunjuk konfigurasi firewall Linux, macOS, dan Windows dalam bahasa yang mudah dipahami.
- **Antarmuka Ganda (TUI & REPL)**: TUI penuh (Bubble Tea) dengan sidebar interaktif, serta mode REPL baris perintah untuk terminal minimalis atau otomatisasi script.
- **Biner Mandiri**: Berkas biner tunggal (*single static binary* ~12 MB, di bawah batas PRD 20 MB) tanpa ketergantungan runtime eksternal.

---

## Panduan Instalasi

### 1. Instalasi Otomatis (Direkomendasikan)

Kerykeion Terminal dilengkapi dengan **installer otomatis** yang melakukan deteksi sistem, kompilasi produksi, penyalinan biner ke `~/.local/bin`, dan konfigurasi PATH secara otomatis:

```bash
# Clone repositori
git clone https://github.com/your-org/kerykeion_terminal.git
cd kerykeion_terminal

# Jalankan installer satu langkah
./install.sh
```

Atau jika kamu sudah memiliki biner `kerxt`:
```bash
kerxt install
```

Installer otomatis ini akan:
1. Mendeteksi distro dan arsitektur CPU (CachyOS, Fedora, Ubuntu, macOS, dll.).
2. Memastikan Go compiler tersedia (memberikan panduan perintah instalasi Go jika belum ada).
3. Mengompilasi biner produksi teroptimasi (`-ldflags="-s -w"`).
4. Menyalin biner ke `~/.local/bin/kerxt` dan menyetel izin eksekusi (`chmod +x`).
5. Memeriksa status `$PATH` dan otomatis menambahkan konfigurasi ke file shell kamu (`~/.bashrc`, `~/.zshrc`, atau `~/.config/fish/config.fish`) bila belum ada.

---

### 2. Panduan Manual per Distro

#### CachyOS / Arch Linux
```bash
sudo pacman -S go git
cd /path/to/kerykeion_terminal
./install.sh
```

#### Fedora / RHEL
```bash
sudo dnf install -y golang git
cd /path/to/kerykeion_terminal
./install.sh
```

---

### 3. Cara Menghapus Instalasi (Uninstall)

Untuk menghapus biner:
```bash
kerxt uninstall
```

Untuk menghapus biner sekaligus membersihkan seluruh data riwayat dan cache:
```bash
kerxt uninstall --purge
```

---

### 3. Kompilasi Silang untuk Windows dan macOS

```bash
# Untuk Windows (64-bit)
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o kerxt.exe ./cmd/kerxt

# Untuk macOS (Apple Silicon arm64)
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o kerxt-darwin-arm64 ./cmd/kerxt
```

---

## Cara Menjalankan

### 1. Membuka Aplikasi (TUI Visual)
Cukup ketik perintah berikut:
```bash
kerxt
```

### 2. Diagnosa Jaringan dan Firewall
Gunakan perintah ini untuk memeriksa apakah interface jaringan, port 42100/42101, dan broadcast LAN berfungsi normal:
```bash
kerxt doctor
```

### 3. Langsung Masuk ke Room
Kamu bisa langsung membuka aplikasi dan bergabung ke room tertentu:
```bash
kerxt join ngobs
```

### 4. Mode REPL (Baris Perintah Sederhana)
Bila ingin antarmuka baris demi baris tanpa rendering TUI penuh:
```bash
kerxt --repl
```

---

## Perintah di Dalam Aplikasi

| Perintah | Fungsi | Contoh |
|---|---|---|
| `/help` | Menampilkan bantuan perintah | `/help` atau `/help join` |
| `/who` | Menampilkan rekan yang online di LAN | `/who` |
| `/dm <rekan>` | Membuka percakapan pribadi dengan rekan | `/dm aria` |
| `/join <room>` | Masuk ke room (buat baru bila belum ada) | `/join ngobs` |
| `/room new <room>` | Membuat room baru secara eksplisit | `/room new proyek-a` |
| `/room list` | Melihat daftar room yang diikuti | `/room list` |
| `/room info <room>` | Melihat informasi detail dan anggota room | `/room info ngobs` |
| `/room transfer <room> <rekan>` | Mengalihkan peran master room ke rekan lain | `/room transfer ngobs budi` |
| `/go <target>` | Berpindah fokus percakapan aktif | `/go ngobs` |
| `/send <path_file>` | Mengirim berkas atau gambar ke percakapan aktif | `/send /home/user/dokumen.pdf` |
| `/get <no>` | Mengunduh berkas dari pengumuman room | `/get 1` |
| `/leave [room]` | Keluar dari room saat ini | `/leave ngobs` |
| `/disband [room]` | Membubarkan room permanen (khusus master) | `/disband ngobs` |
| `/name <baru>` | Mengubah nama tampilan kamu di jaringan | `/name Budi` |
| `/open <no>` | Membuka tautan web di browser default | `/open 1` |
| `/doctor` | Memeriksa status port, jaringan, dan firewall | `/doctor` |
| `/add <ip:port>` | Menghubungkan langsung jika broadcast dibatasi AP | `/add 10.10.40.50:42100` |
| `/quit` | Keluar dari Kerykeion Terminal | `/quit` |

---

## Konfigurasi & Lokasi Data

Kerykeion Terminal menyimpan data secara lokal sesuai standar OS:

- **Konfigurasi & Database**:
  - Linux: `~/.config/kerykeion/` (`identity.key`, `config.json`, `history.db`)
  - Windows: `%APPDATA%\kerykeion\`
  - macOS: `~/Library/Application Support/kerykeion/`
- **Unduhan & Berkas Parsial**:
  - Linux: `~/.cache/kerykeion/downloads/` dan `~/.cache/kerykeion/downloads/.partial/`
  - Windows: `%LOCALAPPDATA%\kerykeion\downloads\`
  - macOS: `~/Library/Caches/kerykeion/downloads/`

---

## Menjalankan Pengujian Otomatis

Seluruh pengujian unit dan pengujian integrasi *End-to-End* (E2E) dapat dijalankan dengan:

```bash
go test -v ./...
```

Pengujian mencakup:
1. Kriptografi Ed25519 dan verifikasi tanda tangan digital.
2. Protokol framing biner dan batas ukuran payload.
3. Operasi database SQLite, indexing, dan retensi 30 hari.
4. Normalisasi room dan transisi state master.
5. Sanitasi nama berkas, deteksi ekstensi berbahaya, dan bitmap resume berkas dari disk.
6. Komunikasi TLS dua arah antara dua peer simultan (Alice & Bob).
