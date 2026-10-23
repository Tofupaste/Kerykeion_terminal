package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
)

type REPL struct {
	cfg       *config.Config
	idMgr     *identity.Manager
	store     *db.Store
	discovery *discovery.Discovery
	messaging *messaging.Service
	roomMgr   *room.Manager
	transfer  *transfer.Manager

	mu           sync.Mutex
	activeTarget string
	isRoom       bool
	in           io.Reader
	out          io.Writer
	running      bool
	initialRoom  string
}

func NewREPL(
	services Services,
	in io.Reader,
	out io.Writer,
	initialRoom string,
) *REPL {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	return &REPL{
		cfg:         services.Config,
		idMgr:       services.Identity,
		store:       services.Store,
		discovery:   services.Discovery,
		messaging:   services.Messaging,
		roomMgr:     services.Rooms,
		transfer:    services.Transfers,
		in:          in,
		out:         out,
		initialRoom: initialRoom,
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
			target = "@" + peerIDPrefix(r.activeTarget, r.discovery.GetPeers())
		}
	}
	fmt.Fprintf(r.out, "\n[%s]> ", target)
}

func (r *REPL) Run() {
	r.running = true
	r.Printf("Kerykeion Terminal REPL siap. Ketik /help untuk daftar perintah.\n")

	r.joinInitialRoom()

	r.Prompt()

	scanner := bufio.NewScanner(r.in)
	for scanner.Scan() {
		if !r.running {
			break
		}
		r.processLine(scanner.Text())
		if r.running {
			r.Prompt()
		}
	}
}

func (r *REPL) joinInitialRoom() {
	if r.initialRoom == "" {
		return
	}
	rec, joined, err := r.roomMgr.JoinOrCreateRoom(r.initialRoom, r.cfg.TCPPort)
	if err != nil || rec == nil {
		return
	}
	r.activeTarget, r.isRoom = rec.Name, true
	if joined {
		r.Printf("Bergabung ke room #%s.\n", rec.Name)
	} else {
		r.Printf("Room #%s berhasil dibuat sebagai master!\n", rec.Name)
	}
}

func (r *REPL) processLine(input string) {
	line := strings.TrimSpace(input)
	if line == "" {
		return
	}
	if strings.HasPrefix(line, "/") {
		r.executeCommand(line)
	} else {
		r.sendMessage(line)
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
	parts := commandParts(raw)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])

	handlers := map[string]func([]string){
		"/help":    r.commandHelp,
		"/who":     r.commandWho,
		"/dm":      r.commandDm,
		"/join":    r.commandJoin,
		"/room":    r.commandRoom,
		"/go":      r.commandGo,
		"/send":    r.commandSend,
		"/get":     r.commandGet,
		"/leave":   r.commandLeave,
		"/disband": r.commandDisband,
		"/name":    r.commandName,
		"/open":    r.commandOpen,
		"/doctor":  r.commandDoctor,
		"/add":     r.commandAdd,
		"/quit":    r.commandQuit,
	}
	if handler, ok := handlers[cmd]; ok {
		handler(parts)
		return
	}

	sugg := SuggestCommand(cmd)
	if sugg != "" {
		r.Printf("Perintah '%s' tidak dikenal. Maksud kamu %s?\n", cmd, sugg)
	} else {
		r.Printf("Perintah '%s' tidak dikenal. Ketik /help untuk panduan.\n", cmd)
	}
}

func (r *REPL) OnNewMessage(msg db.MessageRecord, isSelf bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tStr := time.Unix(msg.Timestamp, 0).Format("15:04")
	authorName := peerIDPrefix(msg.AuthorID, r.discovery.GetPeers())
	if isSelf {
		authorName = "Kamu"
	} else if p, ok := r.discovery.GetPeer(msg.AuthorID); ok {
		authorName = peerLabel(*p, r.discovery.GetPeers())
	}

	convPrefix := ""
	if msg.ConversationID != "" && msg.ConversationID != r.idMgr.PeerID() {
		convPrefix = fmt.Sprintf("[%s] ", msg.ConversationID)
	}

	fmt.Fprintf(r.out, "\n%s%s <%s>: %s\n", convPrefix, tStr, authorName, msg.Body)
}
