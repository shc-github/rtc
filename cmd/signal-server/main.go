package main

import (
	"flag"
	"log"

	"github.com/stan/webrtc/pkg/signal"
)

func main() {
	addr := flag.String("addr", ":8080", "Server address")
	sfuURL := flag.String("sfu", "", "SFU server URL (e.g., ws://localhost:8081/sfu)")
	sfuThreshold := flag.Int("sfu-threshold", 3, "Number of users to switch to SFU mode")
	flag.Parse()

	server := signal.NewServer(*addr)

	// 配置 SFU
	if *sfuURL != "" {
		server.SetSFUURL(*sfuURL)
		server.SetSFUThreshold(*sfuThreshold)
		log.Printf("SFU enabled: %s, threshold: %d", *sfuURL, *sfuThreshold)
	}

	log.Printf("Starting signal server on %s", *addr)
	if err := server.Start(); err != nil {
		log.Fatalf("Server start error: %v", err)
	}
}