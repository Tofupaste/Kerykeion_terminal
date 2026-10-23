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
	"ai": {
		Command:     "/ai",
		Summary:     "Asisten AI kantor & Kolaborasi P2P (Qwen, Git-Agent)",
		Usage:       "/ai <pertanyaan> | /ai model [nama] | /ai handoff <rekan> | /ai ask @<rekan> <tanya> | /ai share | /ai clear",
		Description: "Tanya AI kantor, estafet sesi obrolan + Git diff ke rekan, atau pinjam inferensi model rekan di LAN. Di dalam room, ketik @ai <tanya> untuk respon bersama.",
		Example:     "/ai handoff @budi",
		CommonError: "Ketik /ai model untuk daftar model, atau /ai ask @rekan untuk pinjam model.",
	},
	"lang": {
		Command: "/lang", Summary: "Ganti bahasa Indonesia atau English (tersimpan)",
		Usage: "/lang <id|en>", Description: "Mengganti bahasa antarmuka dan menyimpan pilihan. F2 juga mengganti bahasa.",
		Example: "/lang en", CommonError: "Pilihan bahasa: id atau en.",
	},
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
		Description: "Menampilkan identitas sendiri dan rekan LAN beserta ID lengkap, format nama#ID, dan status koneksi.",
		Example:     "/who",
		CommonError: "Jika rekan tidak muncul, cek apakah sudah di WiFi yang sama atau jalankan /doctor.",
	},
	"dm": {
		Command:     "/dm",
		Summary:     "Buka percakapan pribadi dengan rekan",
		Usage:       "/dm <nama|ID|nama#ID>",
		Description: "Membuka DM berdasarkan nama, ID lengkap, prefix ID unik, atau nama#ID (nama+ID juga bisa). Nama atau prefix yang ambigu menampilkan pilihan. Gunakan /who untuk menyalin ID.",
		Example:     "/dm aria#12ab34cd",
		CommonError: "Nama belum terdaftar di jaringan. Pastikan rekan sudah membuka aplikasi.",
	},
	"join": {
		Command:     "/join",
		Summary:     "Masuk ke room atau otomatis buat baru",
		Usage:       "/join <nama_room>",
		Description: "Mencari room di jaringan LAN. Bila room sudah ada, otomatis bergabung. Bila belum ada, otomatis membuat room baru sebagai master.",
		Example:     "/join ngobs",
		CommonError: "Nama room 2-32 karakter (huruf kecil, angka, '-' atau '_'). Awalan '#' otomatis dibersihkan.",
	},
	"room": {
		Command:     "/room",
		Summary:     "Manajemen room (new, list, info, transfer)",
		Usage:       "/room <new|list|info|transfer> [argumen]",
		Description: "Perintah grup untuk mengelola room. Gunakan /room new <nama> untuk membuat room baru, /room list untuk daftar room, /room info untuk anggota, atau /room transfer untuk memindahkan master.",
		Example:     "/room new proyek-a",
		CommonError: "Sub-perintah: new, list, info, transfer.",
	},
	"room new": {
		Command:     "/room new",
		Summary:     "Buat room baru secara eksplisit",
		Usage:       "/room new <nama_room>",
		Description: "Membuat room baru di jaringan LAN dan menetapkan kamu sebagai master room.",
		Example:     "/room new proyek-a",
		CommonError: "Nama room 2-32 karakter, huruf kecil, angka, '-' atau '_'.",
	},
	"room list": {
		Command:     "/room list",
		Summary:     "Daftar room yang kamu ikuti",
		Usage:       "/room list",
		Description: "Menampilkan daftar seluruh room yang saat ini kamu ikuti beserta peran kamu (master atau guest).",
		Example:     "/room list",
		CommonError: "Tidak ada.",
	},
	"room info": {
		Command:     "/room info",
		Summary:     "Informasi detail room dan anggota",
		Usage:       "/room info [nama_room]",
		Description: "Melihat siapa pembuat (master), tanggal pembuatan, dan seluruh anggota yang terhubung ke room.",
		Example:     "/room info ngobs",
		CommonError: "Jika nama room tidak diberikan, akan menampilkan info room aktif saat ini.",
	},
	"room transfer": {
		Command:     "/room transfer",
		Summary:     "Pindahkan hak master ke rekan lain",
		Usage:       "/room transfer <nama_room> <nama_rekan>",
		Description: "Menyerahkan peran master room kepada rekan lain di jaringan.",
		Example:     "/room transfer ngobs budi",
		CommonError: "Hanya master room yang berhak memindahkan kepemilikan room.",
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
		Description: "Langsung mengirim file ke DM atau menyiapkan pengumuman room. Path boleh mengandung spasi dan diapit kutip. ~/ dimulai dari home; path relatif dimulai dari direktori saat kerxt dijalankan. Di TUI, fokuskan chat lalu Ctrl+O untuk memilih dan mengonfirmasi file.",
		Example:     `/send "~/Downloads/catatan proyek.pdf"`,
		CommonError: "File tidak ditemukan pada path yang diberikan.",
	},
	"get": {
		Command:     "/get",
		Summary:     "Unduh file dari pengumuman room",
		Usage:       "/get <nomor|ID_file>",
		Description: "Mengunduh file berdasarkan nomor pengumuman [#no] atau ID file lengkap dari Ctrl+Y. Lampiran DM yang sudah diunduh menampilkan lokasi file; jika belum tersedia, unduhan dicoba ulang dari pengirim. File dikirim dari cache pemilik atau anggota room lain yang memilikinya. Lokasi penyimpanan ditampilkan setelah selesai. Di TUI, jalankan dari panel sistem.",
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
	sb.WriteString("=== PANDUAN LENGKAP PERINTAH KERYKEION TERMINAL ===\n\n")

	sb.WriteString("Obrolan & Room:\n")
	sb.WriteString("  /join <nama_room>               Masuk ke room atau otomatis buat baru\n")
	sb.WriteString("  /dm <nama|ID|nama#ID>           Buka pesan pribadi; /who untuk ID\n")
	sb.WriteString("  /go <nama_room_atau_rekan>      Pindah fokus ke percakapan lain\n")
	sb.WriteString("  /room new <nama_room>           Buat room baru secara eksplisit\n")
	sb.WriteString("  /room list                      Lihat daftar seluruh room yang kamu ikuti\n")
	sb.WriteString("  /room info [nama_room]          Lihat informasi master dan anggota room\n")
	sb.WriteString("  /room transfer <room> <rekan>   Pindahkan kepemilikan master ke rekan lain\n")
	sb.WriteString("  /leave [nama_room]              Keluar dari room (hanya diri sendiri)\n")
	sb.WriteString("  /disband [nama_room]            Bubarkan room permanen (khusus master)\n\n")

	sb.WriteString("Kirim Berkas & Media:\n")
	sb.WriteString("  /send <path_file>               Kirim file langsung; path boleh mengandung spasi\n")
	sb.WriteString("  Ctrl+O (TUI, fokus chat)         Pilih file, lalu Enter untuk konfirmasi kirim\n")
	sb.WriteString("  /get <nomor>                    Unduh berkas dari pengumuman room\n")
	sb.WriteString("  /open <nomor>                   Buka tautan web yang diterima di browser\n")
	sb.WriteString("  /export                         Simpan riwayat percakapan ke file teks\n\n")

	sb.WriteString("Asisten AI Kantor & Kolaborasi P2P:\n")
	sb.WriteString("  /ai <pertanyaan>                Kirim prompt ke model AI kantor aktif\n")
	sb.WriteString("  /ai model [nama]                Lihat atau ganti model (default: qwen3:14b)\n")
	sb.WriteString("  /ai handoff <nama_rekan>        Estafet sesi AI + Git Diff ke rekan di LAN\n")
	sb.WriteString("  /ai ask @<rekan> <pertanyaan>   Pinjam/delegasi inferensi ke model rekan\n")
	sb.WriteString("  /ai share                       Bagikan konteks riwayat AI ke percakapan\n")
	sb.WriteString("  /ai clear                       Reset riwayat AI pada sesi saat ini\n")
	sb.WriteString("  @ai <pertanyaan> (di room/chat) Jawaban AI otomatis dibagikan ke seluruh anggota\n\n")

	sb.WriteString("Jaringan & Status:\n")
	sb.WriteString("  /who                            Lihat daftar rekan yang sedang online di LAN\n")
	sb.WriteString("  /doctor                         Tes interface, port 42100/42101, & firewall\n")
	sb.WriteString("  /add <ip:port>                  Hubungkan langsung jika broadcast dibatasi AP\n\n")

	sb.WriteString("Pengaturan & Bantuan:\n")
	sb.WriteString("  /name <nama_baru>               Ubah nama tampilan kamu di jaringan\n")
	sb.WriteString("  /help [perintah]                Penjelasan detail perintah (contoh: /help send)\n")
	sb.WriteString("  /quit                           Keluar dari aplikasi\n\n")

	sb.WriteString("Perintah Terminal (CLI):\n")
	sb.WriteString("  kerxt                           Buka antarmuka visual (TUI)\n")
	sb.WriteString("  kerxt join <nama_room>          Langsung masuk atau buat room tertentu\n")
	sb.WriteString("  kerxt doctor                    Jalankan diagnosa jaringan dari terminal\n")
	sb.WriteString("  kerxt install                   Pasang otomatis ke ~/.local/bin dan atur PATH\n")
	sb.WriteString("  kerxt uninstall [--purge]       Hapus biner (dan data riwayat)\n")
	sb.WriteString("  kerxt --repl                    Jalankan mode baris perintah tanpa TUI\n\n")

	sb.WriteString("Tips:\n")
	sb.WriteString("• Ketik teks biasa lalu Enter untuk mengirim pesan ke percakapan aktif.\n")
	sb.WriteString("• Tekan tombol Tab untuk melengkapi perintah atau nama room secara otomatis.\n")
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
