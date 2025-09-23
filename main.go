package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/musix/backhaul/cmd"
	"github.com/musix/backhaul/internal/tuning"
	"github.com/musix/backhaul/internal/utils"
)

var (
	logger       = utils.NewLogger("info")
	configPath   *string
	noAutoTune   *bool
	tuneInterval *time.Duration
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.RWMutex
	tuner        *tuning.Tuner
	appProcess   *os.Process
)

// Define the version of the application
const version = "v0.6.6"

func main() {
	configPath = flag.String("c", "", "path to the configuration file (TOML format)")
	noAutoTune = flag.Bool("no-auto-tune", false, "disable automatic performance tuning")
	tuneInterval = flag.Duration("tune-interval", 10*time.Minute, "interval for automatic tuning (recommended: 10m for tunnels, 15m for very stable networks)")
	showVersion := flag.Bool("v", false, "print the version and exit")
	flag.Parse()

	// If the version flag is provided, print the version and exit
	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	// Check if the configPath is provided
	if *configPath == "" {
		logger.Fatalf("Usage: %s -c /path/to/config.toml", flag.CommandLine.Name())
	}

	// Apply temporary TCP optimizations at startup
	cmd.ApplyTCPTuning()

	// Create a context for graceful shutdown handling
	ctx, cancel = context.WithCancel(context.Background())

	// Set up signal handling for graceful shutdown - AGGRESSIVE MODE
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	// Start shutdown handler in a separate goroutine
	go handleShutdown(sigChan)

	// Start the main application logic
	logger.Info("Starting Backhaul application...")

	// Run the application in a separate goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("Application panic: %v", r)
			}
		}()

		cfg := cmd.Run(*configPath, ctx)

		// Start the dynamic tuner unless --no-auto-tune is set
		if !*noAutoTune {
			mu.Lock()
			tuner = tuning.NewTuner(cfg, utils.NewLogger(cfg.Server.LogLevel))
			tuner.Start(*tuneInterval)
			mu.Unlock()
			logger.Info("Auto-tuning enabled")
		} else {
			logger.Info("Auto-tuning disabled by flag")
		}

		// Wait for context cancellation
		<-ctx.Done()
		logger.Info("Application context cancelled, shutting down...")
	}()

	// Start hot reload monitoring
	wg.Add(1)
	go hotReload()

	logger.Info("Application started successfully. Press Ctrl+C to stop.")

	// Wait for all goroutines to finish
	wg.Wait()
	logger.Info("Application stopped")
}

func handleShutdown(sigChan chan os.Signal) {
	// Wait for first signal
	sig := <-sigChan
	logger.Infof("Received signal: %v, initiating graceful shutdown...", sig)

	// Cancel context immediately
	cancel()

	// Start a goroutine to handle force shutdown
	go func() {
		// Wait for second signal for immediate force shutdown
		select {
		case sig2 := <-sigChan:
			logger.Warnf("Received second signal: %v, forcing immediate shutdown!", sig2)
			forceShutdown()
		case <-time.After(5 * time.Second):
			logger.Warn("Graceful shutdown timeout (5s), forcing shutdown...")
			forceShutdown()
		}
	}()

	// Stop tuner if running
	if noAutoTune != nil && !*noAutoTune {
		mu.RLock()
		if tuner != nil {
			logger.Info("Stopping auto-tuner...")
			tuner.Stop()
		}
		mu.RUnlock()
	}

	// Create a timeout for graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-done:
		logger.Info("Graceful shutdown completed")
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timeout reached, forcing exit")
		forceShutdown()
	}
}

func forceShutdown() {
	logger.Error("Force shutdown initiated!")

	// Stop tuner immediately
	if noAutoTune != nil && !*noAutoTune {
		mu.RLock()
		if tuner != nil {
			tuner.Stop()
		}
		mu.RUnlock()
	}

	// Close all possible file descriptors and connections
	// This is a nuclear option
	if appProcess != nil {
		appProcess.Kill()
	}

	// Force exit
	os.Exit(1)
}

func hotReload() {
	defer wg.Done()

	// Get initial modification time of the config file
	lastModTime, err := getLastModTime(*configPath)
	if err != nil {
		logger.Fatalf("Error getting modification time: %v", err)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	logger.Info("Hot reload monitoring started")

	for {
		select {
		case <-ctx.Done():
			logger.Info("Hot reload monitoring stopped")
			return
		case <-ticker.C:
			modTime, err := getLastModTime(*configPath)
			if err != nil {
				logger.Errorf("Error checking file modification time: %v", err)
				continue
			}

			// If the modification time has changed, reload the app
			if modTime.After(lastModTime) {
				logger.Info("Config file changed, reloading application...")

				// Stop tuner before reloading
				if noAutoTune != nil && !*noAutoTune {
					mu.Lock()
					if tuner != nil {
						tuner.Stop()
						tuner = nil
					}
					mu.Unlock()
				}

				// Cancel the previous context to stop the old running instance
				cancel()

				// Wait a bit for graceful shutdown
				time.Sleep(3 * time.Second)

				// Create a new context for the new instance
				mu.Lock()
				ctx, cancel = context.WithCancel(context.Background())
				mu.Unlock()

				// Start the new instance
				wg.Add(1)
				go func() {
					defer wg.Done()
					cfg := cmd.Run(*configPath, ctx)

					// Restart tuner if needed
					if noAutoTune != nil && !*noAutoTune {
						mu.Lock()
						tuner = tuning.NewTuner(cfg, utils.NewLogger(cfg.Server.LogLevel))
						tuner.Start(*tuneInterval)
						mu.Unlock()
						logger.Info("Auto-tuner restarted")
					}
				}()

				// Update the last modification time
				lastModTime = modTime
				logger.Info("Application reloaded successfully")
			}
		}
	}
}

func getLastModTime(file string) (time.Time, error) {
	absPath, _ := filepath.Abs(file)
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		return time.Time{}, err
	}
	return fileInfo.ModTime(), nil
}

// init function to capture the current process
func init() {
	appProcess = &os.Process{Pid: os.Getpid()}
}
