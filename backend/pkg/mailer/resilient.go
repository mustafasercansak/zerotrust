package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type AlertJob struct {
	To           string
	AlertType    string
	IPAddress    string
	Location     string
	Details      string
	Attempt      int
	MaxRetries   int
	BaseDelay    time.Duration
}

type ResilientMailer struct {
	underlying Mailer
	auditLogFn func(ctx context.Context, email, alertType, ip, details string, err error)
	jobs       chan AlertJob
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	BaseDelay  time.Duration
}

func NewResilientMailer(underlying Mailer, queueSize int, auditLogFn func(ctx context.Context, email, alertType, ip, details string, err error)) *ResilientMailer {
	ctx, cancel := context.WithCancel(context.Background())
	rm := &ResilientMailer{
		underlying: underlying,
		auditLogFn: auditLogFn,
		jobs:       make(chan AlertJob, queueSize),
		ctx:        ctx,
		cancel:     cancel,
		BaseDelay:  1 * time.Second,
	}
	return rm
}

func (rm *ResilientMailer) Start(workers int) {
	for i := 0; i < workers; i++ {
		rm.wg.Add(1)
		go rm.worker()
	}
}

func (rm *ResilientMailer) Stop() {
	rm.cancel()
	close(rm.jobs)
	rm.wg.Wait()
}

func (rm *ResilientMailer) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	// Password resets are synchronous/direct (not anomalous alerts)
	return rm.underlying.SendPasswordReset(ctx, to, resetURL)
}

func (rm *ResilientMailer) SendSecurityAlert(ctx context.Context, to, alertType, ipAddress, location, details string) error {
	baseDelay := rm.BaseDelay
	if baseDelay == 0 {
		baseDelay = 1 * time.Second
	}
	job := AlertJob{
		To:          to,
		AlertType:   alertType,
		IPAddress:   ipAddress,
		Location:    location,
		Details:     details,
		Attempt:     0,
		MaxRetries:  5,
		BaseDelay:   baseDelay,
	}
	select {
	case rm.jobs <- job:
		return nil
	default:
		slog.Error("security alert queue is full, alert dropped", "to", to, "type", alertType)
		return fmt.Errorf("mailer queue is full")
	}
}

func (rm *ResilientMailer) worker() {
	defer rm.wg.Done()
	for {
		select {
		case <-rm.ctx.Done():
			return
		case job, ok := <-rm.jobs:
			if !ok {
				return
			}
			rm.processJob(job)
		}
	}
}

func (rm *ResilientMailer) processJob(job AlertJob) {
	// Build a timeout context for the send operation
	ctx, cancel := context.WithTimeout(rm.ctx, 15*time.Second)
	defer cancel()

	err := rm.underlying.SendSecurityAlert(ctx, job.To, job.AlertType, job.IPAddress, job.Location, job.Details)
	if err == nil {
		slog.Info("security alert email sent successfully", "to", job.To, "type", job.AlertType, "attempt", job.Attempt+1)
		return
	}

	job.Attempt++
	slog.Warn("failed to send security alert email", "to", job.To, "type", job.AlertType, "attempt", job.Attempt, "error", err)

	if job.Attempt >= job.MaxRetries {
		slog.Error("failed to send security alert email: max retries reached", "to", job.To, "type", job.AlertType, "error", err)
		if rm.auditLogFn != nil {
			rm.auditLogFn(context.Background(), job.To, job.AlertType, job.IPAddress, job.Details, err)
		}
		return
	}

	// Calculate exponential backoff: BaseDelay * 2^(Attempt-1)
	delay := job.BaseDelay * (1 << (job.Attempt - 1))
	slog.Info("scheduling security alert email retry", "to", job.To, "delay", delay, "next_attempt", job.Attempt+1)

	time.AfterFunc(delay, func() {
		select {
		case <-rm.ctx.Done():
			return
		case rm.jobs <- job:
		default:
			// If queue is full, attempt to queue synchronously in a separate goroutine so we don't block the timer thread
			go func() {
				select {
				case <-rm.ctx.Done():
					return
				case rm.jobs <- job:
				}
			}()
		}
	})
}
