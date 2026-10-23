package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

var AntigravityModels = []string{
	"claude-sonnet-4-6",
	"claude-opus-4-6-thinking",
	"gemini-3.8-flash-high",
	"gemini-3.8-flash-medium",
	"gemini-3.8-flash-low",
	"gpt-oss-120b-medium",
}

type Service struct {
	mu             sync.RWMutex
	host           string
	apiKey         string
	model          string
	client         *http.Client
	histories      map[string][]ChatMessage // target -> history
	pendingQueries map[string]chan protocol.AIQueryResPayload
	onHandoff      HandoffCallback
	agyPath        string
	claudePath     string
	codexPath      string
}

func FindAgyBinary() string {
	if path, err := exec.LookPath("agy"); err == nil {
		return path
	}
	home, err := os.UserHomeDir()
	if err == nil {
		localBin := filepath.Join(home, ".local", "bin", "agy")
		if _, err := os.Stat(localBin); err == nil {
			return localBin
		}
		geminiBin := filepath.Join(home, ".gemini", "bin", "agy")
		if _, err := os.Stat(geminiBin); err == nil {
			return geminiBin
		}
	}
	return ""
}

func FindClaudeBinary() string {
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}
	home, err := os.UserHomeDir()
	if err == nil {
		localBin := filepath.Join(home, ".local", "bin", "claude")
		if _, err := os.Stat(localBin); err == nil {
			return localBin
		}
	}
	return ""
}

func FindCodexBinary() string {
	if path, err := exec.LookPath("codex"); err == nil {
		return path
	}
	return ""
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
		host:           cleanHost,
		apiKey:         cleanKey,
		model:          defaultModel,
		client:         &http.Client{Timeout: 90 * time.Second},
		histories:      make(map[string][]ChatMessage),
		pendingQueries: make(map[string]chan protocol.AIQueryResPayload),
		agyPath:        FindAgyBinary(),
		claudePath:     FindClaudeBinary(),
		codexPath:      FindCodexBinary(),
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

func (s *Service) isAgyModel(modelName string) bool {
	clean := strings.ToLower(strings.TrimSpace(modelName))
	clean = strings.TrimPrefix(clean, "agy:")
	for _, m := range AntigravityModels {
		if strings.EqualFold(m, clean) {
			return true
		}
	}
	return strings.HasPrefix(clean, "gemini-") || strings.HasPrefix(clean, "claude-sonnet-4") || strings.HasPrefix(clean, "claude-opus-4")
}

func (s *Service) isClaudeCodeModel(modelName string) bool {
	clean := strings.ToLower(strings.TrimSpace(modelName))
	return clean == "claude" || clean == "claude-code" || strings.HasPrefix(clean, "claude:")
}

func (s *Service) isCodexModel(modelName string) bool {
	clean := strings.ToLower(strings.TrimSpace(modelName))
	return clean == "codex" || strings.HasPrefix(clean, "codex:")
}

func (s *Service) ListModels() ([]string, error) {
	var results []string
	seen := make(map[string]bool)

	// 1. Ambil model dari server kantor (HTTP)
	s.mu.RLock()
	url := fmt.Sprintf("%s/v1/models", s.host)
	key := s.apiKey
	s.mu.RUnlock()

	req, err := http.NewRequest("GET", url, nil)
	if err == nil {
		if key != "" {
			req.Header.Set("Authorization", key)
		}
		resp, err := s.client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var listResp ModelListResponse
			if err := json.NewDecoder(resp.Body).Decode(&listResp); err == nil {
				for _, m := range listResp.Data {
					if !seen[m.ID] {
						results = append(results, m.ID)
						seen[m.ID] = true
					}
				}
			}
		}
	}

	// 2. Tambahkan model Antigravity CLI jika binary agy tersedia
	if s.agyPath != "" {
		for _, m := range AntigravityModels {
			if !seen[m] {
				results = append(results, m)
				seen[m] = true
			}
		}
	}

	// 3. Tambahkan Claude Code CLI jika tersedia
	if s.claudePath != "" && !seen["claude-code"] {
		results = append(results, "claude-code")
		seen["claude-code"] = true
	}

	// 4. Tambahkan Codex jika binary codex tersedia
	if s.codexPath != "" && !seen["codex"] {
		results = append(results, "codex")
		seen["codex"] = true
	}

	if len(results) == 0 {
		return []string{"qwen3:14b"}, nil
	}

	return results, nil
}

func (s *Service) Ask(target, userPrompt string) (string, error) {
	s.mu.RLock()
	activeModel := s.model
	s.mu.RUnlock()

	return s.AskModel(target, userPrompt, activeModel)
}

func (s *Service) AskModel(target, userPrompt, chosenModel string) (string, error) {
	if chosenModel == "" {
		s.mu.RLock()
		chosenModel = s.model
		s.mu.RUnlock()
	}

	if target == "" {
		target = "global"
	}

	s.mu.RLock()
	history := s.histories[target]
	s.mu.RUnlock()

	// 1. Eksekusi via Antigravity CLI jika model tergolong agy
	if s.isAgyModel(chosenModel) && s.agyPath != "" {
		cleanModel := strings.TrimPrefix(chosenModel, "agy:")
		ans, err := s.askCLI(s.agyPath, cleanModel, userPrompt, history, true)
		if err == nil {
			s.recordHistory(target, userPrompt, ans, "")
			return fmt.Sprintf("[%s]: %s", chosenModel, strings.TrimSpace(ans)), nil
		}
	}

	// 2. Eksekusi via Claude Code CLI jika diminta
	if s.isClaudeCodeModel(chosenModel) && s.claudePath != "" {
		ans, err := s.askCLI(s.claudePath, "", userPrompt, history, false)
		if err == nil {
			s.recordHistory(target, userPrompt, ans, "")
			return fmt.Sprintf("[%s]: %s", chosenModel, strings.TrimSpace(ans)), nil
		}
	}

	// 3. Eksekusi via Codex CLI jika diminta
	if s.isCodexModel(chosenModel) && s.codexPath != "" {
		ans, err := s.askCodexCLI(s.codexPath, userPrompt, history)
		if err == nil {
			s.recordHistory(target, userPrompt, ans, "")
			return fmt.Sprintf("[%s]: %s", chosenModel, strings.TrimSpace(ans)), nil
		}
	}

	// 4. Eksekusi default via HTTP server kantor (OpenAI-compatible)
	return s.askHTTP(target, userPrompt, chosenModel, history)
}

func (s *Service) askCLI(binPath, modelFlag, prompt string, history []ChatMessage, isAgy bool) (string, error) {
	var fullPrompt string
	if len(history) > 0 {
		var sb strings.Builder
		sb.WriteString("Konteks percakapan sebelumnya:\n")
		start := 0
		if len(history) > 6 {
			start = len(history) - 6
		}
		for _, m := range history[start:] {
			sb.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
		}
		sb.WriteString(fmt.Sprintf("\nPertanyaan baru: %s", prompt))
		fullPrompt = sb.String()
	} else {
		fullPrompt = prompt
	}

	var args []string
	if isAgy {
		args = []string{"-p", fullPrompt, "--model", modelFlag, "--disable-slash-commands"}
	} else {
		args = []string{"-p", fullPrompt}
	}

	cmd := exec.Command(binPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("eksekusi CLI %s gagal: %w", filepath.Base(binPath), err)
	}

	cleanOut := strings.TrimSpace(string(out))
	if cleanOut == "" {
		return "", fmt.Errorf("CLI %s menghasilkan respons kosong", filepath.Base(binPath))
	}
	return cleanOut, nil
}

func (s *Service) askCodexCLI(binPath, prompt string, history []ChatMessage) (string, error) {
	args := []string{"exec", prompt}
	cmd := exec.Command(binPath, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("eksekusi codex gagal: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *Service) askHTTP(target, userPrompt, modelName string, history []ChatMessage) (string, error) {
	userMsg := ChatMessage{
		Role:    "user",
		Content: userPrompt,
	}
	reqMsgs := append(history, userMsg)

	reqPayload := ChatRequest{
		Model:     modelName,
		Messages:  reqMsgs,
		MaxTokens: 2048,
	}

	s.mu.RLock()
	url := fmt.Sprintf("%s/v1/chat/completions", s.host)
	key := s.apiKey
	s.mu.RUnlock()

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
		return "", fmt.Errorf("gagal menghubungi model AI server: %w", err)
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
	reasoning := chatResp.Choices[0].Message.Reasoning
	if strings.TrimSpace(answer) == "" && reasoning != "" {
		answer = reasoning
	}

	s.recordHistory(target, userPrompt, answer, reasoning)
	return fmt.Sprintf("[%s]: %s", modelName, strings.TrimSpace(answer)), nil
}

func (s *Service) recordHistory(target, userPrompt, answer, reasoning string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userMsg := ChatMessage{Role: "user", Content: userPrompt}
	asstMsg := ChatMessage{Role: "assistant", Content: answer, Reasoning: reasoning}
	s.histories[target] = append(s.histories[target], userMsg, asstMsg)
	if len(s.histories[target]) > 30 {
		s.histories[target] = s.histories[target][len(s.histories[target])-30:]
	}
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
	ans, err := s.AskModel(target, req.Prompt, activeModel)
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
