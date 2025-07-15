package tuning

import (
	"context"
	"sync"
	"time"

	"github.com/musix/backhaul/internal/config"
	"github.com/sirupsen/logrus"
)

// Tuner dynamically adjusts application parameters based on system metrics.
type Tuner struct {
	config    *config.Config
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	logger    *logrus.Logger
	ticker    *time.Ticker
}

// NewTuner creates a new Tuner instance.
func NewTuner(cfg *config.Config, logger *logrus.Logger) *Tuner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Tuner{
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
		logger: logger,
	}
}

// Start begins the dynamic tuning process.
func (t *Tuner) Start(interval time.Duration) {
	t.ticker = time.NewTicker(interval)
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		for {
			select {
			case <-t.ticker.C:
				t.adjustParameters()
			case <-t.ctx.Done():
				t.ticker.Stop()
				return
			}
		}
	}()
	t.logger.Info("Dynamic tuner started.")
}

// Stop halts the dynamic tuning process.
func (t *Tuner) Stop() {
	t.cancel()
	t.wg.Wait()
	t.logger.Info("Dynamic tuner stopped.")
}

// adjustParameters collects metrics and adjusts config values.
func (t *Tuner) adjustParameters() {
	t.logger.Debug("Running automatic parameter adjustment...")

	cpuUsage, err := GetCPUUsage()
	if err != nil {
		t.logger.WithError(err).Error("Failed to get CPU usage")
	} else {
		t.logger.WithField("usage", cpuUsage).Debug("Current CPU usage")
	}

	memUsage, err := GetMemoryUsage()
	if err != nil {
		t.logger.WithError(err).Error("Failed to get memory usage")
	} else {
		t.logger.WithField("usage", memUsage).Debug("Current memory usage")
	}

	// Adjust client connection pool
	if cpuUsage > 85.0 || memUsage > 85.0 {
		if t.config.Client.ConnectionPool > 1 {
			t.config.Client.ConnectionPool--
			t.logger.WithField("pool_size", t.config.Client.ConnectionPool).Info("Decreased client connection pool")
		}
	} else if cpuUsage < 50.0 && memUsage < 50.0 {
		if t.config.Client.ConnectionPool < 64 { // Set a reasonable upper limit
			t.config.Client.ConnectionPool++
			t.logger.WithField("pool_size", t.config.Client.ConnectionPool).Info("Increased client connection pool")
		}
	}

	// Adjust server channel size
	if memUsage > 80.0 {
		if t.config.Server.ChannelSize > 256 { // Set a reasonable lower limit
			t.config.Server.ChannelSize /= 2
			t.logger.WithField("channel_size", t.config.Server.ChannelSize).Info("Decreased server channel size")
		}
	} else if memUsage < 40.0 {
		if t.config.Server.ChannelSize < 8192 { // Set a reasonable upper limit
			t.config.Server.ChannelSize *= 2
			t.logger.WithField("channel_size", t.config.Server.ChannelSize).Info("Increased server channel size")
		}
	}
}
