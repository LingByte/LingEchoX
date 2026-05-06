package tasks

import (
	"context"
	"sync"
	"time"

	"github.com/LingByte/SoulNexus/internal/models"
	"github.com/LingByte/SoulNexus/pkg/logger"
	"github.com/LingByte/SoulNexus/pkg/sip/persist"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SIPUserOnlineCleaner periodically flips expired online SIP users to offline.
type SIPUserOnlineCleaner struct {
	db       *gorm.DB
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewSIPUserOnlineCleaner(db *gorm.DB, interval time.Duration) *SIPUserOnlineCleaner {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &SIPUserOnlineCleaner{
		db:       db,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (c *SIPUserOnlineCleaner) Start() {
	if c == nil || c.db == nil {
		return
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		c.sweep()
		for {
			select {
			case <-c.stopCh:
				return
			case <-ticker.C:
				c.sweep()
			}
		}
	}()
}

func (c *SIPUserOnlineCleaner) Stop() {
	if c == nil {
		return
	}
	close(c.stopCh)
	c.wg.Wait()
}

func (c *SIPUserOnlineCleaner) sweep() {
	rows, err := persist.MarkExpiredSIPUsersOffline(context.Background(), c.db, time.Now())
	if err != nil {
		if logger.Lg != nil {
			logger.Lg.Warn("sip user online cleaner failed", zap.Error(err))
		}
	} else if rows > 0 && logger.Lg != nil {
		logger.Lg.Info("sip user online cleaner marked users offline", zap.Int64("rows", rows))
	}

	webRows, webErr := models.MarkStaleWebACDPoolTargetsOffline(context.Background(), c.db, time.Now())
	if webErr != nil {
		if logger.Lg != nil {
			logger.Lg.Warn("web seat cleaner failed", zap.Error(webErr))
		}
	}
	if webRows > 0 && logger.Lg != nil {
		logger.Lg.Info("web seat cleaner marked seats offline", zap.Int64("rows", webRows))
	}

	shiftRows, shiftErr := models.MarkACDPoolTargetsOfflineOutsideSchedule(context.Background(), c.db, time.Now())
	if shiftErr != nil {
		if logger.Lg != nil {
			logger.Lg.Warn("acd shift cleaner failed", zap.Error(shiftErr))
		}
	}
	if shiftRows > 0 && logger.Lg != nil {
		logger.Lg.Info("acd shift cleaner marked seats offline", zap.Int64("rows", shiftRows))
	}
}

