# Spesifikasi Kolaborasi AI Peer-to-Peer & Sesi Estafet (Kerykeion Terminal)

Dokumen ini mendefinisikan arsitektur, protokol transmisi data, dan alur kerja untuk fitur **AI Model Sharing** dan **Session Handoff (Estafet Riwayat Prompting)** di atas platform **Kerykeion Terminal (`kerxt`)**.

---

## 1. Latar Belakang & Visi

Dalam pengembangan perangkat lunak modern dengan bantuan AI (*AI-Assisted Pair Programming*), terdapat dua tantangan utama dalam kerja tim:
1. **Model Fragmentation & API Key Security**: Anggota tim memiliki akses model AI yang berbeda (misalnya: Rekan A memiliki Claude, Rekan B memiliki OpenAI Codex, dan jaringan kantor memiliki server lokal Qwen 3 14B). Membagikan API key secara langsung sangat berisiko terhadap kebocoran kunci dan pembengkakan biaya.
2. **Context Loss (Kehilangan Riwayat)**: Saat seorang developer menyelesaikan sebagian pekerjaan dengan AI dan ingin menyerahkannya kepada rekan kerja, rekan tersebut terpaksa membuat sesi obrolan baru dari nol (*context loss*), kehilangan seluruh konteks arsitektur, keputusan teknis, dan riwayat revisi kode.

**Solusi Kerykeion**:
Memanfaatkan jaringan peer-to-peer (P2P) terenkripsi TLS dan UDP discovery yang sudah ada di Kerykeion untuk:
* Melakukan **delegasi inferensi** (meminjam model tanpa memindahkan API key).
* Melakukan **estafet sesi** (*Session Handoff*) yang membawa riwayat percakapan beserta status kode (*Git Diff*).
* Menghadirkan **AI di Room Obrolan Tim** (*Shared AI Room*) secara real-time.

---

## 2. Arsitektur Model-Sharing P2P (Zero API Key Leakage)

### Diagram Alur Peminjaman Model
```text
[ Laptop Pemohon (Client Node) ]                     [ Laptop Penyedia (AI Worker Node) ]
              │                                                        │
              │─── 1. ANNOUNCE / WHO: Deteksi Model Tersedia ─────────▶│
              │◀── 2. "Menyediakan: Qwen3-14B / Claude 3.5" ───────────│
              │                                                        │
              │─── 3. TypeAIQueryReq (Prompt + Konteks Ringkas) ──────▶│
              │                                                        │─── 4. Panggil LLM Lokal /
              │                                                        │    API Key Rahasia (Aman)
              │◀── 5. TypeAIQueryRes (Jawaban / Patch Kode) ───────────│
              │                                                        │
    [ Terapkan Hasil ke Proyek ]
```

### Prinsip Keamanan:
* **Kunci API Tidak Pernah Ditransmisikan**: Kunci API Claude, Codex, atau token server kantor tetap berada di memori lokal penyedia.
* **Mutual Authentication**: Menggunakan kriptografi Ed25519 bawaan Kerykeion. Pemohon menandatangani request, dan pekerja memverifikasi apakah pengirim adalah rekan yang diizinkan (*whitelisted*).

---

## 3. Protokol Framing Kerykeion untuk AI

Untuk mendukung komunikasi P2P AI, ditambahkan 3 tipe frame baru pada protokol TCP Kerykeion:

| Tipe Frame | Kode Byte | Nama Payload | Fungsi |
|---|---|---|---|
| `TypeAIQueryReq` | `0x40` | `AIQueryReqPayload` | Mengirim prompt ke model rekan di LAN |
| `TypeAIQueryRes` | `0x41` | `AIQueryResPayload` | Mengembalikan jawaban model AI ke pemohon |
| `TypeAISessionSync`| `0x42` | `AISessionSyncPayload`| Mengirim paket estafet sesi (history + git diff) |

### Struktur Payload (JSON over Frame):

#### 1. `AIQueryReqPayload`
```json
{
  "v": 1,
  "req_id": "uuid-v7",
  "from_peer_id": "ed25519-hash",
  "target_model": "qwen3:14b",
  "prompt": "Tolong buatkan unit test untuk auth.go",
  "context": "konten opsional",
  "timestamp": 1792724000
}
```

#### 2. `AIQueryResPayload`
```json
{
  "v": 1,
  "req_id": "uuid-v7",
  "model": "qwen3:14b",
  "answer": "Berikut adalah unit test menggunakan testing package...",
  "error": ""
}
```

#### 3. `AISessionSyncPayload`
```json
{
  "v": 1,
  "from_peer_id": "ed25519-hash",
  "target_conv": "#projek-backend",
  "model": "qwen3:14b",
  "messages": [
    {"role": "user", "content": "..."},
    {"role": "assistant", "content": "..."}
  ],
  "git_diff": "diff --git a/main.go b/main.go ...",
  "timestamp": 1792724000
}
```

---

## 4. Dua Pendekatan Kolaborasi

### A. Pendekatan Estafet Sesi (*Session Handoff*)
Developer A bekerja dengan AI hingga tahap tertentu, lalu menyerahkan pekerjaan ke Developer B:
1. Developer A mengetik:
   ```text
   /ai handoff @budi
   ```
2. Kerykeion mengekstrak riwayat obrolan AI aktif dan mengambil snapshot perubahan git lokal (`git diff`).
3. Payload dikirim ke Budi via `TypeAISessionSync`.
4. Di terminal Budi muncul notifikasi:
   > `[AI Handoff] Menerima sesi AI dari @rafli (8 pesan) dengan Git Diff terkini.`
5. Budi dapat langsung melanjutkan obrolan tanpa membuat sesi baru:
   ```text
   /ai Lanjutkan fungsi verifikasi token jwt berdasarkan rancangan di atas
   ```

### B. Pendekatan Ruang Kolaborasi (*Shared Collaborative Room*)
AI bertindak sebagai peserta dalam room obrolan kelompok (misal `#projek`):
1. Setiap peserta di room dapat mengetik:
   ```text
   @ai tolong buatkan fungsi enkripsi password
   ```
2. Kerykeion memproses prompt menggunakan server AI (default: Qwen 3 14B kantor) dan mem-broadcast jawabannya ke room.
3. Semua rekan di room melihat jawaban yang sama dan riwayat obrolan tersinkronisasi di SQLite lokal masing-masing.

---

## 5. Konfigurasi Server AI Kantor
Secara default, Kerykeion Terminal terhubung ke server AI internal kantor:
* **Host**: `http://10.10.5.141:8080`
* **Model Unggulan**: `qwen3:14b` (Kapasitas penalaran tinggi, 14 miliar parameter)
* **Model Tambahan**: `git-agent:latest`, `qwen3-jarvis:14b`, `gemma4:12b`, `qwen3:8b`.
