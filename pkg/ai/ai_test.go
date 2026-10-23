package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kerykeion/pkg/protocol"
)

func TestAIServiceMock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(ModelListResponse{
				Data: []ModelInfo{
					{ID: "qwen3:8b"},
					{ID: "git-agent:latest"},
				},
			})
			return
		}
		if r.URL.Path == "/v1/chat/completions" {
			_ = json.NewEncoder(w).Encode(ChatResponse{
				ID:    "test-1",
				Model: "qwen3:8b",
				Choices: []ChatChoice{
					{
						Index: 0,
						Message: ChatMessage{
							Role:    "assistant",
							Content: "Jawaban simulasi AI",
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	svc := NewService(ts.URL, "Bearer testkey", "qwen3:8b")

	models, err := svc.ListModels()
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) != 2 || models[0] != "qwen3:8b" {
		t.Fatalf("Unexpected models: %v", models)
	}

	ans, err := svc.Ask("test-room", "Halo")
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	if ans != "[qwen3:8b]: Jawaban simulasi AI" {
		t.Fatalf("Unexpected answer: %s", ans)
	}

	if svc.HistoryCount("test-room") != 2 {
		t.Fatalf("Expected history count 2, got %d", svc.HistoryCount("test-room"))
	}

	// Test Export & Import
	data, err := svc.ExportHistory("test-room")
	if err != nil {
		t.Fatalf("ExportHistory failed: %v", err)
	}

	svc.ClearHistory("test-room")
	if svc.HistoryCount("test-room") != 0 {
		t.Fatalf("Expected history count 0 after clear")
	}

	if err := svc.ImportHistory("test-room", data); err != nil {
		t.Fatalf("ImportHistory failed: %v", err)
	}
	if svc.HistoryCount("test-room") != 2 {
		t.Fatalf("Expected history count 2 after import")
	}

	// Test Handoff Payload creation
	handoff, err := svc.CreateHandoffPayload("peer-alice", "test-room")
	if err != nil {
		t.Fatalf("CreateHandoffPayload failed: %v", err)
	}
	if handoff.TargetConv != "test-room" || handoff.FromPeerID != "peer-alice" {
		t.Fatalf("Unexpected handoff payload: %+v", handoff)
	}

	// Test Incoming Sync
	handoffBytes, _ := json.Marshal(handoff)
	receivedSync, count, err := svc.HandleIncomingSync(handoffBytes)
	if err != nil {
		t.Fatalf("HandleIncomingSync failed: %v", err)
	}
	if count != 2 || receivedSync.FromPeerID != "peer-alice" {
		t.Fatalf("Unexpected sync result: count=%d, sync=%+v", count, receivedSync)
	}

	// Test Incoming Query & Res
	qReq := protocol.AIQueryReqPayload{
		V:          1,
		ReqID:      "q-123",
		FromPeerID: "peer-bob",
		Prompt:     "Test query",
	}
	qBytes, _ := json.Marshal(qReq)
	resBytes, err := svc.HandleIncomingQuery(qBytes)
	if err != nil {
		t.Fatalf("HandleIncomingQuery failed: %v", err)
	}

	ch := svc.RegisterPendingQuery("q-123")
	svc.HandleIncomingRes(resBytes)
	select {
	case res := <-ch:
		if res.ReqID != "q-123" || res.Answer == "" {
			t.Fatalf("Unexpected res from pending query: %+v", res)
		}
	default:
		t.Fatalf("Expected response in channel, got none")
	}
}
