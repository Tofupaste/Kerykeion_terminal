package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
	"kerykeion/pkg/transport"
	"kerykeion/pkg/ui"
)

func main() {
	var (
		customDir  string
		customName string
		tcpPort    int
		udpPort    int
		forceREPL  bool
		debugMode  bool
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

	options, err := runStartupCommand(flag.Args(), customDir, startupOptions{repl: forceREPL, debug: debugMode})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if options.handled {
		return
	}
	initialRoom := options.initialRoom
	forceREPL = options.repl

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

	runFirstRunWizard(cfg)

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

	events := &uiEvents{}

	msgSvc := messaging.New(idMgr, store, transMgr, disc, events.onMessage, events.onStatus)
	msgSvc.Start()
	defer msgSvc.Stop()

	roomMgr := room.New(idMgr, store, disc, transMgr, events.onRoomEvent)
	defer roomMgr.Stop()

	tferMgr := transfer.New(cfg, idMgr, store, disc, transMgr, events.onProgress, events.onAnnounce)
	tferMgr.Start()
	defer tferMgr.Stop()

	dispatcher := frameDispatcher{messages: msgSvc, rooms: roomMgr, transfers: tferMgr}
	transMgr.SetFrameHandler(dispatcher.handle)

	peers := peerCallbacks{config: cfg, store: store, messaging: msgSvc, rooms: roomMgr, events: events}
	disc.SetCallbacks(peers.discovered, peers.updated, peers.roomQuery, peers.roomHere)

	if err := disc.Start(cfg.TCPPort); err != nil {
		fmt.Fprintf(os.Stderr, "Peringatan UDP discovery (port %d): %v\n", cfg.UDPPort, err)
	}
	defer disc.Stop()

	services := ui.Services{
		Config: cfg, Identity: idMgr, Store: store, Discovery: disc,
		Messaging: msgSvc, Rooms: roomMgr, Transfers: tferMgr,
	}
	if forceREPL {
		replApp := ui.NewREPL(services, os.Stdin, os.Stdout, initialRoom)
		events.setREPL(replApp)
		replApp.Run()
	} else {
		appModel := ui.NewTUI(services, initialRoom)
		tuiProgram := tea.NewProgram(appModel, tea.WithAltScreen())
		events.setTUI(tuiProgram)
		if _, err := tuiProgram.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Galat menjalankan TUI: %v\n", err)
			os.Exit(1)
		}
	}
}
