package ui

import ()

func (r *REPL) commandRoomNew(parts []string) {
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
}

func (r *REPL) commandRoomList(parts []string) {
	rooms, err := r.store.GetAllRooms()
	if err != nil {
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
	var available []string
	joined := make(map[string]bool)
	for _, record := range rooms {
		if record.State == "active" {
			joined[record.Name] = true
		}
	}
	for _, record := range r.roomMgr.AvailableRooms() {
		if !joined[record.Name] {
			available = append(available, record.Name)
		}
	}
	r.Printf("%s\n", availableRoomList(r.cfg.Language, available))

}

func (r *REPL) commandRoomInfo(parts []string) {
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
}

func (r *REPL) commandRoomTransfer(parts []string) {
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
