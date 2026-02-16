package backups

import (
	"context"
	"sync"
	"time"

	"berkut-scc/config"
)

type Scheduler struct {
	cfg config.SchedulerConfig
	svc *Service

	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
	wg      sync.WaitGroup
}

func NewScheduler(cfg config.SchedulerConfig, svc *Service) *Scheduler {
	return &Scheduler{cfg: cfg, svc: svc}
}

func (s *Scheduler) StartWithContext(ctx context.Context) {
	if s == nil || s.svc == nil || !s.cfg.Enabled {
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.running = true
	s.wg.Add(1)
	s.mu.Unlock()

	interval := time.Duration(s.cfg.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer s.wg.Done()
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = s.RunOnce(runCtx, time.Now().UTC())
			case <-runCtx.Done():
				return
			}
		}
	}()
}

func (s *Scheduler) StopWithContext(ctx context.Context) error {
	if s == nil || !s.cfg.Enabled {
		return nil
	}
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	wasRunning := s.running
	s.mu.Unlock()
	if !wasRunning || cancel == nil {
		return nil
	}
	cancel()
	waitDone := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) RunOnce(ctx context.Context, now time.Time) error {
	if s == nil || s.svc == nil || !s.cfg.Enabled {
		return nil
	}
	_ = s.svc.RunAutoRestoreTest(ctx, now.UTC())
	plan, err := s.svc.GetPlan(ctx)
	if err != nil || plan == nil || !plan.Enabled {
		return err
	}
	var lastRun *time.Time
	if plan.LastAutoRunAt != nil {
		lastRun = plan.LastAutoRunAt
	} else {
		items, listErr := s.svc.ListArtifacts(ctx, ListArtifactsFilter{Limit: 50, Offset: 0})
		if listErr == nil && len(items) > 0 {
			for i := range items {
				if items[i].Status == StatusSuccess {
					lastRun = &items[i].CreatedAt
					break
				}
			}
		}
	}
	if lastRun == nil {
		seed := now.UTC()
		lastRun = &seed
	}
	if !shouldRunByPlan(*plan, lastRun, now.UTC()) {
		return nil
	}
	return s.svc.RunAutoBackup(ctx)
}
