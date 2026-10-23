package ui

import (
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/transfer"
)

func (r *REPL) OnFileAnnounce(announce protocol.FileAnnouncePayload) {
	r.Printf("%s\n", localized(r.cfg.Language, "[Pengumuman File] %s. Ketik /get %d untuk unduh.", announce.Filename+" ("+formatBytes(announce.Size)+")", announce.AnnounceNo))
}

func (r *REPL) OnTransferProgress(progress transfer.TransferProgress) {
	if progress.ErrorMessage != "" {
		r.Printf("%s\n", localized(r.cfg.Language, "[Galat Transfer] %s: %s", progress.Filename, progress.ErrorMessage))
		return
	}
	if !progress.IsCompleted {
		return
	}
	if progress.IsDownload {
		r.Printf("%s\n", localized(r.cfg.Language, "[Transfer] %s disimpan di %s", progress.Filename, progress.LocalPath))
		return
	}
	r.Printf("[Transfer] %s %s\n", progress.Filename, localized(r.cfg.Language, "Terkirim"))
}
