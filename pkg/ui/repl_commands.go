package ui

import (
	"strconv"
	"strings"

	"kerykeion/pkg/doctor"
)

func (r *REPL) commandHelp(parts []string) {
	if len(parts) > 1 {
		r.Printf("%s\n", FormatDetailHelp(parts[1]))
	} else {
		r.Printf("%s\n", FormatQuickHelp())
	}
}

func (r *REPL) commandWho(parts []string) {
	peers := r.discovery.GetPeers()
	r.Printf("=== DAFTAR REKAN DI LAN ===\n")
	r.Printf("%s", localized(r.cfg.Language, "Identitas kamu: %s\nID: %s\n", r.idMgr.DisplayName(), r.idMgr.PeerID()))
	if len(peers) == 0 {
		r.Printf("Belum ada rekan terdeteksi di jaringan.\n")
	}
	for _, p := range peers {
		st := "🟢 online"
		if !p.IsOnline {
			st = "⚪ stale"
		}
		r.Printf("- %s (%s) IP %s:%d\n  ID: %s\n", peerLabel(p, peers), st, p.IP, p.Port, p.PeerID)
	}
}

func (r *REPL) commandDm(parts []string) {
	if len(parts) < 2 {
		r.Printf("%s\n", localized(r.cfg.Language, "Gunakan: /dm <nama|ID|nama#ID>"))
		return
	}
	target := parts[1]
	peer, ok := r.lookupPeer(target)
	if !ok {
		return
	}
	r.activeTarget = peer.PeerID
	r.isRoom = false
	r.Printf("%s\n", localized(r.cfg.Language, "Membuka percakapan pribadi dengan %s", peerLabel(*peer, r.discovery.GetPeers())))
}

func (r *REPL) commandJoin(parts []string) {
	if len(parts) < 2 {
		r.Printf("Gunakan: /join <nama_room>\n")
		return
	}
	roomName := strings.TrimPrefix(parts[1], "#")
	rec, joined, err := r.roomMgr.JoinOrCreateRoom(roomName, r.cfg.TCPPort)
	if err != nil {
		r.Printf("[Galat] %v\n", err)
		return
	}
	r.activeTarget = rec.Name
	r.isRoom = true
	if joined {
		r.Printf("Bergabung ke room #%s.\n", rec.Name)
	} else {
		r.Printf("Room #%s berhasil dibuat sebagai master!\n", rec.Name)
	}
}

func (r *REPL) commandRoom(parts []string) {
	if len(parts) < 2 {
		r.Printf("Perintah room:\n  • /room new <nama>           Buat room baru\n  • /room list                 Lihat room yang diikuti\n  • /room info [nama]          Detail dan anggota room\n  • /room transfer <nama> <rekan> Pindahkan peran master\n")
		return
	}
	sub := strings.ToLower(parts[1])
	handlers := map[string]func([]string){
		"new":      r.commandRoomNew,
		"list":     r.commandRoomList,
		"info":     r.commandRoomInfo,
		"transfer": r.commandRoomTransfer,
	}
	if handler, ok := handlers[sub]; ok {
		handler(parts)
		return
	}

}

func (r *REPL) commandGo(parts []string) {
	if len(parts) < 2 {
		r.Printf("Gunakan: /go <nama_room_atau_rekan>\n")
		return
	}
	target := parts[1]
	if rm, err := r.store.GetRoom(target); err == nil && rm != nil && rm.State == "active" {
		r.activeTarget = rm.Name
		r.isRoom = true
		r.Printf("Pindah ke room #%s.\n", rm.Name)
		return
	}
	if p, ok := r.lookupPeer(target); ok {
		r.activeTarget = p.PeerID
		r.isRoom = false
		r.Printf("%s\n", localized(r.cfg.Language, "Membuka percakapan pribadi dengan %s", peerLabel(*p, r.discovery.GetPeers())))
		return
	}
}

func (r *REPL) commandSend(parts []string) {
	if len(parts) < 2 {
		r.Printf("Gunakan: /send <path_file>\n")
		return
	}
	if r.activeTarget == "" {
		r.Printf("Pilih percakapan aktif terlebih dahulu.\n")
		return
	}
	filePath := parts[1]
	target, isRoom := r.activeTarget, r.isRoom
	go func() {
		resolved, err := resolveAttachmentPath(filePath)
		if err == nil {
			_, err = attachmentInfo(resolved, r.cfg.MaxFileSize)
		}
		if err != nil {
			r.Printf("[Galat Send] %v\n", err)
			return
		}
		if isRoom {
			_, err = r.transfer.SendFile(resolved, "", target)
		} else {
			_, err = r.transfer.SendFile(resolved, target, "")
		}
		if err != nil {
			r.Printf("[Galat Send] %v\n", err)
		}
	}()
}

func (r *REPL) commandGet(parts []string) {
	if len(parts) < 2 {
		r.Printf("Gunakan: /get <nomor_pengumuman>\n")
		return
	}
	num, err := strconv.Atoi(parts[1])
	if err != nil {
		r.Printf("Nomor pengumuman tidak valid.\n")
		return
	}
	r.Printf("%s\n", localized(r.cfg.Language, "Meminta file #%d...", num))
	go func() {
		err := r.transfer.PullAnnouncedFile(num)
		if err != nil {
			r.Printf("[Galat Get] %v\n", err)
		}
	}()
}

func (r *REPL) commandLeave(parts []string) {
	target := r.activeTarget
	if len(parts) >= 2 {
		target = parts[1]
	}
	err := r.roomMgr.LeaveRoom(target)
	if err != nil {
		r.Printf("[Galat Leave] %v\n", err)
		return
	}
	r.activeTarget = ""
	r.Printf("Telah keluar dari room #%s.\n", target)
}

func (r *REPL) commandDisband(parts []string) {
	target := r.activeTarget
	if len(parts) >= 2 {
		target = parts[1]
	}
	err := r.roomMgr.DisbandRoom(target)
	if err != nil {
		r.Printf("[Galat Disband] %v\n", err)
		return
	}
	r.activeTarget = ""
	r.Printf("Room #%s telah dibubarkan.\n", target)
}

func (r *REPL) commandName(parts []string) {
	if len(parts) < 2 {
		r.Printf("Gunakan: /name <nama_baru>\n")
		return
	}
	newName := parts[1]
	r.idMgr.SetDisplayName(newName)
	r.cfg.DisplayName = newName
	_ = r.cfg.Save()
	_ = r.store.SaveIdentity(r.idMgr.PeerID(), newName)
	r.Printf("Nama tampilan diubah menjadi: %s\n", newName)
}

func (r *REPL) commandOpen(parts []string) {
	if len(parts) < 2 {
		r.Printf("Gunakan: /open <nomor_link>\n")
		return
	}
	num, err := strconv.Atoi(parts[1])
	if err != nil {
		r.Printf("Nomor link tidak valid.\n")
		return
	}
	link, ok := r.messaging.GetLink(num)
	if !ok {
		r.Printf("Link #%d tidak ditemukan.\n", num)
		return
	}
	openBrowser(link)
	r.Printf("Membuka link: %s\n", link)
}

func (r *REPL) commandDoctor(parts []string) {
	report := doctor.RunDiagnosticsForApp(r.cfg, r.discovery, r.cfg.TCPPort)
	r.Printf("%s\n", doctor.FormatReport(report))
}

func (r *REPL) commandAdd(parts []string) {
	if len(parts) < 2 {
		r.Printf("Gunakan: /add <ip:port>\n")
		return
	}
	_, err := r.discovery.AddManualPeer(parts[1])
	if err != nil {
		r.Printf("[Galat Add] %v\n", err)
	} else {
		r.Printf("Broadcast terkirim ke %s\n", parts[1])
	}
}

func (r *REPL) commandQuit(parts []string) {
	r.Printf("Sampai jumpa!\n")
	r.running = false
}
