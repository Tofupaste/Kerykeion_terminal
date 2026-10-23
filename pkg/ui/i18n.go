package ui

import "fmt"

const defaultStatusText = "Ketik /help untuk panduan"

var englishText = map[string]string{
	"Ketik pesan; Ctrl+O file; F6 perintah...":                                 "Type a message; Ctrl+O attach; F6 commands...",
	"Ctrl+O Pilih file saat fokus chat (TUI)":                                  "Ctrl+O Choose a file while chat has focus (TUI)",
	"Picker: Enter pilih lalu konfirmasi kirim; Backspace kembali; Esc batal.": "Picker: Enter selects then confirms sending; Backspace goes back; Esc cancels.",
	"Fokuskan chat untuk melampirkan file.":                                    "Focus a conversation to attach a file.",
	"Lampirkan file":                                                           "Attach a file",
	"Tujuan: %s":                                                               "Recipient: %s",
	"Enter Kirim · Backspace Kembali · Esc Batal":                              "Enter Send · Backspace Back · Esc Cancel",
	"↑/↓ Pilih · Enter Buka · Backspace Induk · Esc Batal":                     "↑/↓ Select · Enter Open · Backspace Parent · Esc Cancel",
	"Membaca folder...":                                                        "Reading directory...",
	"Folder kosong.":                                                           "Empty directory.",

	"F2 Bahasa · F6 Fokus · PgUp/PgDn Gulir · Ctrl+C Keluar":                                               "F2 Language · F6 Focus · PgUp/PgDn Scroll · Ctrl+C Quit",
	"Ketik pesan chat; F6 untuk perintah sistem...":                                                        "Type a chat message; F6 for system commands...",
	"Panel sistem menerima perintah /... saja. Tekan F6 untuk fokus chat, atau F4 jika chat diminimalkan.": "The system panel accepts /commands only. Press F6 to focus chat, or F4 if chat is minimized.",
	"Ketik pesan atau /help untuk bantuan...":                                                              "Type a message or /help for commands...",
	"Ketik perintah atau /help untuk bantuan...":                                                           "Type a command or /help for commands...",
	defaultStatusText: "Type /help for commands",
	"Pesan sistem":    "System messages",
	"Percakapan":      "Conversation",
	"Room":            "Rooms",
	"Rekan":           "Peers",
	"Belum ada room. /join <nama> untuk mulai.":               "No rooms yet. /join <name> to start.",
	"Belum ada rekan terdeteksi. /doctor untuk diagnosis.":    "No peers discovered. /doctor to diagnose.",
	"Belum ada pesan. Ketik pesan lalu Enter untuk mengirim.": "No messages yet. Type a message and press Enter to send.",
	"Kerykeion Terminal siap.\n/join <nama> untuk membuka room, /dm <rekan> untuk pesan pribadi.\n/help untuk panduan. F2 untuk mengganti bahasa.": "Kerykeion Terminal is ready.\n/join <name> to open a room, /dm <peer> for a private conversation.\n/help for commands. F2 to change language.",
	"F2 Bahasa · F4 Chat · F6 Fokus · PgUp/PgDn Gulir · Ctrl+C Keluar":                                                                             "F2 Language · F4 Chat · F6 Focus · PgUp/PgDn Scroll · Ctrl+C Quit",
	"Esc Minimalkan chat":                  "Esc Minimize chat",
	"Chat diminimalkan: %s":                "Chat minimized: %s",
	"Bahasa":                               "Language",
	"Perbesar terminal (minimal 32 × 12).": "Enlarge the terminal (at least 32 × 12).",
	"Bahasa diubah ke Indonesia.":          "Language changed to English.",
	"Gunakan: /lang id atau /lang en":      "Usage: /lang id or /lang en",
	"Tidak ada chat aktif. Gunakan /join <nama> atau /dm <rekan>.":      "No active chat. Use /join <name> or /dm <peer>.",
	"Chat diminimalkan. Tekan F4 untuk membuka sebelum mengirim pesan.": "Chat is minimized. Press F4 to open it before sending a message.",
	"[Galat] %v":                    "[Error] %v",
	"[Galat Transfer] %s: %s":       "[Transfer error] %s: %s",
	"Terkirim":                      "Sent",
	"Selesai diunduh ke downloads/": "Downloaded to downloads/",
	"[#%d] %s (%s) dari %s":         "[#%d] %s (%s) from %s",
	"[Pengumuman File] %s. Ketik /get %d untuk unduh.": "[File announcement] %s. Use /get %d to download.",
	"Room: #%s": "Room: #%s",
	"Room #%s berhasil dibuat sebagai master!":                                           "Room #%s created. You are the master!",
	"Berhasil bergabung ke room #%s!":                                                    "Joined room #%s!",
	"Belum ada percakapan aktif. Ketik /join <nama_room> atau /dm <nama_rekan>.":         "No active conversation. Use /join <room> or /dm <peer>.",
	"=== DAFTAR REKAN DI LAN ===\n":                                                      "=== LAN PEERS ===\n",
	"Belum ada rekan terdeteksi. Pastikan rekan sudah membuka aplikasi dan satu WiFi.\n": "No peers discovered. Make sure they are running the app on the same LAN.\n",
	"Gunakan: /dm <nama_rekan>":                                                          "Usage: /dm <peer>",
	"Rekan '%s' tidak ditemukan. Ketik /who untuk melihat daftar.":                       "Peer '%s' not found. Use /who to list peers.",
	"DM dengan %s":                         "DM with %s",
	"Membuka percakapan pribadi dengan %s": "Opened a private conversation with %s",
	"Gunakan: /join <nama_room>":           "Usage: /join <room>",
	"Mencari atau membuat room '#%s'...":   "Finding or creating room '#%s'...",
	"Perintah room yang tersedia:\n• /room new <nama> - Buat room baru\n• /room list - Daftar room yang diikuti\n• /room info [nama] - Detail dan anggota room\n• /room transfer <nama> <rekan> - Pindahkan master": "Room commands:\n/room new <name> - Create a room\n/room list - List joined rooms\n/room info [name] - Room details and members\n/room transfer <name> <peer> - Transfer ownership",
	"Gunakan: /room new <nama_room>":                          "Usage: /room new <room>",
	"=== DAFTAR ROOM YANG DIIKUTI ===\n":                      "=== JOINED ROOMS ===\n",
	"Room tidak ditemukan.":                                   "Room not found.",
	"=== INFO ROOM: #%s ===\n":                                "=== ROOM INFO: #%s ===\n",
	"Peran  : %s\n":                                           "Role   : %s\n",
	"Dibuat : %s\n":                                           "Created: %s\n",
	"Anggota (%d):\n":                                         "Members (%d):\n",
	"Gunakan: /room transfer <nama_room> <rekan>":             "Usage: /room transfer <room> <peer>",
	"Rekan '%s' tidak ditemukan.":                             "Peer '%s' not found.",
	"Kepemilikan room #%s diserahkan ke %s":                   "Ownership of room #%s transferred to %s",
	"Gunakan: /go <nama_room_atau_rekan>":                     "Usage: /go <room_or_peer>",
	"Percakapan '%s' tidak ditemukan.":                        "Conversation '%s' not found.",
	"Gunakan: /send <path_file>":                              "Usage: /send <file_path>",
	"Pilih percakapan aktif terlebih dahulu.":                 "Open a conversation first.",
	"Gunakan: /get <nomor_pengumuman>":                        "Usage: /get <announcement_number>",
	"Nomor pengumuman tidak valid.":                           "Invalid announcement number.",
	"Gunakan: /name <nama_baru>":                              "Usage: /name <new_name>",
	"Nama tampilan diubah menjadi: %s":                        "Display name changed to: %s",
	"Gunakan: /open <nomor_link>":                             "Usage: /open <link_number>",
	"Nomor link tidak valid.":                                 "Invalid link number.",
	"Link #%d tidak ditemukan.":                               "Link #%d not found.",
	"Membuka link: %s":                                        "Opening link: %s",
	"Gunakan: /add <ip:port>":                                 "Usage: /add <ip:port>",
	"[Galat Add] %v":                                          "[Add peer error] %v",
	"Mengirim broadcast langsung ke %s":                       "Sending an announcement directly to %s",
	"Perintah '%s' tidak dikenal. Maksud kamu %s?":            "Unknown command '%s'. Did you mean %s?",
	"Perintah '%s' tidak dikenal. Ketik /help untuk panduan.": "Unknown command '%s'. Type /help for commands.",
	"Kamu":                          "You",
	" [antre]":                      " [queued]",
	"room tidak ditemukan":          "room not found",
	"kamu tidak berada di room ini": "you have not joined this room",
	"hanya master room yang dapat membubarkan room ini":                     "only the room master can disband this room",
	"hanya master yang dapat mengalihkan kepemilikan room":                  "only the master can transfer room ownership",
	"nama room harus 2-32 karakter, hanya huruf kecil, angka, '-' atau '_'": "room names must be 2-32 characters: lowercase letters, digits, '-' or '_'",
	"batas maksimal 20 room per pengguna telah tercapai":                    "you have reached the limit of 20 rooms",
	"room ini sudah pernah dibubarkan oleh master":                          "this room was disbanded by its master",
}

func localized(language, text string, args ...any) string {
	if language == "en" {
		if translated, ok := englishText[text]; ok {
			text = translated
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}

func (m *AppModel) tr(text string, args ...any) string {
	return localized(m.cfg.Language, text, args...)
}

func (m *AppModel) changeLanguage(language string) {
	if language != "id" && language != "en" {
		m.addSystemLog(m.tr("Gunakan: /lang id atau /lang en"))
		return
	}
	previous := m.cfg.Language
	m.cfg.Language = language
	if err := m.cfg.Save(); err != nil {
		m.cfg.Language = previous
		m.addSystemLog(m.tr("[Galat] %v", err))
		return
	}
	m.statusText = m.tr(defaultStatusText)
	m.addSystemLog(m.tr("Bahasa diubah ke Indonesia."))
	m.resizePanels()
}

func (m *AppModel) roomEventText(event MsgRoomEvent) string {
	if m.cfg.Language != "en" {
		return fmt.Sprintf("[%s] #%s: %s", event.Event, event.Room, event.Detail)
	}
	labels := map[string]string{
		"ROOM_CREATED": "Room created", "ROOM_JOINED": "Joined room", "ROOM_LEFT": "Left room",
		"ROOM_DISBANDED": "Room disbanded", "ROOM_TRANSFERRED": "Room ownership changed",
		"MEMBER_JOINED": "A member joined", "MEMBER_LEFT": "A member left",
	}
	label, ok := labels[event.Event]
	if !ok {
		label = event.Event
	}
	return fmt.Sprintf("[#%s] %s", event.Room, label)
}
