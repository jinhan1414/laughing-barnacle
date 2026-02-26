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
	"laughing-barnacle/internal/memory"
	"laughing-barnacle/internal/scheduler"
	"laughing-barnacle/internal/skills"
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
	memoryStore, err := memory.NewStoreWithFile(cfg.MemoryFile)
	if err != nil {
		return err
	}
	defer memoryStore.Close()
	skillStore, err := skills.NewStore(cfg.SkillsDir, cfg.SkillsStateFile)
	if err != nil {
		return err
	}
	if err := skillStore.SetLocalAPIBaseURL(cfg.LocalAPIBaseURL); err != nil {
		return err
	}
	mcpStore, err := mcp.NewStore(cfg.SettingsFile)
	if err != nil {
		return err
	}
	if err := mcpStore.UpsertAgentPromptConfig(mcp.AgentPromptConfig{
		SystemPrompt:            cfg.AgentSystemPrompt,
		CompressionSystemPrompt: cfg.CompressionSystemPrompt,
	}); err != nil {
		return err
	}
	mcpHTTPClient := mcp.NewHTTPClient(cfg.MCPRequestTimeout, cfg.MCPProtocolVersion)
	mcpToolProvider := mcp.NewToolProvider(mcpStore, mcpHTTPClient, cfg.MCPToolCacheTTL)

	llmClient := cerber.NewClient(cerber.Config{
		BaseURL:        cfg.CerberBaseURL,
		APIKey:         cfg.CerberAPIKey,
		Timeout:        cfg.RequestTimeout,
		LogStore:       logStore,
		MaxRetries:     cfg.CerberMaxRetries,
		RetryBaseDelay: cfg.CerberRetryBaseDelay,
		RetryMaxDelay:  cfg.CerberRetryMaxDelay,
	})
	if cfg.MemoryExtractionUseLLM {
		memoryStore.SetSegmentExtractor(
			memory.NewLLMSegmentExtractor(llmClient, cfg.MemoryExtractionModel, cfg.MemoryExtractionTemperature),
			cfg.MemoryExtractionFallback,
		)
	}

	agentSvc := agent.New(agent.Config{
		Model:                      cfg.CerberModel,
		LocalAPIBaseURL:            cfg.LocalAPIBaseURL,
		Temperature:                cfg.Temperature,
		MaxRecentMessages:          cfg.MaxRecentMessages,
		CompressionTriggerMessages: cfg.CompressionTriggerMessages,
		CompressionTriggerChars:    cfg.CompressionTriggerChars,
		KeepRecentAfterCompression: cfg.KeepRecentAfterCompression,
		MaxCompressionLoopsPerTurn: cfg.MaxCompressionLoopsPerTurn,
		MaxToolCallRounds:          cfg.MaxToolCallRounds,
		SystemPrompt:               cfg.AgentSystemPrompt,
		CompressionSystemPrompt:    cfg.CompressionSystemPrompt,
		EnforceHumanRoutine:        true,
	}, convStore, llmClient, mcpToolProvider)
	agentSvc.SetSkillProvider(skillStore)
	agentSvc.SetMemoryProvider(memoryStore)
	agentSvc.SetPromptProvider(mcpStore)
	agentSvc.SetPromptUpdater(mcpStore)

	memoryWorker := memory.NewWorker(
		memoryStore,
		cfg.MemoryWorkerInterval,
		cfg.MemoryIdleWindow,
		cfg.MemoryMaxSegmentWindow,
		cfg.MemoryMaxSegmentMessages,
		cfg.MemoryTrashTTL,
		cfg.MemoryFailedRetryAfter,
	)
	memoryWorker.Start()
	defer memoryWorker.Stop()

	cronScheduler := scheduler.NewEngine(mcpStore, agentSvc, log.Default())
	if err := cronScheduler.Start(); err != nil {
		return err
	}
	defer cronScheduler.Stop()
	webServer, err := web.NewServer(agentSvc, convStore, logStore, mcpStore, mcpToolProvider, skillStore, cronScheduler)
	if err != nil {
		return err
	}
	webServer.SetMemoryStore(memoryStore)
	webServer.SetMemoryWorkerConfig(
		cfg.MemoryWorkerInterval,
		cfg.MemoryIdleWindow,
		cfg.MemoryMaxSegmentWindow,
		cfg.MemoryMaxSegmentMessages,
	)

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
