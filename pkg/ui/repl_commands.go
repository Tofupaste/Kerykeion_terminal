package ui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"kerykeion/pkg/doctor"
	"kerykeion/pkg/protocol"
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

func (r *REPL) commandAI(parts []string) {
	if r.aiSvc == nil {
		r.Printf("Layanan AI belum dikonfigurasi.\n")
		return
	}
	if len(parts) < 2 {
		r.Printf("Penggunaan /ai:\n  /ai <pertanyaan>                    - Tanya model AI kantor\n  /ai model                           - Cek model aktif & daftar model server\n  /ai model <nama>                    - Ganti model AI\n  /ai handoff <nama_rekan>            - Estafet sesi AI + Git Diff ke rekan\n  /ai ask @<rekan> [model] <tanya>    - Pinjam/delegasi inferensi ke model rekan\n  /ai share                           - Bagikan riwayat sesi ke percakapan\n  /ai clear                           - Bersihkan riwayat percakapan sesi aktif\n")
		return
	}

	sub := strings.ToLower(parts[1])
	switch sub {
	case "model":
		if len(parts) >= 3 {
			newModel := parts[2]
			r.aiSvc.SetModel(newModel)
			r.cfg.AIModel = newModel
			_ = r.cfg.Save()
			r.Printf("Model AI diubah ke: %s\n", newModel)
			return
		}
		activeModel := r.aiSvc.Model()
		r.Printf("Mengambil daftar model dari server AI kantor...\n")
		models, err := r.aiSvc.ListModels()
		if err != nil {
			r.Printf("Gagal memuat model server: %v\n", err)
			return
		}
		r.Printf("=== MODEL AI KANTOR (Aktif: %s) ===\n", activeModel)
		for _, mdl := range models {
			if mdl == activeModel {
				r.Printf("⭐ %s (sedang digunakan)\n", mdl)
			} else {
				r.Printf("- %s\n", mdl)
			}
		}
		r.Printf("\nGunakan: /ai model <nama_model> untuk mengganti.\n")
	case "clear":
		target := r.activeTarget
		if target == "" {
			target = "system"
		}
		r.aiSvc.ClearHistory(target)
		r.Printf("Riwayat percakapan AI untuk '%s' telah dibersihkan.\n", target)
	case "share":
		target := r.activeTarget
		if target == "" {
			target = "system"
		}
		data, err := r.aiSvc.ExportHistory(target)
		if err != nil || r.aiSvc.HistoryCount(target) == 0 {
			r.Printf("Belum ada riwayat AI untuk dibagikan pada percakapan ini.\n")
			return
		}
		summary := fmt.Sprintf("[AI Session Context - %s] Riwayat percakapan (%d pesan):\n%s", r.aiSvc.Model(), r.aiSvc.HistoryCount(target), string(data))
		if r.activeTarget != "" {
			if r.isRoom {
				_, _ = r.messaging.SendRoomMessage(r.activeTarget, summary, "")
			} else {
				_, _ = r.messaging.SendDM(r.activeTarget, summary)
			}
			r.Printf("Riwayat sesi AI berhasil dibagikan ke percakapan aktif!\n")
		} else {
			r.Printf("%s\n", summary)
		}
	case "handoff":
		if len(parts) < 3 {
			r.Printf("Gunakan: /ai handoff <nama_rekan>\n")
			return
		}
		targetPeer := strings.TrimPrefix(parts[2], "@")
		peer, ok := r.lookupPeer(targetPeer)
		if !ok {
			return
		}
		targetConv := r.activeTarget
		if targetConv == "" {
			targetConv = "project"
		}
		payload, err := r.aiSvc.CreateHandoffPayload(r.idMgr.PeerID(), targetConv)
		if err != nil {
			r.Printf("[Galat Handoff] %v\n", err)
			return
		}
		data, err := json.Marshal(payload)
		if err != nil {
			r.Printf("[Galat Handoff] %v\n", err)
			return
		}
		if r.transport != nil {
			if err := r.transport.SendToPeer(peer.PeerID, protocol.TypeAISessionSync, data); err != nil {
				r.Printf("Gagal mengirim handoff ke %s: %v\n", peer.DisplayName, err)
				return
			}
			diffInfo := ""
			if payload.GitDiff != "" {
				diffInfo = " beserta Git Diff terkini"
			}
			r.Printf("✓ Sesi AI (%d pesan)%s berhasil diserahkan ke @%s!\n", r.aiSvc.HistoryCount(targetConv), diffInfo, peer.DisplayName)
		}
	case "ask":
		if len(parts) < 4 || !strings.HasPrefix(parts[2], "@") {
			r.Printf("Gunakan: /ai ask @<nama_rekan> [model] <pertanyaan>\n")
			return
		}
		targetPeer := strings.TrimPrefix(parts[2], "@")
		peer, ok := r.lookupPeer(targetPeer)
		if !ok {
			return
		}

		targetModel := ""
		promptStart := 3
		candidateModel := strings.ToLower(parts[3])
		if len(parts) >= 5 && (strings.Contains(candidateModel, "claude") || strings.Contains(candidateModel, "qwen") || strings.Contains(candidateModel, "gemini") || strings.Contains(candidateModel, "codex") || strings.Contains(candidateModel, "gpt")) {
			targetModel = parts[3]
			promptStart = 4
		}
		prompt := strings.Join(parts[promptStart:], " ")
		reqID := uuid.NewString()

		if targetModel != "" {
			r.Printf("Meminta inferensi model [%s] dari @%s: \"%s\"...\n", targetModel, peer.DisplayName, prompt)
		} else {
			r.Printf("Meminta inferensi model dari @%s: \"%s\"...\n", peer.DisplayName, prompt)
		}

		reqPayload := protocol.AIQueryReqPayload{
			V:           1,
			ReqID:       reqID,
			FromPeerID:  r.idMgr.PeerID(),
			TargetModel: targetModel,
			Prompt:      prompt,
			TS:          time.Now().Unix(),
		}
		reqBytes, err := json.Marshal(reqPayload)
		if err != nil {
			r.Printf("Gagal membentuk query: %v\n", err)
			return
		}
		ch := r.aiSvc.RegisterPendingQuery(reqID)
		defer r.aiSvc.UnregisterPendingQuery(reqID)

		if r.transport == nil {
			r.Printf("Transport belum siap.\n")
			return
		}
		if err := r.transport.SendToPeer(peer.PeerID, protocol.TypeAIQueryReq, reqBytes); err != nil {
			r.Printf("Gagal mengirim query ke @%s: %v\n", peer.DisplayName, err)
			return
		}

		select {
		case res := <-ch:
			if res.Error != "" {
				r.Printf("[Galat AI dari @%s] %s\n", peer.DisplayName, res.Error)
			} else {
				r.Printf("[%s via @%s]: %s\n", res.Model, peer.DisplayName, res.Answer)
			}
		case <-time.After(60 * time.Second):
			r.Printf("Batas waktu menunggu respons AI dari @%s habis.\n", peer.DisplayName)
		}
	default:
		prompt := strings.Join(parts[1:], " ")
		target := r.activeTarget
		if target == "" {
			target = "system"
		}
		r.Printf("Mengirim pertanyaan ke model AI [%s]...\n", r.aiSvc.Model())
		ans, err := r.aiSvc.Ask(target, prompt)
		if err != nil {
			r.Printf("[Galat AI] %v\n", err)
			return
		}
		r.Printf("%s\n", ans)
	}
}
