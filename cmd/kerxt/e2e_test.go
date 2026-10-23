package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
	"kerykeion/pkg/transport"
)

func TestE2EMessagingAndFileTransfer(t *testing.T) {
	tempDirAlice, err := os.MkdirTemp("", "kerxt-e2e-alice-*")
	if err != nil {
		t.Fatalf("failed to create temp dir alice: %v", err)
	}
	defer os.RemoveAll(tempDirAlice)

	tempDirBob, err := os.MkdirTemp("", "kerxt-e2e-bob-*")
	if err != nil {
		t.Fatalf("failed to create temp dir bob: %v", err)
	}
	defer os.RemoveAll(tempDirBob)

	cfgAlice, _ := config.LoadConfig(tempDirAlice)
	cfgAlice.TCPPort = 43100
	cfgAlice.UDPPort = 43101
	cfgAlice.DisplayName = "Alice"

	cfgBob, _ := config.LoadConfig(tempDirBob)
	cfgBob.TCPPort = 43200
	cfgBob.UDPPort = 43201
	cfgBob.DisplayName = "Bob"

	idAlice, _ := identity.LoadOrCreate(cfgAlice.ConfigDir, cfgAlice.DisplayName)
	idBob, _ := identity.LoadOrCreate(cfgBob.ConfigDir, cfgBob.DisplayName)

	dbAlice, _ := db.Open(cfgAlice.ConfigDir)
	defer dbAlice.Close()
	dbBob, _ := db.Open(cfgBob.ConfigDir)
	defer dbBob.Close()

	transAlice, err := transport.New(idAlice, cfgAlice.TCPPort)
	if err != nil {
		t.Fatalf("Alice transport init failed: %v", err)
	}
	if err := transAlice.Start(); err != nil {
		t.Fatalf("Alice transport start failed: %v", err)
	}
	defer transAlice.Stop()

	transBob, err := transport.New(idBob, cfgBob.TCPPort)
	if err != nil {
		t.Fatalf("Bob transport init failed: %v", err)
	}
	if err := transBob.Start(); err != nil {
		t.Fatalf("Bob transport start failed: %v", err)
	}
	defer transBob.Stop()

	discAlice := discovery.New(idAlice, cfgAlice.UDPPort, "")
	discBob := discovery.New(idBob, cfgBob.UDPPort, "")

	// Register each other directly in peer discovery cache
	_ = dbAlice.UpsertPeer(db.PeerRecord{
		PeerID:      idBob.PeerID(),
		DisplayName: idBob.DisplayName(),
		PubKey:      idBob.PublicKeyBase64(),
		LastIP:      "127.0.0.1",
		LastPort:    transBob.Port(),
		LastSeen:    time.Now(),
		Trusted:     true,
	})
	_ = dbBob.UpsertPeer(db.PeerRecord{
		PeerID:      idAlice.PeerID(),
		DisplayName: idAlice.DisplayName(),
		PubKey:      idAlice.PublicKeyBase64(),
		LastIP:      "127.0.0.1",
		LastPort:    transAlice.Port(),
		LastSeen:    time.Now(),
		Trusted:     true,
	})

	bobReceivedMsg := make(chan db.MessageRecord, 5)
	aliceDeliveredStatus := make(chan string, 5)

	msgAlice := messaging.New(idAlice, dbAlice, transAlice, discAlice,
		nil,
		func(msgID, status string) {
			if status == "delivered" {
				aliceDeliveredStatus <- msgID
			}
		},
	)
	msgAlice.Start()
	defer msgAlice.Stop()

	msgBob := messaging.New(idBob, dbBob, transBob, discBob,
		func(msg db.MessageRecord, isSelf bool) {
			if !isSelf {
				bobReceivedMsg <- msg
			}
		},
		nil,
	)
	msgBob.Start()
	defer msgBob.Stop()

	rMgrAlice := room.New(idAlice, dbAlice, discAlice, transAlice, nil)
	rMgrBob := room.New(idBob, dbBob, discBob, transBob, nil)

	tferAlice := transfer.New(cfgAlice, idAlice, dbAlice, discAlice, transAlice, nil, nil)
	tferAlice.SetMessaging(msgAlice)
	tferAlice.Start()
	defer tferAlice.Stop()

	bobDoneCh := make(chan string, 1)
	tferBob := transfer.New(cfgBob, idBob, dbBob, discBob, transBob,
		downloadCompletion(bobDoneCh),
		nil,
	)
	tferBob.SetMessaging(msgBob)
	tferBob.Start()
	defer tferBob.Stop()

	// Dispatchers
	transAlice.SetFrameHandler(func(connID string, frameType byte, payload []byte, remotePeerID string) {
		switch frameType {
		case protocol.TypeMsg:
			msgAlice.HandleIncomingMsg(payload, remotePeerID)
		case protocol.TypeMsgAck:
			msgAlice.HandleIncomingMsgAck(payload, remotePeerID)
		case protocol.TypeFileAccept:
			tferAlice.HandleFileAccept(connID, payload)
		case protocol.TypeFileDone:
			tferAlice.HandleFileDone(connID, payload)
		}
	})

	transBob.SetFrameHandler(func(connID string, frameType byte, payload []byte, remotePeerID string) {
		switch frameType {
		case protocol.TypeMsg:
			msgBob.HandleIncomingMsg(payload, remotePeerID)
		case protocol.TypeMsgAck:
			msgBob.HandleIncomingMsgAck(payload, remotePeerID)
		case protocol.TypeFileOffer:
			tferBob.HandleFileOffer(connID, payload, remotePeerID)
		case protocol.TypeChunk:
			tferBob.HandleChunk(connID, payload, remotePeerID)
		}
	})

	// Pre-establish TLS connection Alice -> Bob
	connAliceToBob, err := transAlice.DialPeer(idBob.PeerID(), "127.0.0.1", transBob.Port(), "chat")
	if err != nil {
		t.Fatalf("failed to dial Alice -> Bob: %v", err)
	}
	defer connAliceToBob.Close()
	time.Sleep(100 * time.Millisecond)

	_ = rMgrBob
	checks := e2eChecks{
		aliceID: idAlice, bobID: idBob, messages: msgAlice, rooms: rMgrAlice, transfers: tferAlice,
		bobMessages: bobReceivedMsg, delivered: aliceDeliveredStatus, downloaded: bobDoneCh,
		aliceDir: tempDirAlice, bobConfig: cfgBob,
	}
	checkDirectMessage(t, checks)
	checkRoomCreation(t, checks)
	checkFileTransfer(t, checks)
}

func downloadCompletion(completed chan<- string) transfer.ProgressCallback {
	return func(progress transfer.TransferProgress) {
		if progress.IsCompleted && progress.IsDownload {
			completed <- progress.FileID
		}
	}
}

type e2eChecks struct {
	aliceID, bobID *identity.Manager
	messages       *messaging.Service
	rooms          *room.Manager
	transfers      *transfer.Manager
	bobMessages    <-chan db.MessageRecord
	delivered      <-chan string
	downloaded     <-chan string
	aliceDir       string
	bobConfig      *config.Config
}

func checkDirectMessage(t *testing.T, checks e2eChecks) {
	t.Helper()
	idAlice, idBob, msgAlice := checks.aliceID, checks.bobID, checks.messages
	bobReceivedMsg, aliceDeliveredStatus := checks.bobMessages, checks.delivered
	// 1. Test DM send Alice -> Bob
	dmText := "Halo Bob dari Alice via Kerykeion Terminal P2P!"
	msgRec, err := msgAlice.SendDM(idBob.PeerID(), dmText)
	if err != nil {
		t.Fatalf("Alice failed to send DM: %v", err)
	}

	select {
	case received := <-bobReceivedMsg:
		if received.Body != dmText {
			t.Errorf("Bob received wrong body: expected %s, got %s", dmText, received.Body)
		}
		if received.AuthorID != idAlice.PeerID() {
			t.Errorf("Bob received wrong author: expected %s, got %s", idAlice.PeerID(), received.AuthorID)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("Bob timed out waiting for incoming DM")
	}

	select {
	case deliveredID := <-aliceDeliveredStatus:
		if deliveredID != msgRec.MsgID {
			t.Errorf("Alice expected delivered status for msg %s, got %s", msgRec.MsgID, deliveredID)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("Alice timed out waiting for delivered ACK")
	}

}

func checkRoomCreation(t *testing.T, checks e2eChecks) {
	t.Helper()
	rMgrAlice, idAlice := checks.rooms, checks.aliceID
	// 2. Test Room Creation & Management
	roomRec, err := rMgrAlice.CreateRoom("ngobs-e2e")
	if err != nil {
		t.Fatalf("Alice failed to create room: %v", err)
	}
	if roomRec.MasterID != idAlice.PeerID() {
		t.Errorf("Alice should be master of created room")
	}

}

func checkFileTransfer(t *testing.T, checks e2eChecks) {
	t.Helper()
	tempDirAlice, tferAlice, idBob := checks.aliceDir, checks.transfers, checks.bobID
	bobDoneCh, cfgBob := checks.downloaded, checks.bobConfig
	// 3. Test File Transfer Alice -> Bob
	testFile := filepath.Join(tempDirAlice, "testdoc.txt")
	testContent := []byte("Isi berkas penting rahasia kantor P2P transfer test.")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	offer, err := tferAlice.SendFile(testFile, idBob.PeerID(), "")
	if err != nil {
		t.Fatalf("Alice failed to send file: %v", err)
	}

	select {
	case message := <-checks.bobMessages:
		if message.FileRef != offer.FileID {
			t.Fatal("DM attachment missing file ID")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("DM attachment did not reach conversation history")
	}

	select {
	case doneFileID := <-bobDoneCh:
		if doneFileID != offer.FileID {
			t.Errorf("Bob downloaded fileID mismatch: expected %s, got %s", offer.FileID, doneFileID)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Bob timed out waiting for file transfer completion")
	}

	// Verify Bob's downloaded file content
	downloadedFile := filepath.Join(cfgBob.DownloadsDir, "testdoc.txt")
	downloadedData, err := os.ReadFile(downloadedFile)
	if err != nil {
		t.Fatalf("failed to read Bob's downloaded file: %v", err)
	}
	if string(downloadedData) != string(testContent) {
		t.Errorf("downloaded content mismatch: expected %q, got %q", string(testContent), string(downloadedData))
	}
}
