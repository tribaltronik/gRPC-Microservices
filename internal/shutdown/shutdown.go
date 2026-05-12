package shutdown

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Callback represents a shutdown function to be called during graceful shutdown.
type Callback struct {
	Name string
	Func func(context.Context) error
}

// Graceful waits for SIGTERM or SIGINT, then calls each callback sequentially
// with the given timeout applied to each.
func Graceful(ctx context.Context, timeout time.Duration, callbacks ...Callback) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case <-ctx.Done():
	case sig := <-sigCh:
		log.Printf("received signal %s, starting graceful shutdown", sig)
	}

	for _, cb := range callbacks {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		log.Printf("shutdown: %s", cb.Name)
		if err := cb.Func(shutdownCtx); err != nil {
			log.Printf("shutdown %s: %v", cb.Name, err)
		}
		cancel()
	}
}
