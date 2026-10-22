# PRD — KantorChat: Chat Room & File Sharing P2P untuk LAN Kantor

| | |
|---|---|
| **Versi** | 0.3 (Draft) |
| **Tanggal** | 22 Oktober 2026 |
| **Status** | Siap masuk M0 (validasi singkat) lalu M1 |
| **Pemilik** | _(isi)_ |
| **Stack usulan** | Go, Bubble Tea (TUI), SQLite (modernc, pure Go), TLS 1.3 |

**Riwayat perubahan**
- v0.1: draf awal dari diskusi + koreksi atas boilerplate.
- v0.2: cakupan dipersempit ke jaringan lokal satu subnet.
- **v0.3:** konsep **Room tanpa host**, perintah `/help` untuk pengguna awam, model media "umumkan lalu tarik", dan penyesuaian temuan jaringan terbaru.

---

## 1. Ringkasan

KantorChat adalah aplikasi chat berbasis terminal yang berjalan **langsung antar-laptop di jaringan lokal kantor, tanpa internet dan tanpa server pusat atau VPS**. Setiap laptop menjalankan program yang sama, yang berperan ganda sebagai pengirim dan penerima.

Fitur utama:
- **Chat pribadi (DM)** dan **Room** bernama bebas (contoh: `ngobs`). Siapa pun cukup mengetik `/join ngobs` untuk masuk.
- **Kirim link, foto, video, dan file** dengan resume otomatis saat koneksi putus (misal saat pindah AP).
- **Riwayat lokal 30 hari**, kecil dan tanpa server.
- **Bantuan bawaan (`/help`)** yang ramah untuk orang awam.
- Satu file binary untuk Windows, macOS, dan Linux.

## 2. Latar Belakang & Temuan Lapangan

**Masalah:** warga kantor mengalami lag dan sering berpindah WiFi (WiFi Bawah 1, Bawah 2, Lt 2). WhatsApp kadang tidak bisa dibuka atau login, sehingga link, gambar, dan video kecil pun gagal terkirim.

**Temuan yang dilaporkan dari pengecekan mandiri:**

| Temuan | Arti |
|---|---|
| Semua SSID berada di subnet yang sama `10.10.40.0/24` | Broadcast lokal bisa menjangkau semua lantai; komunikasi **lintas WiFi** memungkinkan |
| IP tidak berubah saat pindah AP; putus fisik ±1–2 detik | Skenario "IP swap" jarang; yang utama adalah koneksi putus sesaat |
| Gateway lokal `10.10.40.1` ping 3–5 ms saat jalur uplink (`192.168.18.3`) tidak terjangkau (`!H`) | Masalah dominan ada di **uplink internet**, bukan di jaringan internal |

**Yang belum terbukti (dicek di M0):**

| Asumsi | Cara verifikasi |
|---|---|
| Laptop bisa saling terhubung **TCP** antar-AP dan antar-SSID (ping ke gateway **tidak** membuktikan ini) | `python -m http.server` di laptop A, unduh dari laptop B (beda AP/SSID) |
| AP tidak mengaktifkan *client isolation* | Tes di atas + ping A↔B |
| Broadcast/multicast UDP sampai ke laptop lain | `doctor` atau tes UDP sederhana |
| Windows Firewall mengizinkan inbound pada port aplikasi | Uji di laptop non-admin |

## 3. Tujuan & Non-Tujuan

### 3.1 Tujuan
1. Kirim teks/link antar-rekan kantor tanpa bergantung pada internet.
2. Kirim gambar, video, dan file (default hingga 1 GB) dengan **resume otomatis**.
3. **Room** yang bisa dibuat dan diikuti siapa saja hanya dengan mengetik nama.
4. Room tetap hidup selama minimal satu anggota online (**tanpa host**).
5. Riwayat lokal 30 hari yang kecil; anggota baru bisa mendapat riwayat terbaru.
6. Mudah dipakai orang awam lewat `/help`, pesan galat yang ramah, dan saran perintah.
7. Distribusi sederhana: satu binary, tanpa instalasi runtime.

### 3.2 Non-Tujuan (v1)
- Memperbaiki infrastruktur WiFi/uplink (lihat §14).
- Aplikasi mobile (Android/iOS).
- Panggilan suara/video, status baca, reaksi, thread.
- Relay lewat internet, sinkronisasi cloud, multi-device per identitas.
- **Kick/moderasi anggota** (ditunda ke Fase 2).
- Komunikasi lintas subnet yang tidak bisa di-route (di luar kantor ini).
- Menggantikan WhatsApp sepenuhnya.

## 4. Pengguna & Skenario

**Pilot:** tim teknis. **Lanjutan:** semua divisi (tetap lewat CLI dengan bantuan `/help`; web UI lokal dipertimbangkan di Fase 2).

| ID | User story |
|---|---|
| US-1 | Sebagai karyawan, saya ingin mengirim link ke rekan saat WhatsApp tidak bisa dibuka. |
| US-2 | Sebagai karyawan, saya ingin mengirim video 100 MB dan transfer **lanjut sendiri** saat pindah AP. |
| US-3 | Sebagai karyawan, saya ingin melihat siapa yang sedang online. |
| US-4 | Sebagai karyawan, saya ingin membuat room `ngobs` dan rekan cukup mengetik `/join ngobs` untuk masuk. |
| US-5 | Sebagai anggota, saya ingin keluar dari room kapan saja tanpa memengaruhi anggota lain. |
| US-6 | Sebagai master, saya ingin membubarkan room yang sudah tidak terpakai. |
| US-7 | Sebagai pengguna, saya ingin melihat daftar room yang saya ikuti beserta peran saya (master/guest). |
| US-8 | Sebagai anggota baru, saya ingin melihat riwayat terbaru room tanpa harus menunggu master online. |
| US-9 | Sebagai pengguna awam, saya ingin mengetik `/help` dan langsung paham cara memakai aplikasi. |
| US-10 | Sebagai IT, saya ingin alat diagnosis yang menjelaskan kenapa seseorang tidak terlihat di jaringan. |

## 5. Fase 0 (M0) — Validasi Singkat (±1 hari)

Karena temuan jaringan sudah cukup kuat (§2), M0 diringkas menjadi tes yang menentukan layak-tidaknya P2P:

1. **Tes TCP antar-laptop lintas AP/SSID** (bukan ICMP): laptop A menjalankan `python -m http.server 8000`, laptop B mengunduh file 50 MB. Ulangi dengan A dan B di SSID berbeda.
2. **Tes client isolation:** ulangi tes (1) untuk dua laptop di AP yang sama.
3. **Tes broadcast UDP:** dua laptop saling menerima paket ke `10.10.40.255`.
4. **Tes Windows Firewall:** jalankan listener di laptop non-admin, lihat apakah inbound diblokir.
5. *(Opsional)* Coba LocalSend 2–3 hari sebagai pembanding untuk menilai gap fitur (room, riwayat).

**Kriteria Go / No-Go**

| Temuan | Keputusan |
|---|---|
| Semua tes lolos | **Go penuh** ke M1 |
| Hanya lolos dalam satu SSID | **Go terbatas** — komunikasi lintas SSID ditandai belum didukung |
| *Client isolation* aktif dan tidak bisa dimatikan | **No-Go untuk pure P2P** — beralih ke desain berbasis hub di jaringan kabel |
| Firewall memblokir inbound dan tidak bisa diatur | Minta IT menerbitkan aturan firewall (TCP 42100, UDP 42101) sebelum pilot |
| LocalSend memenuhi ≥ 80% kebutuhan | Pertimbangkan hanya membangun gap yang nyata |

## 6. Ruang Lingkup per Rilis

| Rilis | Isi |
|---|---|
| **v0.1 (MVP)** | Identitas persisten, discovery per-interface, DM teks/link dengan ACK + outbox, transfer file dengan resume, SQLite, antarmuka baris perintah (REPL), `/help`, `/doctor`. |
| **v0.2** | Room: `/join`, `/room new`, `/leave`, `/room list`, `/room info`, fan-out pesan, TLS + TOFU, konfirmasi terima file (consent). |
| **v0.3** | `/disband`, `/room transfer`, kedaluwarsa room, sinkronisasi riwayat, media "umumkan lalu tarik" + thumbnail + multi-sumber, TUI Bubble Tea. |
| **v1.0** | Penyempurnaan dari pilot, notifikasi OS, paket distribusi untuk IT. |
| **Fase 2 (opsional)** | Hub mode, web UI lokal, room berpassword dengan rotasi kunci (kick), peer exchange. |

## 7. Kebutuhan Fungsional

Prioritas: **M** = Must, **S** = Should, **C** = Could.

### 7.1 Identitas & Setup
| ID | Kebutuhan | Prio |
|---|---|---|
| FR-1 | Run pertama: wizard singkat (≤ 30 detik) hanya menanyakan **nama tampilan**, membuat keypair **Ed25519**, menyimpan di direktori konfigurasi OS (`os.UserConfigDir`). | M |
| FR-2 | `peer_id` = fingerprint SHA-256 dari public key (persisten antar-run, tidak terikat IP). | M |
| FR-3 | Nama tidak unik; tampil sebagai `nama#abcd` (4 karakter fingerprint) bila ada duplikat. | M |
| FR-4 | `/name <baru>` mengganti nama tanpa mengubah `peer_id`. | S |

### 7.2 Discovery
| ID | Kebutuhan | Prio |
|---|---|---|
| FR-10 | Kirim `ANNOUNCE` UDP tiap 4 detik **per interface aktif**; alamat broadcast **dihitung dari IP + netmask** interface (bukan di-hardcode, mis. `10.10.40.255`), plus multicast. | M |
| FR-11 | Peer `online` bila `ANNOUNCE` terakhir ≤ 15 detik, selain itu `stale`. | M |
| FR-12 | Deteksi perubahan interface/IP **sendiri** (polling ≤ 2 detik) lalu langsung re-announce dan reconnect. | M |
| FR-13 | `ANNOUNCE` dari `peer_id` yang sama dari IP baru memperbarui alamat dan memicu retry outbox/transfer tertunda. | M |
| FR-14 | `/add <ip:port>` untuk menambah peer manual (fallback bila broadcast dibatasi). | M |
| FR-15 | Cache alamat terakhir peer; coba unicast saat startup. | S |
| FR-17 | `ANNOUNCE` ditandatangani Ed25519; paket tidak valid diabaikan; paket dari diri sendiri diabaikan. | M |
| FR-18 | *Office key* opsional untuk memfilter perangkat di luar kantor. | C |
| FR-19 | Peer yang pergi/offline tidak menghapus pesan tertunda; outbox mengirim saat peer muncul lagi (selama TTL). | M |

### 7.3 Pesan Pribadi (DM)
| ID | Kebutuhan | Prio |
|---|---|---|
| FR-20 | Kirim DM teks/link (maks 64 KB per pesan; lebih besar disarankan dikirim sebagai file). | M |
| FR-21 | Setiap pesan punya `msg_id` UUIDv7 dan **ditandatangani penulisnya**; penerima melakukan dedupe berdasarkan `msg_id`. | M |
| FR-22 | `MSG_ACK(msg_id)`; status `queued → sent → delivered`. | M |
| FR-23 | Tanpa ACK dalam 3 detik → outbox; retry dengan *exponential backoff* (1, 2, 4, 8, hingga 15 detik) dan otomatis saat peer terdeteksi lagi; TTL outbox default 7 hari. | M |
| FR-25 | Link dapat dibuka dari aplikasi (`/open <no>`), hanya skema `http(s)`. | S |

### 7.4 Room (Tanpa Host)

**Konsep:** room = **nama + daftar anggota + riwayat** yang disimpan di laptop masing-masing anggota. Tidak ada server. Pembuat room menjadi **master** (pemilik metadata), tetapi room **tidak bergantung** pada master; semua anggota setara dalam mengirim dan menerima.

| ID | Kebutuhan | Prio |
|---|---|---|
| FR-70 | **Identitas room = nama** (huruf kecil, `a-z 0-9 - _`, 2–32 karakter). Nama dinormalisasi (`Ngobs` = `ngobs`). | M |
| FR-71 | `/join <nama>`: kirim `ROOM_QUERY` ke jaringan (tunggu ≤ 3 detik, ulang 2×). Bila ada yang membalas `ROOM_HERE` → bergabung otomatis. Bila tidak ada → tawarkan: "Belum ada room 'ngobs'. Buat baru? (y/n)". | M |
| FR-72 | `/room new <nama>`: buat room secara eksplisit; pembuat menjadi **master**. | M |
| FR-73 | Argumen CLI: `kantorchat join ngobs` langsung membuka aplikasi dan masuk/buat room. | S |
| FR-74 | Pesan room dikirim ke **semua anggota online** (fan-out langsung); untuk anggota offline dilayani outbox pengirim. Maks 100 anggota/room, 20 room/pengguna. | M |
| FR-75 | Semua peristiwa room (`ROOM_JOIN`, `ROOM_LEAVE`, `ROOM_TRANSFER`, `ROOM_CLOSED`, pesan) **ditandatangani** pembuatnya sehingga dapat diteruskan oleh anggota mana pun. | M |
| FR-76 | `/leave [nama]`: hanya **diri sendiri** yang keluar. Mengirim `ROOM_LEAVE` bertanda tangan; room tetap hidup untuk yang lain. Menampilkan konfirmasi; riwayat room di laptop ini dihapus (opsi `--simpan` mengekspor dulu). | M |
| FR-77 | `/disband [nama]` (**hanya master**): mengirim `ROOM_CLOSED` bertanda tangan; semua anggota menutup dan menghapus room itu. Anggota offline menerimanya saat online; **anggota mana pun yang sudah menerima ikut meneruskan**. | M |
| FR-78 | `/room transfer <nama> <anggota>`: master menyerahkan peran ke anggota lain (peristiwa bertanda tangan master lama). Bila master `/leave` tanpa transfer, room menjadi **tanpa master** (tetap hidup, tidak bisa `/disband`, bubar lewat kedaluwarsa). | M |
| FR-79 | `/room list`: tampilkan room yang diikuti — nama, **peran (master/guest)**, jumlah anggota (online), aktivitas terakhir. `--semua` menampilkan room lain yang terlihat di jaringan (hanya yang publik). | M |
| FR-80 | `/room info <nama>`: master, tanggal dibuat, daftar anggota + status online. | S |
| FR-81 | **Kedaluwarsa:** room tanpa aktivitas 30 hari dihapus lokal oleh tiap laptop (tanpa koordinasi). `/join` setelahnya membuat room baru. | M |
| FR-82 | **Generasi room (`gen`)**: `room_id = (nama, created_at)`. `ROOM_CLOSED` hanya menutup `gen` tertentu, sehingga membuat ulang `ngobs` setelah dibubarkan tidak tertutup oleh data lama. *Tombstone* disimpan 30 hari agar salinan basi tidak menghidupkan kembali room yang sudah dibubarkan. | M |
| FR-83 | **Pembuatan bersamaan:** bila dua orang membuat `ngobs` hampir bersamaan, dianggap satu room; yang `created_at`-nya paling awal menjadi master (seri: `peer_id` lebih kecil); riwayat digabung. | S |
| FR-84 | **Sinkronisasi riwayat:** anggota baru menarik ≤ 7 hari/500 pesan terakhir dari anggota online mana pun; anggota yang baru online mencocokkan daftar `msg_id` (7 hari terakhir) lalu menukar pesan yang kurang. | S |
| FR-85 | Room berpassword (kunci turunan dari passphrase; pesan dienkripsi) — **Fase 2**, bersama kick lewat rotasi kunci. | C |
| FR-86 | Master dapat mengganti judul/deskripsi room (peristiwa bertanda tangan). | C |

**Batas yang diakui (tanpa server):** (a) program menaati perintah bubar/keluar, tetapi tidak ada mekanisme yang memaksa laptop lain menghapus data; (b) pesan yang hanya dimiliki anggota yang semuanya offline baru bisa diambil saat salah satu online; (c) siapa pun yang menebak nama bisa masuk room publik.

### 7.5 Transfer File & Media

**Dua model:** DM → kirim langsung ke penerima. **Room → "umumkan lalu tarik"**: pengirim hanya mengirim pengumuman kecil; anggota mengambil file saat perlu.

| ID | Kebutuhan | Prio |
|---|---|---|
| FR-30 | `/send <path>` kirim file ke percakapan aktif (DM/room); progress bar, kecepatan, estimasi waktu. | M |
| FR-31 | File dipecah **chunk 256 KB**; tiap transfer memakai **koneksi TCP terpisah** dari chat. | M |
| FR-32 | `file_id` = SHA-256 isi file (content-addressed) → resume andal, tidak bentrok antar-pengirim. | M |
| FR-33 | Penerima menyimpan `.part` dan **bitmap chunk di disk** (`.meta`) → resume tetap jalan setelah aplikasi restart. | M |
| FR-34 | Saat koneksi putus (mis. saat pindah AP), pengirim re-dial dengan backoff 1→15 detik; lewat itu status `tertunda`, lanjut otomatis saat peer terlihat lagi (selama aplikasi pengirim hidup). | M |
| FR-35 | Verifikasi **SHA-256 file utuh** di akhir; baru rename `.part` → nama final, lalu kirim `FILE_DONE`. | M |
| FR-36 | **Sanitasi nama file**: hanya basename; tolak `..`, separator path, nama reserved Windows; tangani bentrok dengan suffix. | M |
| FR-37 | **Consent penerima:** auto-terima dari kontak/room dikenal di bawah ambang; selain itu minta konfirmasi. Batas ukuran, transfer paralel, dan kuota disk dapat dikonfigurasi. | M |
| FR-38 | Peringatan sebelum membuka file berekstensi eksekusi; tidak pernah dijalankan otomatis. | S |
| FR-39 | Pengirim menganggap sukses hanya setelah menerima `FILE_DONE`. | M |
| FR-90 | **Pengumuman file di room** (`FILE_ANNOUNCE`): `file_id`, nama, ukuran, jenis, sumber, dan **thumbnail kecil (≤ 8 KB)** untuk gambar. Tiap pengumuman diberi nomor pendek, mis. `[#3] video.mp4 (84 MB) dari aria`. | M |
| FR-91 | **Unduh:** gambar ≤ 5 MB **otomatis**; video dan file lain **manual** lewat `/get <no>`. | M |
| FR-92 | **Multi-sumber:** bila pengirim offline, penerima menanyakan anggota room lain (`FILE_WHO_HAS`); siapa pun yang sudah punya file boleh melayani. Hanya **anggota room yang sama** yang dilayani. | S |
| FR-93 | **Cache berbagi lokal:** pengirim menyalin file ke cache sementara (default 7 hari, batas ukuran dikonfigurasi) agar tetap tersedia walau file aslinya dipindah/dihapus. Hasil unduhan juga menjadi sumber selama masa cache. | S |
| FR-94 | Bila sumber tidak ada yang online, tampil "tersedia saat aria online". | M |

### 7.6 Penyimpanan & Retensi
| ID | Kebutuhan | Prio |
|---|---|---|
| FR-40 | Riwayat di SQLite (`history.db`): teks dan metadata file, **bukan** isi media. | M |
| FR-41 | Retensi **30 hari**: pemangkasan saat startup dan tiap 24 jam, lalu `incremental_vacuum`. | M |
| FR-42 | Bersihkan `.part` terbengkalai > 24 jam. | M |
| FR-43 | File yang sudah **diunduh ke folder downloads tidak dihapus otomatis** (milik pengguna); tampilkan peringatan bila folder melebihi ambang (mis. 5 GB). Pembersih opsional dan default mati. | S |
| FR-44 | `/export [nama]` riwayat ke teks/JSONL. | C |

### 7.7 Antarmuka & Bantuan (`/help`)

**Perintah dasar** (tampilan ringkas bahasa Indonesia):

| Perintah | Fungsi |
|---|---|
| `/help` · `/help <perintah>` | Bantuan |
| `/who` | Siapa yang online |
| `/dm <nama>` | Buka pesan pribadi dengan seseorang |
| `/join <nama>` | Masuk room (buat baru bila belum ada) |
| `/room new <nama>` | Buat room |
| `/room list` · `/room info <nama>` | Lihat room yang diikuti / detail room |
| `/go <room atau nama>` | Pindah percakapan aktif |
| `/send <file>` | Kirim file/foto/video ke percakapan aktif |
| `/get <no>` | Unduh file dari pengumuman bernomor |
| `/leave [nama]` | Keluar dari room (hanya kamu) |
| `/disband [nama]` | Bubarkan room (khusus master) |
| `/room transfer <nama> <anggota>` | Serahkan peran master |
| `/name <baru>` | Ganti nama |
| `/open <no>` | Buka link yang diterima |
| `/doctor` | Cek koneksi & firewall |
| `/export` · `/quit` | Ekspor riwayat · keluar |

| ID | Kebutuhan | Prio |
|---|---|---|
| FR-50 | Teks tanpa awalan `/` dikirim ke **percakapan aktif**; bila belum ada, tampilkan petunjuk ("Belum ada room. Ketik /join ngobs atau /who"). | M |
| FR-51 | `/help` menampilkan ±8 perintah paling penting dengan **contoh** satu baris; `/help <perintah>` menampilkan penjelasan, contoh, dan kesalahan umum. | M |
| FR-52 | **Wizard run pertama** ditutup dengan panduan cepat 3 baris (cara lihat siapa online, bergabung ke room, kirim file). | M |
| FR-53 | **Pesan galat yang ramah** dan bisa ditindaklanjuti, mis.: "Belum ada yang terlihat online. Cek: 1) satu WiFi kantor? 2) ketik /doctor." Tidak menampilkan trace/teknis mentah kecuali `--debug`. | M |
| FR-54 | **Saran perintah** untuk salah ketik ("Maksud kamu /join?") dan **Tab-completion** untuk perintah, nama room, dan nama peer. | S |
| FR-55 | Baris status menampilkan percakapan aktif + petunjuk singkat (`? bantuan`). | S |
| FR-56 | Indikator status peer (online/stale), status pesan (✓ terkirim, ✓✓ delivered, ⏳ antre), peran di room (👑 master). | M |
| FR-57 | Notifikasi pesan masuk (bel terminal; notifikasi OS opsional). | S |
| FR-58 | v0.1–v0.2 memakai REPL berbasis baris dengan pencetakan aman terhadap input; **TUI Bubble Tea** (daftar peer/room di kiri, chat di tengah, input di bawah) di v0.3. | M |
| FR-59 | Seluruh teks antarmuka berbahasa Indonesia (opsi bahasa Inggris Could). | S |

### 7.8 Diagnosis
| ID | Kebutuhan | Prio |
|---|---|---|
| FR-60 | `/doctor` (juga `kantorchat doctor`): tampilkan interface/IP/subnet, hasil tes broadcast, status bind port, profil jaringan Windows (Public/Private), petunjuk firewall, dan peer terjangkau vs tidak, dengan saran perbaikan dalam bahasa awam. | M |
| FR-61 | Log ke file dengan rotasi; flag `--debug`. | S |

## 8. Kebutuhan Non-Fungsional

| Area | Target |
|---|---|
| **Platform** | Windows 10/11 (prioritas), macOS, Linux; amd64 dan arm64 |
| **Ukuran binary** | < 20 MB |
| **Resource** | RAM idle < 50 MB, CPU idle < 1% |
| **Discovery** | Peer baru terlihat ≤ 10 detik setelah start |
| **Re-binding IP** | ≤ 10 detik setelah peer pindah AP (jarang terjadi di jaringan ini) |
| **Pengiriman teks** | Delivered ≤ 3 detik bila peer online dan terjangkau |
| **Skala** | ≤ 100 anggota/room, ≤ 20 room/pengguna, ≤ ~100 peer online |
| **Port tetap** | TCP `42100`, UDP `42101` (dapat dikonfigurasi) agar IT bisa membuat aturan firewall |
| **Ketahanan** | Proses dimatikan saat transfer → resume tanpa mengirim ulang chunk yang valid |
| **Dependensi runtime** | Tidak ada |

## 9. Arsitektur & Protokol

### 9.1 Prinsip
- **Murni client ke client:** tiap program membuka port sendiri; tidak perlu server atau VPS.
- **Dua bidang:** *Discovery plane* (UDP) dan *Data plane* (TCP+TLS).
- **Identitas tidak terikat IP:** `peer_id` = hash public key.
- **Semua peristiwa bertanda tangan** → dapat dititipkan/diteruskan oleh anggota mana pun tanpa dipalsukan.
- Ketahanan di level aplikasi (outbox, bitmap chunk, retry), bukan fitur transport.

### 9.2 Transport
- **TCP + TLS 1.3** (self-signed Ed25519 per perangkat, pinning fingerprint ↔ `peer_id`, TOFU untuk kontak baru). QUIC ditunda: resume per-chunk sudah cukup, QUIC menambah kompleksitas sertifikat.
- Koneksi chat dan koneksi transfer file **terpisah** agar chat tidak tertahan video.

### 9.3 Framing
```
[1 byte Type][4 byte Length, uint32 big-endian][Payload]
```
Payload kontrol JSON (`v: 1`), payload chunk biner. Batas frame 1 MB.

| Type | Nama | Keterangan |
|---|---|---|
| `0x01` | HELLO | Versi protokol, kapabilitas |
| `0x02` | MSG | Pesan (DM/room), bertanda tangan penulis |
| `0x03` | MSG_ACK | `msg_id` |
| `0x10` | FILE_OFFER | `file_id`, nama, ukuran, `chunk_size`, `total_chunks`, `sha256` |
| `0x11` | FILE_ACCEPT | Bitmap chunk yang dibutuhkan |
| `0x12` | CHUNK | `[4B index][data]` |
| `0x13` | CHUNK_ACK | ACK berjendela tiap N chunk |
| `0x14` | FILE_DONE | Hasil verifikasi SHA-256 |
| `0x15` | FILE_REJECT / CANCEL | |
| `0x20` | ROOM_META | Nama, `gen`, master, judul |
| `0x21` | ROOM_JOIN | Anggota bergabung (bertanda tangan) |
| `0x22` | ROOM_LEAVE | Anggota keluar |
| `0x23` | ROOM_CLOSED | Dibubarkan master (+ tombstone) |
| `0x24` | ROOM_TRANSFER | Pengalihan master |
| `0x25` | ROOM_SYNC_REQ / `0x26` ROOM_SYNC_RES | Cocokkan daftar `msg_id`, tukar pesan yang kurang |
| `0x30` | FILE_ANNOUNCE | Pengumuman file di room (+ thumbnail) |
| `0x31` | FILE_WHO_HAS / `0x32` FILE_HAVE | Cari sumber alternatif |
| `0x7F` | PING | Keepalive |

**UDP (discovery):** `ANNOUNCE` (kehadiran peer), `ROOM_QUERY` ("siapa punya `ngobs`?"), `ROOM_HERE` (jawaban unicast).

### 9.4 `ANNOUNCE` (UDP, JSON ≤ 512 B)
```json
{
  "v": 1,
  "type": "ANNOUNCE",
  "pubkey": "<base64 ed25519 32B>",
  "name": "aria",
  "port": 42100,
  "seq": 1042,
  "ts": 1792643700,
  "sig": "<base64 signature>"
}
```
Penerima: verifikasi `sig`, hitung `peer_id = sha256(pubkey)`, tolak bila `ts` basi atau `seq` mundur, abaikan paket sendiri, perbarui tabel `peer_id → (ip, port, last_seen)`.

**Lapisan discovery:** (1) broadcast/multicast per interface, (2) unicast ke alamat cache, (3) `/add <ip:port>` manual.

### 9.5 Alur Join Room
```
Budi: /join ngobs
  |-- ROOM_QUERY("ngobs") (UDP broadcast) ---------> semua peer
  |<-- ROOM_HERE (unicast dari anggota yang online)--|
  |== TCP+TLS ke satu/lebih anggota ================>|
  |-- ROOM_JOIN (bertanda tangan) ------------------>|  disebar ke anggota lain
  |<-- ROOM_META + daftar anggota ------------------|
  |<-- ROOM_SYNC_RES (riwayat ≤ 7 hari/500 pesan) --|
  Budi sudah di room; pesan berikutnya di-fan-out ke semua anggota online.
```
Tidak ada yang menjawab dalam ±9 detik (3 detik × 3 percobaan) → tawarkan membuat room baru.

### 9.6 Alur Transfer File (DM)
```
[Pengirim]                                       [Penerima]
  hitung SHA-256 → file_id
  |-- FILE_OFFER ----------------------------->|  cek consent + .meta
  |<-- FILE_ACCEPT (chunk yang dibutuhkan) ----|
  |== CHUNK ... (jendela N chunk) ============>|  tulis .part, update bitmap
  |   ... putus sesaat saat pindah AP ...      |
  |-- re-dial, FILE_OFFER sama --------------->|
  |<-- FILE_ACCEPT (sisa chunk saja) ----------|
  |== CHUNK sisa ============================>|
  |<-- FILE_DONE (sha256 OK) -------------------|  rename .part → final
```
CRC32 per-chunk opsional (TLS sudah menjaga integritas); SHA-256 file utuh wajib.

### 9.7 Alur Media di Room (umumkan lalu tarik)
```
Aria: /send video.mp4 (di room ngobs)
  |-- FILE_ANNOUNCE [#3] (nama, ukuran, hash, thumbnail) --> semua anggota
Budi: /get 3 → tarik dari Aria (alur 9.6). Aria offline → FILE_WHO_HAS ke anggota lain.
Selesai diunduh → Budi menjadi sumber tambahan selama masa cache.
```
Alasan: video 100 MB ke 20 anggota = 2 GB unggahan dari satu laptop. Dengan model ini beban terbagi.

### 9.8 Hub Opsional (Fase 2)
Satu PC kabel yang selalu menyala menjalankan mode `hub`: store-and-forward, arsip riwayat room, titik temu discovery. Peer tetap P2P; hub hanya menambah ketersediaan. **Dipicu bila** client isolation tidak bisa dimatikan, atau pesan untuk anggota offline menjadi keluhan utama pilot.

## 10. Model Data (SQLite)

| Tabel | Kolom utama |
|---|---|
| `identity` | `peer_id`, `display_name`, `created_at` (private key sebagai file terpisah izin `0600`) |
| `peers` | `peer_id`, `display_name`, `pubkey`, `last_ip`, `last_port`, `last_seen`, `trusted` |
| `rooms` | `name`, `gen`, `master_id`, `my_role` (master/guest/none), `created_at`, `last_activity`, `state` |
| `room_members` | `room`, `peer_id`, `joined_at`, `left_at` |
| `room_tombstones` | `name`, `gen`, `closed_at` (disimpan 30 hari) |
| `messages` | `msg_id` (UUIDv7, PK), `conversation_id`, `author_id`, `body`, `ts`, `sig`, `status`, `file_ref` |
| `outbox` | `msg_id`, `target_peer_id`, `attempts`, `next_retry_at`, `expires_at` |
| `transfers` | `file_id`, `peer_id`, `direction`, `filename`, `size`, `state`, `local_path`, `updated_at` |
| `media_cache` | `file_id`, `path`, `size`, `expires_at` |

`journal_mode=WAL`, `auto_vacuum=INCREMENTAL`. Kompresi teks tidak masuk v1 (±1–2 MB per 10.000 pesan).

**Struktur direktori** (via `os.UserConfigDir`/`os.UserCacheDir`; Windows di `%APPDATA%`):
```
<config>/kantorchat/  identity.key (0600) · config.json · history.db
<data>/kantorchat/    downloads/ · downloads/.partial/ · cache/
```

## 11. Keamanan & Privasi

| Ancaman | Mitigasi |
|---|---|
| Penyamaran peer lain | `peer_id` = hash public key; `ANNOUNCE`, pesan, dan peristiwa room bertanda tangan; TLS dengan pinning |
| Penyadapan di LAN | TLS 1.3 antar-peer |
| Pemalsuan perintah master (`disband`/`transfer`) | Hanya valid bila ditandatangani `master_id` yang tercatat |
| Path traversal lewat nama file | FR-36 |
| Disk penuh / spam file | FR-37 (consent, batas ukuran, kuota, transfer paralel) |
| File berbahaya | FR-38; tidak ada eksekusi otomatis |
| Orang luar/bukan anggota mengambil file | FILE_WHO_HAS hanya dilayani untuk anggota room yang sama |
| Tebak nama room | Room publik terbuka untuk siapa pun di LAN (diakui); room berpassword di Fase 2 |
| Flood UDP/TCP | Batas laju per sumber; batas koneksi; batas frame 1 MB |
| Pencurian private key | Izin file `0600`; tidak pernah dikirim |

**Privasi:** data hanya tersimpan lokal; tanpa telemetri. Kebijakan retensi riwayat perlu disetujui kantor (§17).

## 12. Distribusi & Deployment

- Build lintas platform (`GOOS/GOARCH`); distribusi awal via flashdisk/shared folder.
- **Windows Firewall:** prompt inbound muncul saat run pertama; profil *Public* memblokir inbound dan pengguna non-admin tidak bisa membuat aturan. Sediakan skrip/aturan IT (TCP 42100, UDP 42101, profil Private/Domain); `/doctor` menampilkan petunjuknya.
- **macOS:** izin *Local Network* dan peringatan Gatekeeper.
- Update manual (ganti binary); versi protokol `v` untuk kompatibilitas mundur.

## 13. Metrik Keberhasilan (Pilot 2–4 minggu)

| Metrik | Target |
|---|---|
| Teks delivered ≤ 5 detik saat peer online & terjangkau | ≥ 99% |
| Transfer file 10 MB selesai tanpa intervensi | ≥ 95% |
| Transfer terputus lalu **resume otomatis** sampai selesai | ≥ 90% |
| Waktu discovery peer baru (median) | ≤ 10 detik |
| `/join` ke room yang ada sampai tampil (median) | ≤ 5 detik |
| Anggota baru menerima riwayat ≤ 500 pesan | ≤ 5 detik |
| **Pengguna awam mengirim pesan pertama tanpa bantuan** (uji 5 orang non-teknis) | ≤ 2 menit |
| Pengguna aktif mingguan dari tim pilot | ≥ 70% |
| Berkurangnya insiden "tidak bisa kirim file/link" (survei) | ≥ 50% |

## 14. Pekerjaan Paralel di Luar Aplikasi

Aplikasi adalah mitigasi untuk **uplink yang bermasalah**. Disarankan IT mengecek paralel: stabilitas uplink `192.168.18.3` (jalur yang tampak mati di pengecekan), bandwidth dan QoS pada jam sibuk, serta perencanaan channel dan kapasitas AP.

## 15. Risiko & Mitigasi

| # | Risiko | Mitigasi |
|---|---|---|
| R1 | AP *client isolation* aktif | Dicek di M0; minta IT mematikan; atau hub kabel |
| R2 | Broadcast UDP dibatasi AP | Unicast cache, `/add` manual, `/doctor` |
| R3 | Firewall Windows memblokir inbound | Port tetap, skrip IT, `/doctor` |
| R4 | Orang awam bingung dengan CLI | `/help`, wizard, galat ramah, saran perintah, uji 5 non-teknis; web UI di Fase 2 |
| R5 | Pesan/file untuk peer offline tidak sampai saat semua pemilik offline | Outbox, sinkronisasi riwayat, multi-sumber, hub (Fase 2) |
| R6 | Room tidak bisa dipaksa dihapus di laptop lain | Diakui sebagai batas desain tanpa server |
| R7 | Pembuatan room kembar saat jaringan bermasalah | Aturan penggabungan FR-83 |
| R8 | Solusi sudah ada (LocalSend/BeeBEEP) | Baseline di M0; bangun hanya gap yang nyata |
| R9 | Disk terisi cache/unduhan | Kuota, peringatan, cache dengan TTL |
| R10 | Kompleksitas protokol room | Rilis bertahap (v0.2 dasar → v0.3 sinkronisasi), uji dengan banyak simulasi laptop |

## 16. Rencana & Milestone (estimasi kasar, belum divalidasi)

| Milestone | Isi | Estimasi |
|---|---|---|
| **M0** | Validasi TCP lintas AP/SSID, isolation, broadcast, firewall | ±1 hari |
| **M1** | Identitas persisten, discovery per-interface, DM + ACK/outbox, `/help`, `/doctor`, REPL | ±1–2 minggu |
| **M2** | Transfer file: framing, bitmap persisten, resume, SHA-256, sanitasi, consent | ±1–2 minggu |
| **M3** | SQLite + retensi, TLS + TOFU, Room dasar (`/join`, `/room new`, `/leave`, `/room list`, fan-out) | ±2 minggu |
| **M4** | `/disband`, `/room transfer`, kedaluwarsa, tombstone, sinkronisasi riwayat | ±1–2 minggu |
| **M5** | Media: pengumuman, thumbnail, `/get`, multi-sumber, cache; TUI Bubble Tea | ±2 minggu |
| **M6** | Pilot 2–4 minggu, evaluasi, keputusan Fase 2 | 2–4 minggu |

## 17. Pertanyaan Terbuka

1. Berapa jumlah pengguna dan mayoritas OS (Windows?)
2. Apakah IT bisa menyebarkan aturan firewall, atau pengguna punya hak admin?
3. Apakah ada kebijakan kantor soal penyimpanan riwayat chat dan file?
4. Kapan UI untuk non-teknis (web UI lokal) dibutuhkan, setelah hasil pilot?
5. Apakah ada PC kabel yang selalu menyala untuk calon hub?
6. Apakah room perlu opsi privat (tidak muncul di `/room list --semua`) sejak awal?
7. Batas ukuran file maksimum (default 1 GB cukup?) dan ambang unduh otomatis (default gambar ≤ 5 MB)?

## 18. Catatan Keputusan

| Topik | Diskusi awal | PRD ini | Alasan |
|---|---|---|---|
| Arsitektur | Pure P2P / "tanpa server" | **Pure P2P tanpa VPS**; hub opsional di Fase 2 | Satu subnet kantor; tiap laptop berperan klien + penerima |
| Validasi | Langsung membangun | **M0 singkat** (TCP lintas AP, isolation, firewall) | Ping gateway tidak membuktikan antar-laptop bisa terhubung |
| Room | Channel `#umum` | **Room bebas, tanpa host**, master = pemilik metadata | Tahan laptop putus/pindah WiFi |
| Keluar vs bubar | — | **`/leave` = diri sendiri; `/disband` = semua (master)** | Pemisahan yang jelas |
| Media di room | Kirim ke semua | **Umumkan lalu tarik** + multi-sumber | Hemat unggahan pengirim |
| Hardcode `10.10.40.255` | Usul awal | **Hitung dari interface** + `/add` manual | Tidak rapuh di VPN/Docker/WSL atau jaringan lain |
| Retry 5× jeda 1,5 detik | Usul awal | **Backoff hingga peer terlihat** + state di disk | Pindah AP bisa lebih lama dari 7,5 detik |
| Penyimpanan | JSONL + gzip | **SQLite**, tanpa gzip | Pemangkasan atomik; teks kecil tidak perlu gzip |
| Transport | QUIC | **TCP+TLS** | Resume level aplikasi cukup |
| Identitas | UUID acak per-run | **Hash public key persisten** | Anti-spoofing, identitas stabil |
| Keamanan | Tidak ada | **Tanda tangan, TLS, sanitasi, consent** | LAN kantor bukan lingkungan tepercaya penuh |
| Kick | Diminta | **Ditunda ke Fase 2** | Butuh room berpassword + rotasi kunci |
| Bantuan | — | **`/help`, wizard, galat ramah, saran perintah** | Target pengguna awam |
| Pembuatan room | `/room <nama>` | **`/join` pintar + `/room new` eksplisit** | Orang awam cukup satu perintah |

## Lampiran A — Temuan pada Boilerplate Go Sebelumnya (jangan dipakai apa adanya)

1. `generatePeerID()` acak tiap run — tidak persisten.
2. Status chunk hanya di memori; setelah penerima restart, semua chunk dikirim ulang (resume tidak bekerja).
3. Tidak ada `FILE_DONE`/verifikasi SHA-256; pengirim menganggap sukses setelah menulis ke socket.
4. `filepath.Join("./downloads", offer.Filename+".part")` memakai nama dari peer → **path traversal**.
5. Penerimaan file otomatis tanpa consent/batas ukuran/kuota.
6. `transfer_id` dari `path+size` pengirim → bisa bentrok.
7. UDP bind port tetap 9999 → instance kedua di laptop yang sama gagal bind.
8. Broadcast `255.255.255.255` hanya via interface default; tidak andal pada multi-NIC.
9. `listenLoop` memakai `continue` saat error baca → berpotensi *busy loop*.
10. Tidak ada autentikasi/enkripsi; `HELLO.target_id` tidak menjamin identitas.
11. `FindByUsername` mengembalikan kecocokan pertama pada nama ganda.
12. `fmt.Printf` bercampur dengan prompt.
13. `SendText` tanpa outbox/retry/dedupe; ACK tidak mengikat `msg_id`.
