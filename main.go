package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"awesome-token-monitor/collector"
	"awesome-token-monitor/db"
	"awesome-token-monitor/server"
)

func isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func main() {
	portFlag := flag.Int("port", 8765, "HTTP server port")
	dbFlag := flag.String("db", "token_monitor_cache.db", "SQLite cache database file")
	webFlag := flag.String("web", "web", "Directory containing dashboard web frontend")
	flag.Parse()

	// Ensure absolute or clean paths
	execPath, err := os.Executable()
	baseDir := "."
	if err == nil {
		baseDir = filepath.Dir(execPath)
	}

	webDir := *webFlag
	if !filepath.IsAbs(webDir) {
		// check relative to current dir, then relative to executable
		if _, err := os.Stat(webDir); os.IsNotExist(err) {
			candidate := filepath.Join(baseDir, webDir)
			if _, err := os.Stat(candidate); err == nil {
				webDir = candidate
			}
		}
	}

	dbPath := *dbFlag
	if !filepath.IsAbs(dbPath) {
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			candidate := filepath.Join(baseDir, dbPath)
			if _, err := os.Stat(candidate); err == nil {
				dbPath = candidate
			}
		}
	}

	fmt.Println("[*] Opening SQLite database:", dbPath)
	database, err := db.NewDB(dbPath)
	if err != nil {
		log.Fatalf("[!] Failed to initialize database: %v", err)
	}
	defer database.Close()

	fmt.Println("[*] Initializing multi-agent collectors...")
	orchestrator, err := collector.NewOrchestrator(database)
	if err != nil {
		log.Fatalf("[!] Failed to initialize orchestrator: %v", err)
	}

	fmt.Println("[*] Running initial multi-agent token scan...")
	syncRes, err := orchestrator.Sync()
	if err != nil {
		log.Printf("[!] Initial sync warning: %v", err)
	} else {
		fmt.Printf("[*] Initial sync complete in %.2fs. Agent breakdown: %+v\n", syncRes.DurationSeconds, syncRes.Counts)
	}

	// Background auto-refresh worker
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			_, err := orchestrator.Sync()
			if err != nil {
				log.Printf("[Sync Worker Error]: %v", err)
			}
		}
	}()

	port := *portFlag
	if !isPortAvailable(port) {
		fmt.Printf("[!] Port %d is in use, trying %d...\n", port, port+1)
		port++
	}

	srv := server.NewServer(port, webDir, database, orchestrator)
	if err := srv.Start(); err != nil {
		log.Fatalf("[!] Server failed: %v", err)
	}
}
