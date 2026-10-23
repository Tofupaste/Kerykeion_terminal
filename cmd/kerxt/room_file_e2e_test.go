package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kerykeion/pkg/protocol"
)

func sharedRoom(t *testing.T, peers ...*roomTestPeer) {
	t.Helper()
	connectRoomTestPeers(t, peers...)
	if _, err := peers[0].rooms.CreateRoom("room-e2e"); err != nil {
		t.Fatal(err)
	}
	for _, peer := range peers[1:] {
		if _, found, err := peer.rooms.JoinRoom("room-e2e", peer.transport.Port()); err != nil || !found {
			t.Fatalf("room join: %v", err)
		}
	}
	for _, peer := range peers {
		awaitRoomMembers(t, peer, len(peers))
	}
}

func roomAnnouncement(t *testing.T, peer *roomTestPeer) protocol.FileAnnouncePayload {
	t.Helper()
	select {
	case announcement := <-peer.announcements:
		return announcement
	case <-time.After(5 * time.Second):
		t.Fatal("file announcement did not reach room member")
		return protocol.FileAnnouncePayload{}
	}
}

func pullRoomFile(t *testing.T, peer *roomTestPeer, announcement protocol.FileAnnouncePayload) string {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- peer.transfers.PullAnnouncedFile(announcement.AnnounceNo) }()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("/get did not finish downloading")
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case progress := <-peer.progress:
			if progress.FileID == announcement.FileID && progress.IsDownload && progress.IsCompleted {
				return progress.LocalPath
			}
		case <-timer.C:
			t.Fatal("download finished without a completion path")
			return ""
		}
	}

}

func TestE2ERoomGetDownloadsFile(t *testing.T) {
	for _, size := range []int{0, 2*protocol.ChunkSize + 137} {
		t.Run(stringSize(size), func(t *testing.T) { checkRoomFileDownload(t, size) })
	}
}

func checkRoomFileDownload(t *testing.T, size int) {
	t.Helper()
	alice, bob := newRoomTestPeer(t, "Alice"), newRoomTestPeer(t, "Bob")
	sharedRoom(t, alice, bob)
	path := filepath.Join(t.TempDir(), "project notes (1).txt")
	content := bytes.Repeat([]byte{'x'}, size)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.transfers.SendFile(path, "", "room-e2e"); err != nil {
		t.Fatal(err)
	}
	announce := roomAnnouncement(t, bob)
	downloaded := pullRoomFile(t, bob, announce)
	got, err := os.ReadFile(downloaded)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("wrong downloaded content: %v", err)
	}
	if filepath.Base(downloaded) != filepath.Base(path) {
		t.Fatal("download filename changed")
	}
	reply, err := bob.messages.SendRoomMessage("room-e2e", "Chat after /get", "")
	if err != nil {
		t.Fatal(err)
	}
	awaitRoomMessage(t, alice, reply.MsgID)
}

func stringSize(size int) string {
	if size == 0 {
		return "empty"
	}
	return "multiple-chunks"
}

func TestE2ERoomGetUsesAnotherMembersCache(t *testing.T) {
	alice, bob, carol := newRoomTestPeer(t, "Alice"), newRoomTestPeer(t, "Bob"), newRoomTestPeer(t, "Carol")
	sharedRoom(t, alice, bob, carol)
	source := filepath.Join(t.TempDir(), "cached report.txt")
	content := []byte("A report shared with the whole room")
	if err := os.WriteFile(source, content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.transfers.SendFile(source, "", "room-e2e"); err != nil {
		t.Fatal(err)
	}
	carolAnnouncement := roomAnnouncement(t, carol)
	pullRoomFile(t, carol, carolAnnouncement)
	cache, err := alice.store.GetMediaCache(carolAnnouncement.FileID)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(cache.Path); err != nil {
		t.Fatal(err)
	}
	bobAnnouncement := roomAnnouncement(t, bob)
	downloaded := pullRoomFile(t, bob, bobAnnouncement)
	actual, err := os.ReadFile(downloaded)
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatalf("cache fallback content: %v", err)
	}
}

func TestE2ERoomGetAnnouncementsFromMultipleSenders(t *testing.T) {
	alice, bob, carol := newRoomTestPeer(t, "Alice"), newRoomTestPeer(t, "Bob"), newRoomTestPeer(t, "Carol")
	sharedRoom(t, alice, bob, carol)
	for _, peer := range []*roomTestPeer{alice, bob} {
		path := filepath.Join(t.TempDir(), "report.txt")
		if err := os.WriteFile(path, []byte(peer.id.DisplayName()), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := peer.transfers.SendFile(path, "", "room-e2e"); err != nil {
			t.Fatal(err)
		}
	}
	first, second := roomAnnouncement(t, carol), roomAnnouncement(t, carol)
	if first.AnnounceNo == second.AnnounceNo {
		t.Fatal("sender announcement numbers collided")
	}
	paths := []string{pullRoomFile(t, carol, first), pullRoomFile(t, carol, second)}
	if paths[0] == paths[1] {
		t.Fatal("second download overwrote first filename")
	}
	for i, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sender := []protocol.FileAnnouncePayload{first, second}[i].SenderID
		expected := "Alice"
		if sender == bob.id.PeerID() {
			expected = "Bob"
		}
		if string(content) != expected {
			t.Fatalf("wrong file for local announcement number: %s", content)
		}
	}
}

func TestE2ERoomGetReportsUnavailableFile(t *testing.T) {
	alice, bob := newRoomTestPeer(t, "Alice"), newRoomTestPeer(t, "Bob")
	sharedRoom(t, alice, bob)
	source := filepath.Join(t.TempDir(), "missing.txt")
	if err := os.WriteFile(source, []byte("temporary file"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.transfers.SendFile(source, "", "room-e2e"); err != nil {
		t.Fatal(err)
	}
	announce := roomAnnouncement(t, bob)
	cached, err := alice.store.GetMediaCache(announce.FileID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cached.Path); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- bob.transfers.PullAnnouncedFile(announce.AnnounceNo) }()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("missing file reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("missing file request did not report failure")
	}
	if _, err := os.Stat(filepath.Join(bob.config.DownloadsDir, announce.Filename)); !os.IsNotExist(err) {
		t.Fatal("missing file appeared as a completed download")
	}
}

func TestE2ERoomGetResumesPartialFile(t *testing.T) {
	alice, bob := newRoomTestPeer(t, "Alice"), newRoomTestPeer(t, "Bob")
	sharedRoom(t, alice, bob)
	source := filepath.Join(t.TempDir(), "resume.txt")
	content := bytes.Repeat([]byte("resume"), 100000)
	if err := os.WriteFile(source, content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.transfers.SendFile(source, "", "room-e2e"); err != nil {
		t.Fatal(err)
	}
	announce := roomAnnouncement(t, bob)
	partial := make([]byte, len(content))
	copy(partial, content[:protocol.ChunkSize])
	if err := os.WriteFile(filepath.Join(bob.config.PartialDir, announce.FileID+".part"), partial, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bob.config.PartialDir, announce.FileID+".meta"), []byte{6}, 0600); err != nil {
		t.Fatal(err)
	}
	downloaded := pullRoomFile(t, bob, announce)
	actual, err := os.ReadFile(downloaded)
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatalf("resumed file mismatch: %v", err)
	}
}

func TestE2ERoomGetReportsSaveFailure(t *testing.T) {
	alice, bob := newRoomTestPeer(t, "Alice"), newRoomTestPeer(t, "Bob")
	sharedRoom(t, alice, bob)
	source := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(source, []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.transfers.SendFile(source, "", "room-e2e"); err != nil {
		t.Fatal(err)
	}
	announce := roomAnnouncement(t, bob)
	blocked := filepath.Join(t.TempDir(), "not-a-folder")
	if err := os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	bob.config.DownloadsDir = blocked
	if err := bob.transfers.PullAnnouncedFile(announce.AnnounceNo); err == nil {
		t.Fatal("failed save reported success")
	}
	record, err := bob.store.GetTransfer(announce.FileID, alice.id.PeerID(), "download")
	if err != nil || record == nil || record.State == "completed" {
		t.Fatal("failed save marked completed")
	}
}
