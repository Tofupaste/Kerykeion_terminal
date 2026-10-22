package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/doctor"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
)

type REPL struct {
	cfg        *config.Config
	idMgr      *identity.Manager
	store      *db.Store
	discovery  *discovery.Discovery
	messaging  *messaging.Service
	roomMgr    *room.Manager
	transfer   *transfer.Manager

	mu           sync.Mutex
	activeTarget string
	isRoom       bool
	in           io.Reader
	out          io.Writer
	running      bool
}

func NewREPL(
	cfg *config.Config,
	idMgr *identity.Manager,
	store *db.Store,
	disc *discovery.Discovery,
	msgSvc *messaging.Service,
	rMgr *room.Manager,
	tMgr *transfer.Manager,
	in io.Reader,
	out io.Writer,
) *REPL {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	return &REPL{
		cfg:       cfg,
		idMgr:     idMgr,
		store:     store,
		discovery: disc,
		messaging: msgSvc,
		roomMgr:   rMgr,
		transfer:  tMgr,
		in:        in,
		out:       out,
	}
}

func (r *REPL) Printf(format string, a ...interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.out, format, a...)
}

func (r *REPL) Prompt() {
	r.mu.Lock()
	defer r.mu.Unlock()
	target := "(tanpa percakapan)"
	if r.activeTarget != "" {
		if r.isRoom {
			target = "#" + r.activeTarget
		} else {
			target = "@" + r.activeTarget[:6]
		}
	}
	fmt.Fprintf(r.out, "\n[%s]> ", target)
}

func (r *REPL) Run() {
	r.running = true
	r.Printf("Kerykeion Terminal REPL siap. Ketik /help untuk daftar perintah.\n")
	r.Prompt()

	scanner := bufio.NewScanner(r.in)
	for scanner.Scan() {
		if !r.running {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			if strings.HasPrefix(line, "/") {
				r.executeCommand(line)
			} else {
				r.sendMessage(line)
			}
		}
		if r.running {
			r.Prompt()
		}
	}
}

func (r *REPL) Stop() {
	r.running = false
}

func (r *REPL) sendMessage(body string) {
	if r.activeTarget == "" {
		r.Printf("Belum ada percakapan aktif. Gunakan /join <nama_room> atau /dm <nama_rekan>.\n")
		return
	}

	if r.isRoom {
		_, err := r.messaging.SendRoomMessage(r.activeTarget, body, "")
		if err != nil {
			r.Printf("[Galat] %v\n", err)
		}
	} else {
		_, err := r.messaging.SendDM(r.activeTarget, body)
		if err != nil {
			r.Printf("[Galat] %v\n", err)
		}
	}
}

func (r *REPL) executeCommand(raw string) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/help":
		if len(parts) > 1 {
			r.Printf("%s\n", FormatDetailHelp(parts[1]))
		} else {
			r.Printf("%s\n", FormatQuickHelp())
		}

	case "/who":
		peers := r.discovery.GetPeers()
		r.Printf("=== DAFTAR REKAN DI LAN ===\n")
		if len(peers) == 0 {
			r.Printf("Belum ada rekan terdeteksi di jaringan.\n")
		}
		for _, p := range peers {
			st := "🟢 online"
			if !p.IsOnline {
				st = "⚪ stale"
			}
			r.Printf("- %s (%s) IP %s:%d (ID: %s)\n", p.DisplayName, st, p.IP, p.Port, p.PeerID[:8])
		}

	case "/dm":
		if len(parts) < 2 {
			r.Printf("Gunakan: /dm <nama_rekan>\n")
			return
		}
		target := parts[1]
		peer, ok := r.discovery.FindPeerByName(target)
		if !ok {
			for _, p := range r.discovery.GetPeers() {
				if strings.HasPrefix(p.PeerID, target) {
					peer = &p
					ok = true
					break
				}
			}
		}
		if !ok {
			r.Printf("Rekan '%s' tidak ditemukan.\n", target)
			return
		}
		r.activeTarget = peer.PeerID
		r.isRoom = false
		r.Printf("Beralih ke pesan pribadi dengan %s.\n", peer.DisplayName)

	case "/join":
		if len(parts) < 2 {
			r.Printf("Gunakan: /join <nama_room>\n")
			return
		}
		roomName := parts[1]
		rec, joined, err := r.roomMgr.JoinRoom(roomName, r.cfg.TCPPort)
		if err != nil {
			r.Printf("[Galat] %v\n", err)
			return
		}
		if !joined {
			r.Printf("Room '%s' belum ada di jaringan. Ketik '/room new %s' untuk membuatnya.\n", roomName, roomName)
			return
		}
		r.activeTarget = rec.Name
		r.isRoom = true
		r.Printf("Bergabung ke room #%s.\n", rec.Name)

	case "/room":
		if len(parts) < 2 {
			r.Printf("Gunakan: /room <new|list|info|transfer>\n")
			return
		}
		sub := strings.ToLower(parts[1])
		switch sub {
		case "new":
			if len(parts) < 3 {
				r.Printf("Gunakan: /room new <nama_room>\n")
				return
			}
			rec, err := r.roomMgr.CreateRoom(parts[2])
			if err != nil {
				r.Printf("[Galat] %v\n", err)
				return
			}
			r.activeTarget = rec.Name
			r.isRoom = true
			r.Printf("Room #%s berhasil dibuat sebagai master!\n", rec.Name)

		case "list":
			rooms, err := r.store.GetAllRooms()
			if err != nil || len(rooms) == 0 {
				r.Printf("Belum ada room yang diikuti.\n")
				return
			}
			r.Printf("=== DAFTAR ROOM ===\n")
			for _, rm := range rooms {
				role := "guest"
				if rm.MyRole == "master" {
					role = "👑 master"
				}
				r.Printf("- #%s [%s]\n", rm.Name, role)
			}

		case "info":
			name := r.activeTarget
			if len(parts) >= 3 {
				name = parts[2]
			}
			rec, err := r.store.GetRoom(name)
			if err != nil || rec == nil {
				r.Printf("Room '%s' tidak ditemukan.\n", name)
				return
			}
			members, _ := r.store.GetRoomMembers(name)
			r.Printf("=== INFO ROOM: #%s ===\n", rec.Name)
			r.Printf("Master : %s\n", rec.MasterID)
			r.Printf("Peran  : %s\n", rec.MyRole)
			r.Printf("Dibuat : %s\n", rec.CreatedAt.Format("02 Jan 2006 15:04"))
			r.Printf("Anggota (%d):\n", len(members))
			for _, mID := range members {
				r.Printf("  - %s\n", mID)
			}

		case "transfer":
			if len(parts) < 4 {
				r.Printf("Gunakan: /room transfer <nama_room> <rekan>\n")
				return
			}
			targetPeer, ok := r.discovery.FindPeerByName(parts[3])
			if !ok {
				r.Printf("Rekan '%s' tidak ditemukan.\n", parts[3])
				return
			}
			err := r.roomMgr.TransferMaster(parts[2], targetPeer.PeerID)
			if err != nil {
				r.Printf("[Galat] %v\n", err)
				return
			}
			r.Printf("Kepemilikan room #%s diserahkan ke %s.\n", parts[2], targetPeer.DisplayName)
		}

	case "/go":
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
		if p, ok := r.discovery.FindPeerByName(target); ok {
			r.activeTarget = p.PeerID
			r.isRoom = false
			r.Printf("Pindah ke DM dengan %s.\n", p.DisplayName)
			return
		}
		r.Printf("Percakapan '%s' tidak ditemukan.\n", target)

	case "/send":
		if len(parts) < 2 {
			r.Printf("Gunakan: /send <path_file>\n")
			return
		}
		if r.activeTarget == "" {
			r.Printf("Pilih percakapan aktif terlebih dahulu.\n")
			return
		}
		filePath := parts[1]
		go func() {
			if r.isRoom {
				_, err := r.transfer.SendFile(filePath, "", r.activeTarget)
				if err != nil {
					r.Printf("[Galat Send] %v\n", err)
				}
			} else {
				_, err := r.transfer.SendFile(filePath, r.activeTarget, "")
				if err != nil {
					r.Printf("[Galat Send] %v\n", err)
				}
			}
		}()

	case "/get":
		if len(parts) < 2 {
			r.Printf("Gunakan: /get <nomor_pengumuman>\n")
			return
		}
		num, err := strconv.Atoi(parts[1])
		if err != nil {
			r.Printf("Nomor pengumuman tidak valid.\n")
			return
		}
		go func() {
			err := r.transfer.PullAnnouncedFile(num)
			if err != nil {
				r.Printf("[Galat Get] %v\n", err)
			}
		}()

	case "/leave":
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

	case "/disband":
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

	case "/name":
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

	case "/open":
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

	case "/doctor":
		report := doctor.RunDiagnostics(r.cfg, r.discovery)
		r.Printf("%s\n", doctor.FormatReport(report))

	case "/add":
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

	case "/quit":
		r.Printf("Sampai jumpa!\n")
		r.running = false

	default:
		sugg := SuggestCommand(cmd)
		if sugg != "" {
			r.Printf("Perintah '%s' tidak dikenal. Maksud kamu %s?\n", cmd, sugg)
		} else {
			r.Printf("Perintah '%s' tidak dikenal. Ketik /help untuk panduan.\n", cmd)
		}
	}
}

func (r *REPL) OnNewMessage(msg db.MessageRecord, isSelf bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tStr := time.Unix(msg.Timestamp, 0).Format("15:04")
	authorName := msg.AuthorID[:6]
	if isSelf {
		authorName = "Kamu"
	} else if p, ok := r.discovery.GetPeer(msg.AuthorID); ok {
		authorName = p.DisplayName
	}

	convPrefix := ""
	if msg.ConversationID != "" && msg.ConversationID != r.idMgr.PeerID() {
		convPrefix = fmt.Sprintf("[%s] ", msg.ConversationID)
	}

	fmt.Fprintf(r.out, "\n%s%s <%s>: %s\n", convPrefix, tStr, authorName, msg.Body)
}
