package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"kerykeion/pkg/discovery"
)

type copyIDItem struct{ label, value string }
type copyIDMenu struct {
	items      []copyIDItem
	selected   int
	pending    bool
	status     string
	generation uint64
}
type clipboardResult struct {
	generation uint64
	label      string
	err        error
}

func (m *AppModel) openCopyIDs() {
	m.copyGeneration++
	own := discovery.PeerInfo{DisplayName: m.idMgr.DisplayName(), PeerID: m.idMgr.PeerID()}
	items := []copyIDItem{{m.tr("Kamu") + ": " + peerLabel(own, m.peers), own.PeerID}}
	peers := append([]discovery.PeerInfo(nil), m.peers...)
	sort.Slice(peers, func(i, j int) bool { return peers[i].PeerID < peers[j].PeerID })
	for _, peer := range peers {
		items = append(items, copyIDItem{peerLabel(peer, peers), peer.PeerID})
	}
	items = append(items, m.copyFileIDs()...)
	menu := &copyIDMenu{items: items, generation: m.copyGeneration}
	for index, item := range items {
		if item.value == m.activeTarget {
			menu.selected = index
		}
	}
	m.copyIDs = menu
}

func (m *AppModel) copyFileIDs() []copyIDItem {
	var items []copyIDItem
	seen := make(map[string]bool)
	for _, message := range m.messages {
		if message.FileRef != "" && !seen[message.FileRef] {
			items = append(items, copyIDItem{attachmentText(m.cfg.Language, message), message.FileRef})
			seen[message.FileRef] = true
		}
	}
	numbers := make([]int, 0, len(m.fileAnnounceMap))
	for number := range m.fileAnnounceMap {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	for _, number := range numbers {
		announce := m.fileAnnounceMap[number]
		if !seen[announce.FileID] {
			items = append(items, copyIDItem{fmt.Sprintf("File #%d: %s · #%s", number, announce.Filename, announce.Room), announce.FileID})
			seen[announce.FileID] = true
		}
	}
	return items
}

func (m *AppModel) handleCopyKey(key tea.KeyMsg) tea.Cmd {
	if key.Type == tea.KeyCtrlC {
		return tea.Quit
	}
	if key.Type == tea.KeyEsc {
		m.copyIDs = nil
		return nil
	}
	menu := m.copyIDs
	if menu.pending {
		return nil
	}
	switch key.Type {
	case tea.KeyUp:
		menu.selected = max(0, menu.selected-1)
	case tea.KeyDown:
		menu.selected = min(len(menu.items)-1, menu.selected+1)
	case tea.KeyPgUp:
		menu.selected = max(0, menu.selected-max(1, (m.height-6)/2))
	case tea.KeyPgDown:
		menu.selected = min(len(menu.items)-1, menu.selected+max(1, (m.height-6)/2))
	case tea.KeyEnter:
		menu.pending = true
		item, generation := menu.items[menu.selected], menu.generation
		return copyIDCommand(item, generation, clipboard.WriteAll)
	}
	return nil
}

func copyIDCommand(item copyIDItem, generation uint64, write func(string) error) tea.Cmd {
	return func() tea.Msg {
		return clipboardResult{generation: generation, label: item.label, err: write(item.value)}
	}
}

func (m *AppModel) applyClipboardResult(result clipboardResult) {
	if m.copyIDs == nil || m.copyIDs.generation != result.generation {
		return
	}
	m.copyIDs.pending = false
	if result.err != nil {
		m.copyIDs.status = m.tr("Clipboard tidak tersedia. Pilih teks ID lalu gunakan tombol salin terminal.") + "\n" + result.err.Error()
		return
	}
	m.copyIDs = nil
	m.statusText = m.tr("ID disalin: %s", strings.Split(result.label, "\n")[0])
	m.addSystemLog(m.statusText)
}

func (m *AppModel) renderCopyIDs() string {
	menu := m.copyIDs
	if m.width < 32 || m.height < 12 {
		return ansi.Truncate(m.tr("Perbesar terminal (minimal 32 × 12)."), max(1, m.width), "")
	}
	lines := []string{m.tr("Salin ID"), m.tr("↑/↓ Pilih · Enter Salin · Esc Kembali"), m.tr("Daftar tetap diam agar teks bisa dipilih dengan mouse.")}
	count := max(1, m.height-9)
	start := max(0, menu.selected-count+1)
	for index := start; index < min(len(menu.items), start+count); index++ {
		item := menu.items[index]
		mark := "  "
		if index == menu.selected {
			mark = "> "
		}
		label := strings.Split(item.label, "\n")[0]
		lines = append(lines, mark+label)
	}
	lines = append(lines, fmt.Sprintf("%d / %d", menu.selected+1, len(menu.items)))
	lines = append(lines, strings.Split(ansi.Wrap("ID: "+menu.items[menu.selected].value, m.width, ""), "\n")...)
	if menu.pending {
		lines = append(lines, m.tr("Menyalin..."))
	}
	if menu.status != "" {
		lines = append(lines, strings.Split(menu.status, "\n")...)
	}
	for i := range lines {
		lines[i] = ansi.Truncate(ansi.Strip(strings.ReplaceAll(lines[i], "\r", " ")), m.width, "…")
	}
	return strings.Join(lines[:min(len(lines), m.height)], "\n")
}
