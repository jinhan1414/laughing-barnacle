package web

import (
	"net/http"
	"net/url"
	"time"
)

func (s *Server) handleChatReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := s.convStore.Reset(); err != nil {
		http.Redirect(w, r, "/chat?error="+url.QueryEscape("重置上下文失败："+err.Error()), http.StatusFound)
		return
	}
	if s.turns != nil {
		if err := s.turns.Reset(); err != nil {
			http.Redirect(w, r, "/chat?error="+url.QueryEscape("重置聊天队列失败："+err.Error()), http.StatusFound)
			return
		}
	}
	if s.agent != nil {
		if err := s.agent.ResetAsyncTasks(); err != nil {
			http.Redirect(w, r, "/chat?error="+url.QueryEscape("重置后台任务失败："+err.Error()), http.StatusFound)
			return
		}
	}
	if err := s.logStore.Clear(); err != nil {
		http.Redirect(w, r, "/chat?error="+url.QueryEscape("清空日志失败："+err.Error()), http.StatusFound)
		return
	}
	if s.memoryStore != nil {
		if err := s.memoryStore.Reset(); err != nil {
			http.Redirect(w, r, "/chat?error="+url.QueryEscape("重置记忆失败："+err.Error()), http.StatusFound)
			return
		}
	}
	if s.mcpStore != nil {
		if err := s.mcpStore.SetLastChatGreetingState("", time.Time{}, ""); err != nil {
			http.Redirect(w, r, "/chat?error="+url.QueryEscape("重置问候状态失败："+err.Error()), http.StatusFound)
			return
		}
	}

	http.Redirect(w, r, "/chat", http.StatusFound)
}
