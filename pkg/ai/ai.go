package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"kerykeion/pkg/protocol"
)

type ChatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Reasoning string `json:"reasoning,omitempty"`
}

type ModelInfo struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by,omitempty"`
}

type ModelListResponse struct {
	Data []ModelInfo `json:"data"`
}

type ChatRequest struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type ChatChoice struct {
	Index   int         `json:"index"`
	Message ChatMessage `json:"message"`
}

type ChatResponse struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
}

type HandoffCallback func(sync protocol.AISessionSyncPayload, count int)

type Service struct {
	mu             sync.RWMutex
	host           string
	apiKey         string
	model          string
	client         *http.Client
	histories      map[string][]ChatMessage // target -> history
	pendingQueries map[string]chan protocol.AIQueryResPayload
	onHandoff      HandoffCallback
}

func NewService(host, apiKey, defaultModel string) *Service {
	cleanHost := strings.TrimRight(host, "/")
	if cleanHost == "" {
		cleanHost = "http://10.10.5.141:8080"
	}
	cleanKey := strings.TrimSpace(apiKey)
	if cleanKey != "" && !strings.HasPrefix(strings.ToLower(cleanKey), "bearer ") {
		cleanKey = "Bearer " + cleanKey
	}
	if defaultModel == "" {
		defaultModel = "qwen3:14b"
	}

	return &Service{
		host:      cleanHost,
		apiKey:    cleanKey,
		model:          defaultModel,
		client:         &http.Client{Timeout: 90 * time.Second},
		histories:      make(map[string][]ChatMessage),
		pendingQueries: make(map[string]chan protocol.AIQueryResPayload),
	}
}

func (s *Service) Host() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.host
}

func (s *Service) Model() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}

func (s *Service) SetModel(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = strings.TrimSpace(name)
}

func (s *Service) SetConfig(host, apiKey, model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if host != "" {
		s.host = strings.TrimRight(host, "/")
	}
	if apiKey != "" {
		cleanKey := strings.TrimSpace(apiKey)
		if !strings.HasPrefix(strings.ToLower(cleanKey), "bearer ") {
			cleanKey = "Bearer " + cleanKey
		}
		s.apiKey = cleanKey
	}
	if model != "" {
		s.model = strings.TrimSpace(model)
	}
}

func (s *Service) ListModels() ([]string, error) {
	s.mu.RLock()
	url := fmt.Sprintf("%s/v1/models", s.host)
	key := s.apiKey
	s.mu.RUnlock()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat request list models: %w", err)
	}
	if key != "" {
		req.Header.Set("Authorization", key)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi server AI kantor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server mengembalikan status %d: %s", resp.StatusCode, string(b))
	}

	var listResp ModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("gagal membaca format JSON models: %w", err)
	}

	var names []string
	for _, m := range listResp.Data {
		names = append(names, m.ID)
	}
	return names, nil
}

func (s *Service) Ask(target, userPrompt string) (string, error) {
	s.mu.Lock()
	if target == "" {
		target = "global"
	}
	history := s.histories[target]

	userMsg := ChatMessage{
		Role:    "user",
		Content: userPrompt,
	}
	reqMsgs := append(history, userMsg)

	reqPayload := ChatRequest{
		Model:     s.model,
		Messages:  reqMsgs,
		MaxTokens: 2048,
	}

	url := fmt.Sprintf("%s/v1/chat/completions", s.host)
	key := s.apiKey
	activeModel := s.model
	s.mu.Unlock()

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("gagal serialize chat request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("gagal membuat request completion: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", key)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gagal menghubungi model AI: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("gagal membaca respons AI: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("galat AI status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", fmt.Errorf("gagal parse respons AI: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("respons AI kosong")
	}

	answer := chatResp.Choices[0].Message.Content
	if strings.TrimSpace(answer) == "" && chatResp.Choices[0].Message.Reasoning != "" {
		answer = chatResp.Choices[0].Message.Reasoning
	}

	// Update history
	s.mu.Lock()
	asstMsg := ChatMessage{
		Role:      "assistant",
		Content:   answer,
		Reasoning: chatResp.Choices[0].Message.Reasoning,
	}
	s.histories[target] = append(s.histories[target], userMsg, asstMsg)
	// Prune history jika terlalu panjang (jaga 30 turn terakhir)
	if len(s.histories[target]) > 30 {
		s.histories[target] = s.histories[target][len(s.histories[target])-30:]
	}
	s.mu.Unlock()

	return fmt.Sprintf("[%s]: %s", activeModel, strings.TrimSpace(answer)), nil
}

func (s *Service) ClearHistory(target string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if target == "" {
		target = "global"
	}
	delete(s.histories, target)
}

func (s *Service) ExportHistory(target string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if target == "" {
		target = "global"
	}
	hist := s.histories[target]
	return json.MarshalIndent(hist, "", "  ")
}

func (s *Service) ImportHistory(target string, data []byte) error {
	var imported []ChatMessage
	if err := json.Unmarshal(data, &imported); err != nil {
		return fmt.Errorf("format riwayat sesi AI tidak valid: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if target == "" {
		target = "global"
	}
	s.histories[target] = imported
	return nil
}

func (s *Service) HistoryCount(target string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if target == "" {
		target = "global"
	}
	return len(s.histories[target])
}

func (s *Service) SetHandoffCallback(fn HandoffCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onHandoff = fn
}

func (s *Service) GetGitDiff() string {
	cmd := exec.Command("git", "diff", "--stat", "-p")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	if len(out) > 64*1024 {
		return string(out[:64*1024]) + "\n... [diff terpotong karena melebihi 64KB]"
	}
	return string(out)
}

func (s *Service) CreateHandoffPayload(fromPeerID, targetConv string) (*protocol.AISessionSyncPayload, error) {
	data, err := s.ExportHistory(targetConv)
	if err != nil {
		return nil, err
	}
	if s.HistoryCount(targetConv) == 0 {
		return nil, fmt.Errorf("tidak ada riwayat sesi AI untuk percakapan ini")
	}

	diff := s.GetGitDiff()

	return &protocol.AISessionSyncPayload{
		V:           1,
		FromPeerID:  fromPeerID,
		TargetConv:  targetConv,
		Model:       s.Model(),
		HistoryJSON: string(data),
		GitDiff:     diff,
		TS:          time.Now().Unix(),
	}, nil
}

func (s *Service) RegisterPendingQuery(reqID string) chan protocol.AIQueryResPayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan protocol.AIQueryResPayload, 1)
	s.pendingQueries[reqID] = ch
	return ch
}

func (s *Service) UnregisterPendingQuery(reqID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pendingQueries, reqID)
}

func (s *Service) HandleIncomingQuery(payload []byte) ([]byte, error) {
	var req protocol.AIQueryReqPayload
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("gagal unmarshal AIQueryReq: %w", err)
	}

	activeModel := s.Model()
	if req.TargetModel != "" {
		activeModel = req.TargetModel
	}

	target := "remote:" + req.FromPeerID
	ans, err := s.Ask(target, req.Prompt)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	res := protocol.AIQueryResPayload{
		V:      1,
		ReqID:  req.ReqID,
		Model:  activeModel,
		Answer: ans,
		Error:  errMsg,
	}

	return json.Marshal(res)
}

func (s *Service) HandleIncomingRes(payload []byte) {
	var res protocol.AIQueryResPayload
	if err := json.Unmarshal(payload, &res); err != nil {
		return
	}

	s.mu.Lock()
	ch, ok := s.pendingQueries[res.ReqID]
	s.mu.Unlock()

	if ok && ch != nil {
		select {
		case ch <- res:
		default:
		}
	}
}

func (s *Service) HandleIncomingSync(payload []byte) (*protocol.AISessionSyncPayload, int, error) {
	var syncPayload protocol.AISessionSyncPayload
	if err := json.Unmarshal(payload, &syncPayload); err != nil {
		return nil, 0, fmt.Errorf("gagal unmarshal AISessionSync: %w", err)
	}

	if err := s.ImportHistory(syncPayload.TargetConv, []byte(syncPayload.HistoryJSON)); err != nil {
		return nil, 0, err
	}

	count := s.HistoryCount(syncPayload.TargetConv)

	s.mu.RLock()
	cb := s.onHandoff
	s.mu.RUnlock()

	if cb != nil {
		cb(syncPayload, count)
	}

	return &syncPayload, count, nil
}
