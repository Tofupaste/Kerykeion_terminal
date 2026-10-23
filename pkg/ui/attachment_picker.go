package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type attachmentPicker struct {
	directory, target string
	isRoom            bool
	entries           []os.DirEntry
	selected          int
	loading           bool
	err               error
	file              string
	size              int64
}

type attachmentDirectoryResult struct {
	generation uint64
	directory  string
	entries    []os.DirEntry
	file       string
	size       int64
	err        error
}

func (m *AppModel) openAttachmentPicker() tea.Cmd {
	if !m.chatVisible || m.systemFocused || m.activeTarget == "" {
		m.addSystemLog(m.tr("Fokuskan chat untuk melampirkan file."))
		return nil
	}
	directory := m.pickerDirectory
	if directory == "" {
		var err error
		directory, err = os.UserHomeDir()
		if err != nil {
			m.addSystemLog(err.Error())
			return nil
		}
	}
	m.picker = &attachmentPicker{target: m.activeTarget, isRoom: m.isRoom}
	return m.loadAttachmentPath(directory)
}

func (m *AppModel) loadAttachmentPath(path string) tea.Cmd {
	m.pickerGeneration++
	generation, limit := m.pickerGeneration, m.cfg.MaxFileSize
	m.picker.loading = true
	m.picker.err = nil
	return func() tea.Msg {
		result := attachmentDirectoryResult{generation: generation, directory: path}
		info, err := os.Stat(path)
		if err != nil {
			result.err = err
			return result
		}
		if !info.IsDir() {
			info, result.err = attachmentInfo(path, limit)
			if result.err == nil {
				result.file, result.size = path, info.Size()
			}
			return result
		}
		result.entries, result.err = os.ReadDir(path)
		sort.SliceStable(result.entries, func(i, j int) bool {
			a, b := result.entries[i], result.entries[j]
			if a.IsDir() != b.IsDir() {
				return a.IsDir()
			}
			return strings.ToLower(a.Name()) < strings.ToLower(b.Name())
		})
		return result
	}
}

func (m *AppModel) applyAttachmentDirectory(result attachmentDirectoryResult) {
	if m.picker == nil || result.generation != m.pickerGeneration {
		return
	}
	p := m.picker
	p.loading, p.err = false, result.err
	if result.err != nil {
		return
	}
	if result.file != "" {
		p.file, p.size = result.file, result.size
		return
	}
	p.directory, p.entries, p.selected = result.directory, result.entries, 0
	m.pickerDirectory = result.directory
}

func (m *AppModel) handleAttachmentKey(key tea.KeyMsg) tea.Cmd {
	if key.Type == tea.KeyCtrlC {
		return tea.Quit
	}
	if key.Type == tea.KeyEsc {
		m.picker = nil
		return nil
	}
	p := m.picker
	if p.loading {
		return nil
	}
	if p.file != "" {
		return m.confirmAttachment(key)
	}
	switch key.Type {
	case tea.KeyUp:
		p.selected = max(0, p.selected-1)
	case tea.KeyDown:
		p.selected = min(max(0, len(p.entries)-1), p.selected+1)
	case tea.KeyPgUp:
		p.selected = max(0, p.selected-max(1, m.height-8))
	case tea.KeyPgDown:
		p.selected = min(max(0, len(p.entries)-1), p.selected+max(1, m.height-8))
	case tea.KeyBackspace, tea.KeyLeft:
		return m.loadAttachmentPath(filepath.Dir(p.directory))
	case tea.KeyEnter, tea.KeyRight:
		if len(p.entries) > 0 {
			return m.loadAttachmentPath(filepath.Join(p.directory, p.entries[p.selected].Name()))
		}
	}
	return nil
}

func (m *AppModel) confirmAttachment(key tea.KeyMsg) tea.Cmd {
	p := m.picker
	if key.Type == tea.KeyBackspace {
		p.file = ""
		return nil
	}
	if key.Type != tea.KeyEnter {
		return nil
	}
	m.picker = nil
	if p.target != m.activeTarget || p.isRoom != m.isRoom {
		return nil
	}
	return m.sendAttachment(p.file, p.target, p.isRoom)
}

func (m *AppModel) sendAttachment(path, target string, isRoom bool) tea.Cmd {
	manager, limit := m.transfer, m.cfg.MaxFileSize
	return func() tea.Msg {
		resolved, err := resolveAttachmentPath(path)
		if err == nil {
			_, err = attachmentInfo(resolved, limit)
		}
		if err == nil {
			if isRoom {
				_, err = manager.SendFile(resolved, "", target)
			} else {
				_, err = manager.SendFile(resolved, target, "")
			}
		}
		return MsgCommandResult{Err: err}
	}
}

func (m *AppModel) renderAttachmentPicker() string {
	if m.width < 32 || m.height < 12 {
		return ansi.Truncate(m.tr("Perbesar terminal (minimal 32 × 12)."), max(1, m.width), "")
	}
	p := m.picker
	lines := []string{m.tr("Lampirkan file"), p.directory, m.tr("Tujuan: %s", p.target)}
	if p.file != "" {
		lines = append(lines, p.file, formatBytes(p.size), m.tr("Enter Kirim · Backspace Kembali · Esc Batal"))
	} else {
		lines = append(lines, m.tr("↑/↓ Pilih · Enter Buka · Backspace Induk · Esc Batal"))
		lines = append(lines, m.attachmentRows()...)
	}
	if p.loading {
		lines = append(lines, m.tr("Membaca folder..."))
	}
	if p.err != nil {
		lines = append(lines, fmt.Sprintf("[Error] %v", p.err))
	}
	for i := range lines {
		lines[i] = ansi.Truncate(strings.NewReplacer("\n", " ", "\r", " ", "\x1b", "?").Replace(lines[i]), max(1, m.width), "…")
	}
	return strings.Join(lines[:min(len(lines), max(1, m.height))], "\n")
}

func (m *AppModel) attachmentRows() []string {
	p := m.picker
	height := max(1, m.height-7)
	start := max(0, p.selected-height+1)
	rows := []string{}
	for i := start; i < min(len(p.entries), start+height); i++ {
		entry := p.entries[i]
		prefix, suffix := "  ", ""
		if i == p.selected {
			prefix = "> "
		}
		if entry.IsDir() {
			suffix = string(os.PathSeparator)
		}
		rows = append(rows, prefix+entry.Name()+suffix)
	}
	if len(rows) == 0 {
		rows = append(rows, m.tr("Folder kosong."))
	}
	return append(rows, fmt.Sprintf("%d / %d", min(p.selected+1, len(p.entries)), len(p.entries)))
}
