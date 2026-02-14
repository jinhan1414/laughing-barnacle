package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"laughing-barnacle/internal/agent"
	"laughing-barnacle/internal/config"
	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm/cerber"
	"laughing-barnacle/internal/llmlog"
	"laughing-barnacle/internal/mcp"
	"laughing-barnacle/internal/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logStore, err := llmlog.NewStoreWithFile(cfg.LLMLogLimit, cfg.LLMLogFile)
	if err != nil {
		return err
	}
	convStore, err := conversation.NewStoreWithFile(cfg.ConversationFile)
	if err != nil {
		return err
	}
	mcpStore, err := mcp.NewStore(cfg.SettingsFile)
	if err != nil {
		return err
	}
	mcpHTTPClient := mcp.NewHTTPClient(cfg.MCPRequestTimeout, cfg.MCPProtocolVersion)
	mcpToolProvider := mcp.NewToolProvider(mcpStore, mcpHTTPClient, cfg.MCPToolCacheTTL)

	llmClient := cerber.NewClient(cerber.Config{
		BaseURL:  cfg.CerberBaseURL,
		APIKey:   cfg.CerberAPIKey,
		Timeout:  cfg.RequestTimeout,
		LogStore: logStore,
	})

	agentSvc := agent.New(agent.Config{
		Model:                      cfg.CerberModel,
		Temperature:                cfg.Temperature,
		MaxRecentMessages:          cfg.MaxRecentMessages,
		CompressionTriggerMessages: cfg.CompressionTriggerMessages,
		CompressionTriggerChars:    cfg.CompressionTriggerChars,
		KeepRecentAfterCompression: cfg.KeepRecentAfterCompression,
		MaxCompressionLoopsPerTurn: cfg.MaxCompressionLoopsPerTurn,
		MaxToolCallRounds:          cfg.MaxToolCallRounds,
		SystemPrompt:               cfg.AgentSystemPrompt,
		CompressionSystemPrompt:    cfg.CompressionSystemPrompt,
	}, convStore, llmClient, mcpToolProvider)
	agentSvc.SetSkillProvider(mcpStore)
	agentSvc.SetPromptProvider(mcpStore)

	webServer, err := web.NewServer(agentSvc, convStore, logStore, mcpStore, mcpToolProvider)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	webServer.RegisterRoutes(mux)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("HTTP server listening on %s", cfg.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("listen error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpServer.Shutdown(ctx)
}
