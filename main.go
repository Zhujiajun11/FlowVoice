package main

import (
	"encoding/json"
	"errors"
	"flag"
	"flowvoice/desktop"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode"
)

type Hotword struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type Settings struct {
	Mode           string    `json:"mode"`
	Language       string    `json:"language"`
	Insertion      string    `json:"insertion"`
	ShowInterim    bool      `json:"showInterim"`
	EnableCommands bool      `json:"enableCommands"`
	Hotwords       []Hotword `json:"hotwords"`
}

type ProcessRequest struct {
	Transcript string `json:"transcript"`
}

type CommandPayload struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
	Label string `json:"label"`
}

type ProcessResponse struct {
	Kind    string          `json:"kind"`
	Text    string          `json:"text,omitempty"`
	Message string          `json:"message,omitempty"`
	Command *CommandPayload `json:"command,omitempty"`
}

type DesktopPasteRequest struct {
	Text string `json:"text"`
}

type SettingsStore struct {
	mu       sync.RWMutex
	filePath string
	settings Settings
}

func main() {
	modeFlag := flag.String("mode", "web", "run mode: web or desktop")
	portFlag := flag.String("port", "", "http port, defaults to FLOWVOICE_PORT or 4173")
	openFlag := flag.Bool("open", false, "open the desktop input window on startup in desktop mode")
	flag.Parse()

	root, err := os.Getwd()
	if err != nil {
		log.Fatalf("resolve working directory: %v", err)
	}

	store, err := NewSettingsStore(filepath.Join(root, "data", "settings.json"))
	if err != nil {
		log.Fatalf("initialize settings store: %v", err)
	}

	mode := strings.ToLower(strings.TrimSpace(*modeFlag))
	port := resolvePort(*portFlag)
	uiURL := fmt.Sprintf("http://localhost:%s?desktop=1", port)

	var desktopIntegration desktop.Integration
	if mode == "desktop" {
		desktopIntegration, err = desktop.New(uiURL)
		if err != nil {
			log.Fatalf("initialize desktop integration: %v", err)
		}
	}

	server := NewServer(root, store, mode, desktopIntegration)

	log.Printf("FlowVoice %s server listening on http://localhost:%s", mode, port)
	if mode == "desktop" && *openFlag && desktopIntegration != nil {
		go func() {
			if openErr := desktopIntegration.OpenUI(); openErr != nil {
				log.Printf("open desktop ui: %v", openErr)
			}
		}()
	}

	if err := http.ListenAndServe(":"+port, server.routes()); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func resolvePort(flagValue string) string {
	port := strings.TrimSpace(flagValue)
	if port == "" {
		port = strings.TrimSpace(os.Getenv("FLOWVOICE_PORT"))
	}
	if port == "" {
		return "4173"
	}
	return port
}

func DefaultSettings() Settings {
	return Settings{
		Mode:           "balanced",
		Language:       "zh-CN",
		Insertion:      "cursor",
		ShowInterim:    true,
		EnableCommands: true,
		Hotwords: []Hotword{
			{Source: "欧喷 AI", Target: "OpenAI"},
			{Source: "大模型", Target: "大模型"},
			{Source: "flow voice", Target: "FlowVoice"},
		},
	}
}

func cloneSettings(input Settings) Settings {
	output := input
	output.Hotwords = append([]Hotword(nil), input.Hotwords...)
	return output
}

func sanitizeSettings(input Settings) Settings {
	output := cloneSettings(DefaultSettings())

	switch input.Mode {
	case "speed", "balanced", "review":
		output.Mode = input.Mode
	}

	switch input.Language {
	case "zh-CN", "en-US", "ja-JP":
		output.Language = input.Language
	}

	switch input.Insertion {
	case "cursor", "append":
		output.Insertion = input.Insertion
	}

	output.ShowInterim = input.ShowInterim
	output.EnableCommands = input.EnableCommands

	if len(input.Hotwords) > 0 {
		output.Hotwords = make([]Hotword, 0, len(input.Hotwords))
		for _, item := range input.Hotwords {
			output.Hotwords = append(output.Hotwords, Hotword{
				Source: strings.TrimSpace(item.Source),
				Target: strings.TrimSpace(item.Target),
			})
		}
	}

	return output
}

func NewSettingsStore(filePath string) (*SettingsStore, error) {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, err
	}

	store := &SettingsStore{
		filePath: filePath,
		settings: DefaultSettings(),
	}

	if data, err := os.ReadFile(filePath); err == nil {
		var saved Settings
		if unmarshalErr := json.Unmarshal(data, &saved); unmarshalErr != nil {
			return nil, unmarshalErr
		}
		store.settings = sanitizeSettings(saved)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if err := store.Save(store.settings); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *SettingsStore) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSettings(s.settings)
}

func (s *SettingsStore) Save(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.settings = sanitizeSettings(settings)
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0o644)
}

type Server struct {
	root    string
	store   *SettingsStore
	mode    string
	desktop desktop.Integration
}

func NewServer(root string, store *SettingsStore, mode string, desktopIntegration desktop.Integration) *Server {
	return &Server{
		root:    root,
		store:   store,
		mode:    mode,
		desktop: desktopIntegration,
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/process", s.handleProcess)
	mux.HandleFunc("/api/desktop/status", s.handleDesktopStatus)
	mux.HandleFunc("/api/desktop/open", s.handleDesktopOpen)
	mux.HandleFunc("/api/desktop/paste", s.handleDesktopPaste)
	mux.Handle("/", http.FileServer(http.Dir(s.root)))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"backend": "golang",
		"mode":    s.mode,
		"desktop": s.desktop != nil,
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.Get())
	case http.MethodPut:
		var payload Settings
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		if err := s.store.Save(payload); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save settings failed"})
			return
		}

		writeJSON(w, http.StatusOK, s.store.Get())
	default:
		writeMethodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

func (s *Server) handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}

	var payload ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	settings := s.store.Get()
	response := processTranscript(settings, payload.Transcript)
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleDesktopStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	if s.desktop == nil {
		writeJSON(w, http.StatusOK, DesktopStatus{
			Enabled:  false,
			Platform: runtime.GOOS,
			Mode:     s.mode,
			Note:     "当前服务未启用桌面集成，请用 -mode desktop 启动。",
		})
		return
	}

	writeJSON(w, http.StatusOK, s.desktop.Status())
}

type DesktopStatus = desktop.Status

func (s *Server) handleDesktopOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}

	if s.desktop == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "desktop mode is not enabled"})
		return
	}

	if err := s.desktop.OpenUI(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "opened"})
}

func (s *Server) handleDesktopPaste(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}

	if s.desktop == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "desktop mode is not enabled"})
		return
	}

	var payload DesktopPasteRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	if err := s.desktop.PasteText(payload.Text); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "pasted"})
}

func processTranscript(settings Settings, transcript string) ProcessResponse {
	normalized := normalizeText(transcript)
	if normalized == "" {
		return ProcessResponse{Kind: "noop"}
	}

	if settings.EnableCommands {
		if command, ok := resolveCommand(normalized); ok {
			return ProcessResponse{
				Kind:    "command",
				Message: "已执行语音命令",
				Command: &command,
			}
		}
	}

	processed := applyModeFormatting(normalized, settings)
	processed = applyHotwords(processed, settings.Hotwords)
	if processed == "" {
		return ProcessResponse{Kind: "noop"}
	}

	if settings.Mode == "review" {
		return ProcessResponse{
			Kind:    "queue",
			Text:    processed,
			Message: "片段已加入待确认队列",
		}
	}

	return ProcessResponse{
		Kind:    "insert",
		Text:    processed,
		Message: "片段已写入编辑区",
	}
}

func normalizeText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func stripCommandPunctuation(text string) string {
	var builder strings.Builder
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			continue
		case strings.ContainsRune("。，“”、？！,.!?", r):
			continue
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func resolveCommand(text string) (CommandPayload, bool) {
	switch stripCommandPunctuation(text) {
	case "换行", "下一行":
		return CommandPayload{Type: "insert", Value: "\n", Label: "换行"}, true
	case "句号":
		return CommandPayload{Type: "insert", Value: "。", Label: "句号"}, true
	case "逗号":
		return CommandPayload{Type: "insert", Value: "，", Label: "逗号"}, true
	case "问号":
		return CommandPayload{Type: "insert", Value: "？", Label: "问号"}, true
	case "感叹号":
		return CommandPayload{Type: "insert", Value: "！", Label: "感叹号"}, true
	case "空格":
		return CommandPayload{Type: "insert", Value: " ", Label: "空格"}, true
	case "删除上一段", "撤销":
		return CommandPayload{Type: "undo", Label: "撤销"}, true
	default:
		return CommandPayload{}, false
	}
}

func applyModeFormatting(text string, settings Settings) string {
	if settings.Mode == "speed" {
		return strings.Join(strings.Fields(text), "")
	}

	compact := normalizeText(text)
	if settings.Language == "zh-CN" {
		replacer := strings.NewReplacer(
			" ,", "，",
			" .", "。",
			" ?", "？",
			" !", "！",
		)
		return replacer.Replace(compact)
	}

	return compact
}

func applyHotwords(text string, hotwords []Hotword) string {
	output := text
	for _, pair := range hotwords {
		if pair.Source == "" || pair.Target == "" {
			continue
		}
		output = strings.ReplaceAll(output, pair.Source, pair.Target)
	}
	return output
}

func writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write json response: %v", err)
	}
}
