package ui

import (
	"fmt"
	"strings"
)

type englishCommandHelp struct {
	usage       string
	description string
	example     string
}

var englishHelp = map[string]englishCommandHelp{
	"help":          {"/help [command]", "Show commands or detailed help for one command.", "/help room new"},
	"who":           {"/who", "List your identity and discovered LAN peers, including full IDs, name#ID selectors, and connection status.", "/who"},
	"dm":            {"/dm <name|ID|name#ID>", "Open a private conversation by name, full ID, unique ID prefix, or name#ID (name+ID also works). Duplicate names and ambiguous prefixes show choices. Use /who to copy an ID.", "/dm aria#12ab34cd"},
	"join":          {"/join <room>", "Join a LAN room, or create it if it cannot be found.", "/join ngobs"},
	"room":          {"/room <new|list|info|transfer> [arguments]", "Create rooms, list memberships, inspect members, or transfer ownership.", "/room info ngobs"},
	"room new":      {"/room new <name>", "Create a room. You become its master. Names use 2-32 lowercase letters, digits, '-' or '_'.", "/room new project-a"},
	"room list":     {"/room list", "List your active rooms and your role in each room.", "/room list"},
	"room info":     {"/room info [name]", "Show the room master, creation date, and members. Defaults to the active room.", "/room info ngobs"},
	"room transfer": {"/room transfer <room> <peer>", "Transfer room ownership to another peer. Only the master can do this.", "/room transfer ngobs aria"},
	"go":            {"/go <room_or_peer>", "Open a joined room or a private conversation, including minimized chats.", "/go ngobs"},
	"send":          {"/send <file_path>", "Send immediately to the active DM or prepare a room announcement. Paths may contain spaces; ~/ starts at home and relative paths start where kerxt was launched. In the TUI, focus chat and press Ctrl+O to choose and confirm a file.", `/send "~/Downloads/project notes.pdf"`},
	"get":           {"/get <number>", "Download a file using the number shown in this app. The owner sends it from cache; another room member can serve a cached copy if needed. Completion shows the saved path. Run from the system panel in the TUI.", "/get 1"},
	"leave":         {"/leave [room]", "Leave a room without closing it for other members.", "/leave ngobs"},
	"disband":       {"/disband [room]", "Disband a room for its members. Only the master can do this.", "/disband ngobs"},
	"name":          {"/name <new_name>", "Change your display name while keeping your cryptographic identity.", "/name Aria"},
	"open":          {"/open <number>", "Open a registered HTTP or HTTPS link in your default browser.", "/open 1"},
	"doctor":        {"/doctor", "Show network interfaces, listener status, and troubleshooting advice.", "/doctor"},
	"add":           {"/add <ip:port>", "Send a discovery announcement to a peer address when broadcast is restricted.", "/add 10.10.40.55:42100"},
	"lang":          {"/lang <id|en>", "Switch between Indonesian and English. This setting is saved. F2 also switches language.", "/lang en"},
	"quit":          {"/quit", "Close Kerykeion Terminal.", "/quit"},
}

func quickHelp(language string) string {
	var out strings.Builder
	if language == "en" {
		out.WriteString("KERYKEION COMMANDS\n\n")
	} else {
		out.WriteString("PERINTAH KERYKEION\n\n")
	}
	for _, name := range []string{"join", "dm", "who", "go", "room", "send", "get", "leave", "disband", "name", "open", "doctor", "add", "lang", "help", "quit"} {
		if language == "en" {
			help := englishHelp[name]
			fmt.Fprintf(&out, "%s\n  %s\n", help.usage, help.description)
		} else {
			help := CommandCatalog[name]
			fmt.Fprintf(&out, "%s\n  %s\n", help.Usage, help.Summary)
		}
	}
	out.WriteString("\n" + localized(language, "F2 Bahasa · F4 Chat · F6 Fokus · PgUp/PgDn Gulir · Ctrl+C Keluar"))
	out.WriteString("\n" + localized(language, "Ctrl+O Pilih file saat fokus chat (TUI)"))
	out.WriteString("\n" + localized(language, "Picker: Enter pilih lalu konfirmasi kirim; Backspace kembali; Esc batal."))
	out.WriteString("\n" + localized(language, "Esc Minimalkan chat"))
	return out.String()
}

func detailHelp(language, name string) string {
	name = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "/")
	if language != "en" {
		return FormatDetailHelp(name)
	}
	if help, ok := englishHelp[name]; ok {
		return fmt.Sprintf("HELP: /%s\nUsage: %s\n%s\nExample: %s", name, help.usage, help.description, help.example)
	}
	if suggestion := SuggestCommand(name); suggestion != "" {
		return fmt.Sprintf("Unknown command '/%s'. Did you mean %s?", name, suggestion)
	}
	return fmt.Sprintf("Unknown command '/%s'. Type /help for commands.", name)
}
