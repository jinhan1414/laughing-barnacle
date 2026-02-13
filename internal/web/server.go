package web

import (
	"context"
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llmlog"
)

//go:embed templates/*.html
var embeddedTemplates embed.FS

type Server struct {
	agent     *agent.Agent
	convStore *conversation.Store
	logStore  *llmlog.Store
	tmpl      *template.Template
}

type chatPageData struct {
	Summary  string
	Messages []conversation.Message
	Error    string
}

type logsPageData struct {
	Entries []llmlog.Entry
}

func NewServer(agent *agent.Agent, convStore *conversation.Store, logStore *llmlog.Store) (*Server, error) {
	tmpl, err := template.ParseFS(embeddedTemplates, "templates/*.html")
	if err != nil {
		return nil, err
	}

	return &Server{
		agent:     agent,
		convStore: convStore,
		logStore:  logStore,
		tmpl:      tmpl,
	}, nil
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/chat", s.handleChatPage)
	mux.HandleFunc("/chat/send", s.handleChatSend)
	mux.HandleFunc("/logs", s.handleLogsPage)
	mux.HandleFunc("/healthz", s.handleHealthz)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/chat", http.StatusFound)
}

func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	summary, messages := s.convStore.Snapshot()
	data := chatPageData{
		Summary:  summary,
		Messages: messages,
		Error:    r.URL.Query().Get("error"),
	}
	_ = s.tmpl.ExecuteTemplate(w, "chat.html", data)
}

func (s *Server) handleChatSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/chat?error="+url.QueryEscape("请求参数解析失败"), http.StatusFound)
		return
	}

	message := strings.TrimSpace(r.FormValue("message"))
	if message == "" {
		http.Redirect(w, r, "/chat", http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	if _, err := s.agent.HandleUserMessage(ctx, message); err != nil {
		http.Redirect(w, r, "/chat?error="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}

	http.Redirect(w, r, "/chat", http.StatusFound)
}

func (s *Server) handleLogsPage(w http.ResponseWriter, r *http.Request) {
	data := logsPageData{Entries: s.logStore.List()}
	_ = s.tmpl.ExecuteTemplate(w, "logs.html", data)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
