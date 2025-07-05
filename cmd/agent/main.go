package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/onkar717/visual-eyes/internal/agent"
	"github.com/onkar717/visual-eyes/internal/common/config"
)

func main() {
	// Load configuration from file
	configPath := os.Getenv("VISUAL_EYES_CONFIG")
	if configPath == "" {
		configPath = "configs/config.yaml"
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create and start agent
	agent := agent.NewAgent(cfg)
	if err := agent.Start(); err != nil {
		log.Fatalf("Failed to start agent: %v", err)
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Stop agent gracefully
	agent.Stop()
	log.Println("Agent stopped")
}
