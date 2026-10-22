package ui

import (
	"fmt"
	"strings"
)

type CommandHelp struct {
	Command     string
	Summary     string
	Usage       string
	Description string
	Example     string
	CommonError string
}

var CommandCatalog = map[string]CommandHelp{
	"help": {
		Command:     "/help",
		Summary:     "Bantuan penggunaan perintah",
		Usage:       "/help [perintah]",
		Description: "Menampilkan daftar seluruh perintah atau penjelasan mendalam untuk satu perintah tertentu.",
		Example:     "/help join",
		CommonError: "Ketik perintah tanpa tanda kurung.",
	},
	"who": {
		Command:     "/who",
		Summary:     "Lihat daftar rekan yang sedang online",
		Usage:       "/who",
		Description: "Memindai dan menampilkan rekan di jaringan kantor lokal beserta status koneksinya.",
		Example:     "/who",
		CommonError: "Jika rekan tidak muncul, cek apakah sudah di WiFi yang sama atau jalankan /doctor.",
	},
	"dm": {
		Command:     "/dm",
		Summary:     "Buka percakapan pribadi dengan rekan",
		Usage:       "/dm <nama_atau_peer_id>",
		Description: "Mengalihkan percakapan aktif ke pesan pribadi (Direct Message) dengan rekan yang dituju.",
		Example:     "/dm aria",
		CommonError: "Nama belum terdaftar di jaringan. Pastikan rekan sudah membuka aplikasi.",
	},
	"join": {
		Command:     "/join",
		Summary:     "Masuk ke room percakapan",
		Usage:       "/join <nama_room>",
		Description: "Mencari room di jaringan LAN. Bila room sudah ada, otomatis bergabung. Bila belum ada, tawarkan untuk membuatnya.",
		Example:     "/join ngobs",
		CommonError: "Nama room hanya boleh huruf kecil, angka, '-' atau '_'.",
	},
	"room": {
		Command:     "/room",
		Summary:     "Manajemen room (new, list, info, transfer)",
		Usage:       "/room <new|list|info|transfer> [argumen]",
		Description: "Perintah grup untuk membuat room (/room new <nama>), melihat daftar (/room list), info detail (/room info <nama>), atau transfer master (/room transfer <nama> <rekan>).",
		Example:     "/room new proyek-a",
		CommonError: "Sub-perintah tidak valid. Gunakan /room list atau /room new <nama>.",
	},
	"go": {
		Command:     "/go",
		Summary:     "Pindah ke percakapan lain",
		Usage:       "/go <nama_room_atau_rekan>",
		Description: "Mengganti fokus percakapan aktif ke room atau pesan pribadi yang telah diikuti.",
		Example:     "/go ngobs",
		CommonError: "Belum mengikuti room tersebut. Gunakan /join <nama> terlebih dahulu.",
	},
	"send": {
		Command:     "/send",
		Summary:     "Kirim file atau gambar ke percakapan aktif",
		Usage:       "/send <path_file>",
		Description: "Mengirim file ke DM atau mengumumkan file ke room dengan dukungan resume otomatis jika koneksi putus.",
		Example:     "/send /home/user/dokumen.pdf",
		CommonError: "File tidak ditemukan pada path yang diberikan.",
	},
	"get": {
		Command:     "/get",
		Summary:     "Unduh file dari pengumuman room",
		Usage:       "/get <nomor_pengumuman>",
		Description: "Mengunduh file yang telah diumumkan di room berdasarkan nomor pengumuman [#no].",
		Example:     "/get 1",
		CommonError: "Nomor pengumuman tidak ditemukan di room ini.",
	},
	"leave": {
		Command:     "/leave",
		Summary:     "Keluar dari room saat ini",
		Usage:       "/leave [nama_room]",
		Description: "Hanya mengeluarkan diri kamu dari room. Room tetap berjalan untuk anggota lainnya.",
		Example:     "/leave ngobs",
		CommonError: "Kamu tidak sedang berada di dalam room tersebut.",
	},
	"disband": {
		Command:     "/disband",
		Summary:     "Bubarkan room secara permanen (khusus master)",
		Usage:       "/disband [nama_room]",
		Description: "Menutup dan membubarkan room untuk semua anggota. Hanya dapat dijalankan oleh pembuat/master room.",
		Example:     "/disband ngobs",
		CommonError: "Hanya master yang memiliki izin untuk membubarkan room.",
	},
	"name": {
		Command:     "/name",
		Summary:     "Ganti nama tampilan kamu",
		Usage:       "/name <nama_baru>",
		Description: "Mengubah nama yang dilihat oleh rekan lain di jaringan tanpa mengubah identitas kriptografi kamu.",
		Example:     "/name Budi",
		CommonError: "Nama tidak boleh kosong.",
	},
	"open": {
		Command:     "/open",
		Summary:     "Buka link web yang diterima di browser",
		Usage:       "/open <nomor_link>",
		Description: "Membuka URL http/https yang diterima pada obrolan langsung ke peramban web bawaan sistem.",
		Example:     "/open 1",
		CommonError: "Nomor link tidak valid atau URL bukan skema http/https.",
	},
	"doctor": {
		Command:     "/doctor",
		Summary:     "Diagnosa koneksi LAN dan firewall",
		Usage:       "/doctor",
		Description: "Memeriksa interface jaringan, status port TCP 42100 & UDP 42101, serta saran perbaikan bila rekan tidak terlihat.",
		Example:     "/doctor",
		CommonError: "Tidak ada.",
	},
	"add": {
		Command:     "/add",
		Summary:     "Tambah alamat IP rekan secara manual",
		Usage:       "/add <ip:port>",
		Description: "Menyambungkan langsung ke IP rekan bila broadcast jaringan dibatasi oleh Access Point/router.",
		Example:     "/add 10.10.40.55:42100",
		CommonError: "Format alamat harus ip:port yang valid.",
	},
	"export": {
		Command:     "/export",
		Summary:     "Ekspor riwayat percakapan ke file",
		Usage:       "/export [nama_percakapan]",
		Description: "Menyimpan seluruh isi pesan dari percakapan saat ini ke file teks di folder downloads.",
		Example:     "/export",
		CommonError: "Percakapan aktif kosong.",
	},
	"quit": {
		Command:     "/quit",
		Summary:     "Keluar dari aplikasi",
		Usage:       "/quit",
		Description: "Menutup aplikasi Kerykeion Terminal dengan aman.",
		Example:     "/quit",
		CommonError: "Tidak ada.",
	},
}

func SuggestCommand(input string) string {
	clean := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(input)), "/")
	if clean == "" {
		return ""
	}

	bestMatch := ""
	bestScore := 999

	for cmd := range CommandCatalog {
		if strings.HasPrefix(cmd, clean) {
			return "/" + cmd
		}
		score := levenshteinDistance(clean, cmd)
		if score < bestScore && score <= 2 {
			bestScore = score
			bestMatch = "/" + cmd
		}
	}
	return bestMatch
}

func levenshteinDistance(s1, s2 string) int {
	d := make([][]int, len(s1)+1)
	for i := range d {
		d[i] = make([]int, len(s2)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}

	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			d[i][j] = min(
				d[i-1][j]+1,
				d[i][j-1]+1,
				d[i-1][j-1]+cost,
			)
		}
	}
	return d[len(s1)][len(s2)]
}

func min(vals ...int) int {
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func FormatQuickHelp() string {
	var sb strings.Builder
	sb.WriteString("=== PANDUAN PERINTAH KERYKEION TERMINAL ===\n")
	sb.WriteString("Perintah Utama:\n")
	sb.WriteString("  /who                Lihat siapa saja rekan yang sedang online\n")
	sb.WriteString("  /join <room>        Masuk ke room (contoh: /join ngobs)\n")
	sb.WriteString("  /dm <nama>          Kirim pesan pribadi (contoh: /dm aria)\n")
	sb.WriteString("  /send <file>        Kirim file/foto/video ke percakapan aktif\n")
	sb.WriteString("  /get <no>           Unduh file dari pengumuman room (contoh: /get 1)\n")
	sb.WriteString("  /open <no>          Buka link web di browser (contoh: /open 1)\n")
	sb.WriteString("  /doctor             Periksa koneksi jaringan dan firewall\n")
	sb.WriteString("  /help <perintah>    Penjelasan lengkap perintah (contoh: /help join)\n")
	sb.WriteString("  /quit               Keluar dari aplikasi\n\n")
	sb.WriteString("Tips: Ketik teks biasa lalu tekan Enter untuk mengirim pesan ke percakapan aktif.\n")
	return sb.String()
}

func FormatDetailHelp(cmdName string) string {
	clean := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(cmdName)), "/")
	help, ok := CommandCatalog[clean]
	if !ok {
		suggestion := SuggestCommand(clean)
		if suggestion != "" {
			return fmt.Sprintf("Perintah '/%s' tidak dikenal. Maksud kamu %s?", clean, suggestion)
		}
		return fmt.Sprintf("Perintah '/%s' tidak ditemukan. Ketik /help untuk melihat daftar perintah.", clean)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== BANTUAN: %s ===\n", help.Command))
	sb.WriteString(fmt.Sprintf("Deskripsi : %s\n", help.Description))
	sb.WriteString(fmt.Sprintf("Format    : %s\n", help.Usage))
	sb.WriteString(fmt.Sprintf("Contoh    : %s\n", help.Example))
	if help.CommonError != "" {
		sb.WriteString(fmt.Sprintf("Catatan   : %s\n", help.CommonError))
	}
	return sb.String()
}
