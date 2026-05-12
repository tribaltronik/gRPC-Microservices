package shutdown

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
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

	var wg sync.WaitGroup
	for _, cb := range callbacks {
		wg.Add(1)
		go func(c Callback) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			log.Printf("shutdown: %s", c.Name)
			if err := c.Func(ctx); err != nil {
				log.Printf("shutdown %s: %v", c.Name, err)
			}
		}(cb)
	}
	wg.Wait()
}
