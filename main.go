package main

import (
	"flag"
	"log"
	"os"

	"ccproxy/config"
	"ccproxy/server"
)

func main() {
	var configFile = flag.String("config", "config.yaml", "Configuration file path")
	flag.Parse()

	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	srv := server.NewServer(cfg)

	log.Printf("Proxy targets configured:")
	for _, target := range cfg.Proxy.Targets {
		log.Printf("  %s -> %s (methods: %v)", target.Path, target.TargetURL, target.Methods)
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
		os.Exit(1)
	}
}
