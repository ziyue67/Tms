package maintenance

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/nodehub"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

type Store interface {
	RunSubscriptionMaintenance(context.Context, time.Time) error
	RunDailyFlowReset(context.Context, time.Time) error
	ExpireDueRecords(context.Context, time.Time) ([]store.ExpiredForward, error)
	RecordHourlyStatistics(context.Context, time.Time) error
}

type Scheduler struct {
	store  Store
	hub    *nodehub.Hub
	logger *slog.Logger
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func New(repository Store, hub *nodehub.Hub, logger *slog.Logger) *Scheduler {
	return &Scheduler{store: repository, hub: hub, logger: logger, done: make(chan struct{})}
}
func (s *Scheduler) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	go s.run(ctx)
}
func (s *Scheduler) Close() {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		<-s.done
	})
}
func (s *Scheduler) run(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	s.tick(ctx, time.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.tick(ctx, now)
		}
	}
}
func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	runCtx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	if err := s.store.RunSubscriptionMaintenance(runCtx, now); err != nil {
		s.logger.Error("subscription maintenance failed", "error", err)
	}
	expired, err := s.store.ExpireDueRecords(runCtx, now)
	if err != nil {
		s.logger.Error("expiry maintenance failed", "error", err)
	} else {
		for _, item := range expired {
			name := fmt.Sprintf("%d_%d_0", item.ID, item.UserID)
			_ = s.hub.SendCommand(runCtx, item.NodeID, "PauseService", map[string]any{"services": []string{name + "_tcp", name + "_udp"}})
		}
	}
	if now.Minute() == 0 {
		if err := s.store.RecordHourlyStatistics(runCtx, now); err != nil {
			s.logger.Error("hourly statistics failed", "error", err)
		}
	}
	if now.Hour() == 0 && now.Minute() == 0 {
		if err := s.store.RunDailyFlowReset(runCtx, now); err != nil {
			s.logger.Error("daily flow reset failed", "error", err)
		}
	}
}
