package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/doctor"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/installer"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
	"kerykeion/pkg/transport"
	"kerykeion/pkg/ui"
)

func main() {
	var (
		customDir   string
		customName  string
		tcpPort     int
		udpPort     int
		forceREPL   bool
		debugMode   bool
	)

	flag.StringVar(&customDir, "config", "", "Direktori konfigurasi kustom")
	flag.StringVar(&customName, "name", "", "Nama tampilan")
	flag.IntVar(&tcpPort, "port", 0, "Port TCP untuk chat dan transfer file")
	flag.IntVar(&udpPort, "udp-port", 0, "Port UDP untuk penemuan peer (discovery)")
	flag.BoolVar(&forceREPL, "repl", false, "Gunakan antarmuka baris perintah (REPL) bukan TUI")
	flag.BoolVar(&debugMode, "debug", false, "Aktifkan log debug rinci")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Penggunaan: kerxt [perintah] [opsi]\n\n")
		fmt.Fprintf(os.Stderr, "Perintah:\n")
		fmt.Fprintf(os.Stderr, "  install             Pasang otomatis (deteksi OS, kompilasi, salin biner, setel PATH)\n")
		fmt.Fprintf(os.Stderr, "  uninstall           Hapus instalasi biner (gunakan --purge untuk bersihkan data)\n")
		fmt.Fprintf(os.Stderr, "  join <nama_room>    Langsung masuk atau buat room tertentu\n")
		fmt.Fprintf(os.Stderr, "  doctor              Jalankan tes diagnosa jaringan dan firewall\n\n")
		fmt.Fprintf(os.Stderr, "Opsi:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	args := flag.Args()
	initialRoom := ""

	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "install":
			targetDir := ""
			if len(args) > 1 {
				targetDir = args[1]
			}
			err := installer.RunInstall(installer.InstallOptions{
				TargetDir: targetDir,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Galat saat instalasi: %v\n", err)
				os.Exit(1)
			}
			return

		case "uninstall":
			purge := false
			for _, a := range args[1:] {
				if a == "--purge" || a == "-purge" {
					purge = true
				}
			}
			err := installer.RunUninstall(purge)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Galat saat uninstall: %v\n", err)
				os.Exit(1)
			}
			return

		case "doctor":
			cfg, err := config.LoadConfig(customDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Galat memuat konfigurasi: %v\n", err)
				os.Exit(1)
			}
			report := doctor.RunDiagnostics(cfg, nil)
			fmt.Println(doctor.FormatReport(report))
			return

		case "join":
			if len(args) > 1 {
				initialRoom = args[1]
			}
		}
	}

	cfg, err := config.LoadConfig(customDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Galat memuat konfigurasi: %v\n", err)
		os.Exit(1)
	}

	if tcpPort != 0 {
		cfg.TCPPort = tcpPort
	}
	if udpPort != 0 {
		cfg.UDPPort = udpPort
	}
	if customName != "" {
		cfg.DisplayName = customName
	}

	// First run wizard if display name is empty
	if cfg.DisplayName == "" {
		fmt.Println("==================================================")
		fmt.Println("     Selamat Datang di Kerykeion Terminal (P2P)   ")
		fmt.Println("==================================================")
		fmt.Println("Aplikasi chat & file sharing langsung antar-laptop")
		fmt.Println("tanpa internet dan tanpa server pusat.")
		fmt.Println()
		fmt.Print("Masukkan nama tampilan kamu: ")
		reader := bufio.NewReader(os.Stdin)
		inputName, _ := reader.ReadString('\n')
		inputName = strings.TrimSpace(inputName)
		if inputName == "" {
			inputName = "anon"
		}
		cfg.DisplayName = inputName
		_ = cfg.Save()
		fmt.Printf("\nNama kamu disetel sebagai: %s\n", inputName)
		fmt.Println("Panduan Cepat:")
		fmt.Println(" 1. Ketik /who untuk melihat siapa saja yang online di WiFi.")
		fmt.Println(" 2. Ketik /join <nama_room> untuk mengobrol bersama.")
		fmt.Println(" 3. Ketik /send <file> untuk mengirim berkas.")
		fmt.Println("--------------------------------------------------")
	}

	idMgr, err := identity.LoadOrCreate(cfg.ConfigDir, cfg.DisplayName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Galat identitas: %v\n", err)
		os.Exit(1)
	}

	store, err := db.Open(cfg.ConfigDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Galat database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	_ = store.SaveIdentity(idMgr.PeerID(), idMgr.DisplayName())
	_ = store.PruneRetention(cfg.RetentionDays)

	transMgr, err := transport.New(idMgr, cfg.TCPPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Galat inisialisasi transport: %v\n", err)
		os.Exit(1)
	}
	if err := transMgr.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Galat membuka listener TCP port %d: %v\n", cfg.TCPPort, err)
		os.Exit(1)
	}
	cfg.TCPPort = transMgr.Port()
	defer transMgr.Stop()

	disc := discovery.New(idMgr, cfg.UDPPort, cfg.OfficeKey)

	var (
		tuiProgram *tea.Program
		replApp    *ui.REPL
	)

	onNewMessage := func(m db.MessageRecord, isSelf bool) {
		if tuiProgram != nil {
			tuiProgram.Send(ui.MsgNewMessage(m))
		}
		if replApp != nil {
			replApp.OnNewMessage(m, isSelf)
		}
	}

	onStatusChange := func(msgID, status string) {
		if tuiProgram != nil {
			tuiProgram.Send(ui.MsgStatusChange{MsgID: msgID, Status: status})
		}
	}

	onProgress := func(p transfer.TransferProgress) {
		if tuiProgram != nil {
			tuiProgram.Send(ui.MsgProgress(p))
		}
	}

	onAnnounce := func(a protocol.FileAnnouncePayload) {
		if tuiProgram != nil {
			tuiProgram.Send(ui.MsgAnnounce(a))
		}
	}

	onRoomEvent := func(evt, rName, detail string) {
		if tuiProgram != nil {
			tuiProgram.Send(ui.MsgRoomEvent{Event: evt, Room: rName, Detail: detail})
		}
		if replApp != nil {
			replApp.Printf("[%s] #%s: %s\n", evt, rName, detail)
		}
	}

	msgSvc := messaging.New(idMgr, store, transMgr, disc, onNewMessage, onStatusChange)
	msgSvc.Start()
	defer msgSvc.Stop()

	roomMgr := room.New(idMgr, store, disc, transMgr, onRoomEvent)
	defer roomMgr.Stop()

	tferMgr := transfer.New(cfg, idMgr, store, disc, transMgr, onProgress, onAnnounce)
	tferMgr.Start()
	defer tferMgr.Stop()

	transMgr.SetFrameHandler(func(connID string, frameType byte, payload []byte, remotePeerID string) {
		switch frameType {
		case protocol.TypeMsg:
			msgSvc.HandleIncomingMsg(payload, remotePeerID)
		case protocol.TypeMsgAck:
			msgSvc.HandleIncomingMsgAck(payload)
		case protocol.TypeRoomJoin:
			roomMgr.HandleRoomJoin(payload)
		case protocol.TypeRoomLeave:
			roomMgr.HandleRoomLeave(payload)
		case protocol.TypeRoomClosed:
			roomMgr.HandleRoomClosed(payload)
		case protocol.TypeRoomTransfer:
			roomMgr.HandleRoomTransfer(payload)
		case protocol.TypeRoomSyncReq:
			roomMgr.HandleRoomSyncReq(payload, remotePeerID)
		case protocol.TypeRoomSyncRes:
			roomMgr.HandleRoomSyncRes(payload)
		case protocol.TypeFileOffer:
			tferMgr.HandleFileOffer(connID, payload, remotePeerID)
		case protocol.TypeFileAccept:
			tferMgr.HandleFileAccept(payload)
		case protocol.TypeChunk:
			tferMgr.HandleChunk(connID, payload, remotePeerID)
		case protocol.TypeFileDone:
			tferMgr.HandleFileDone(payload)
		case protocol.TypeFileAnnounce:
			tferMgr.HandleFileAnnounce(payload)
		case protocol.TypeFileWhoHas:
			tferMgr.HandleFileWhoHas(payload, remotePeerID)
		case protocol.TypeFileHave:
			tferMgr.HandleFileHave(payload)
		}
	})

	disc.SetCallbacks(
		func(p discovery.PeerInfo) {
			_ = store.UpsertPeer(db.PeerRecord{
				PeerID:      p.PeerID,
				DisplayName: p.DisplayName,
				PubKey:      p.PubKey,
				LastIP:      p.IP,
				LastPort:    p.Port,
				LastSeen:    p.LastSeen,
				Trusted:     true,
			})
			msgSvc.TriggerPeerRetry(p.PeerID)
			if tuiProgram != nil {
				tuiProgram.Send(ui.MsgPeerDiscovered(p))
			}
		},
		func(p discovery.PeerInfo) {
			_ = store.UpsertPeer(db.PeerRecord{
				PeerID:      p.PeerID,
				DisplayName: p.DisplayName,
				PubKey:      p.PubKey,
				LastIP:      p.IP,
				LastPort:    p.Port,
				LastSeen:    p.LastSeen,
				Trusted:     true,
			})
			msgSvc.TriggerPeerRetry(p.PeerID)
			if tuiProgram != nil {
				tuiProgram.Send(ui.MsgPeerUpdated(p))
			}
		},
		func(q protocol.RoomQueryPacket, addr *net.UDPAddr) {
			roomMgr.HandleRoomQuery(q, cfg.TCPPort)
		},
		func(h protocol.RoomHerePacket, addr *net.UDPAddr) {
			roomMgr.HandleRoomHere(h)
		},
	)

	if err := disc.Start(cfg.TCPPort); err != nil {
		fmt.Fprintf(os.Stderr, "Peringatan UDP discovery (port %d): %v\n", cfg.UDPPort, err)
	}
	defer disc.Stop()

	if initialRoom != "" {
		go func() {
			time.Sleep(500 * time.Millisecond)
			_, _, _ = roomMgr.JoinRoom(initialRoom, cfg.TCPPort)
		}()
	}

	if forceREPL {
		replApp = ui.NewREPL(cfg, idMgr, store, disc, msgSvc, roomMgr, tferMgr, os.Stdin, os.Stdout)
		replApp.Run()
	} else {
		appModel := ui.NewTUI(cfg, idMgr, store, disc, msgSvc, roomMgr, tferMgr)
		tuiProgram = tea.NewProgram(appModel, tea.WithAltScreen())
		if _, err := tuiProgram.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Galat menjalankan TUI: %v\n", err)
			os.Exit(1)
		}
	}
}
