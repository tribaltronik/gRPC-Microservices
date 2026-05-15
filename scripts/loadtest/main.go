package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"github.com/tiagoricardo/grpc-microservices/internal/tls"
	orderv1 "github.com/tiagoricardo/grpc-microservices/services/order/v1"
	userv1 "github.com/tiagoricardo/grpc-microservices/services/user/v1"
)

type result struct {
	duration time.Duration
	err      bool
	op       string
}

type opStats struct {
	count    int64
	errCount int64
	durs     []time.Duration
	mu       sync.Mutex
}

func (s *opStats) add(d time.Duration, err bool) {
	atomic.AddInt64(&s.count, 1)
	if err {
		atomic.AddInt64(&s.errCount, 1)
	}
	s.mu.Lock()
	s.durs = append(s.durs, d)
	s.mu.Unlock()
}

func (s *opStats) report(label string) {
	n := atomic.LoadInt64(&s.count)
	if n == 0 {
		return
	}
	s.mu.Lock()
	durs := make([]time.Duration, len(s.durs))
	copy(durs, s.durs)
	s.mu.Unlock()
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })

	var total time.Duration
	for _, d := range durs {
		total += d
	}
	avg := total / time.Duration(len(durs))
	p50 := durs[len(durs)/2]
	p95 := durs[int(math.Ceil(float64(len(durs))*0.95))-1]
	p99 := durs[int(math.Ceil(float64(len(durs))*0.99))-1]
	rps := float64(n) / total.Seconds()
	errRate := float64(atomic.LoadInt64(&s.errCount)) / float64(n) * 100

	fmt.Printf("  %-14s | %6d req | %6.1f rps | avg %6s | p50 %6s | p95 %6s | p99 %6s | err %4.1f%%\n",
		label, n, rps, fmtDuration(avg), fmtDuration(p50), fmtDuration(p95), fmtDuration(p99), errRate)
}

func fmtDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.0fµs", float64(d.Microseconds()))
	}
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d.Milliseconds()))
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func main() {
	vus := flag.Int("vus", 10, "number of concurrent virtual users")
	duration := flag.Duration("duration", 30*time.Second, "test duration")
	userAddr := flag.String("user-addr", "localhost:50051", "user-service address")
	orderAddr := flag.String("order-addr", "localhost:50052", "order-service address")
	certDir := flag.String("cert-dir", "certs/api-gateway", "client cert directory")
	caFile := flag.String("ca-file", "certs/ca/ca.pem", "CA certificate file")
	writeRatio := flag.Float64("write-ratio", 0.3, "ratio of write operations (0-1)")
	flag.Parse()

	ca := *caFile
	cert := *certDir + "/cert.pem"
	key := *certDir + "/key.pem"

	userConn := dial(*userAddr, ca, cert, key)
	defer userConn.Close()
	orderConn := dial(*orderAddr, ca, cert, key)
	defer orderConn.Close()

	userClient := userv1.NewUserServiceClient(userConn)
	orderClient := orderv1.NewOrderServiceClient(orderConn)

	ctx := context.Background()

	// Pre-create test users
	fmt.Println("=== Pre-creating test users ===")
	var userIDs []string
	for i := 0; i < *vus*2; i++ {
		email := fmt.Sprintf("loadtest_%d_%d@example.com", time.Now().UnixNano(), i)
		resp, err := userClient.CreateUser(ctx, &userv1.CreateUserRequest{
			Name: fmt.Sprintf("LoadTestUser_%d", i), Email: email,
		})
		if err == nil {
			userIDs = append(userIDs, resp.User.Id)
		}
	}
	fmt.Printf("Created %d test users\n", len(userIDs))
	if len(userIDs) == 0 {
		log.Fatal("failed to create any test users")
	}

	// Pre-create orders for read operations
	fmt.Println("=== Pre-creating test orders ===")
	var orderIDs []string
	for i := 0; i < *vus; i++ {
		uid := userIDs[i%len(userIDs)]
		resp, err := orderClient.CreateOrder(ctx, &orderv1.CreateOrderRequest{
			UserId: uid,
			Items: []*orderv1.OrderItem{{
				ProductId: "loadtest-prod", ProductName: "Load Test Product",
				Quantity: 1, UnitPrice: 10.0,
			}},
		})
		if err == nil {
			orderIDs = append(orderIDs, resp.Order.Id)
		}
	}
	fmt.Printf("Created %d test orders\n\n", len(orderIDs))

	total := &opStats{}
	createUserStats := &opStats{}
	getUserStats := &opStats{}
	createOrderStats := &opStats{}
	getOrderStats := &opStats{}

	start := time.Now()
	var wg sync.WaitGroup
	results := make(chan result, *vus*100)

	go func() {
		for r := range results {
			total.add(r.duration, r.err)
			switch r.op {
			case "CreateUser":
				createUserStats.add(r.duration, r.err)
			case "GetUser":
				getUserStats.add(r.duration, r.err)
			case "CreateOrder":
				createOrderStats.add(r.duration, r.err)
			case "GetOrder":
				getOrderStats.add(r.duration, r.err)
			}
		}
	}()

	for i := 0; i < *vus; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)))
			for {
				if time.Since(start) >= *duration {
					return
				}

				op := randomOp(*writeRatio, rng)
				opStart := time.Now()
				var err error

				switch op {
				case "CreateUser":
					email := fmt.Sprintf("vu_%d_%d@example.com", id, time.Now().UnixNano())
					_, err = userClient.CreateUser(ctx, &userv1.CreateUserRequest{
						Name: fmt.Sprintf("VU_%d", id), Email: email,
					})
				case "GetUser":
					uid := userIDs[rng.Intn(len(userIDs))]
					_, err = userClient.GetUser(ctx, &userv1.GetUserRequest{Id: uid})
				case "CreateOrder":
					uid := userIDs[rng.Intn(len(userIDs))]
					_, err = orderClient.CreateOrder(ctx, &orderv1.CreateOrderRequest{
						UserId: uid,
						Items: []*orderv1.OrderItem{{
							ProductId: "loadtest-prod", ProductName: "Load Test Product",
							Quantity: int32(rng.Intn(5) + 1), UnitPrice: 10.0,
						}},
					})
				case "GetOrder":
					if len(orderIDs) > 0 {
						oid := orderIDs[rng.Intn(len(orderIDs))]
						_, err = orderClient.GetOrder(ctx, &orderv1.GetOrderRequest{Id: oid})
					}
				}

				results <- result{op: op, duration: time.Since(opStart), err: err != nil}
			}
		}(i)
	}

	wg.Wait()
	close(results)
	elapsed := time.Since(start)

	fmt.Println("=== Load Test Results ===")
	fmt.Printf("Duration:      %v\n", elapsed)
	fmt.Printf("Concurrency:   %d VUs\n", *vus)
	fmt.Printf("Write ratio:   %.0f%%\n", *writeRatio*100)
	fmt.Println()

	fmt.Printf("  %-14s | %6s | %7s | %7s | %7s | %7s | %7s | %6s\n",
		"Operation", "count", "rps", "avg", "p50", "p95", "p99", "err%")
	fmt.Println("  " + strings.Repeat("-", 95))

	total.report("TOTAL")
	createUserStats.report("CreateUser")
	getUserStats.report("GetUser")
	createOrderStats.report("CreateOrder")
	getOrderStats.report("GetOrder")

	writeCount := createUserStats.count + createOrderStats.count
	readCount := getUserStats.count + getOrderStats.count
	totalCount := total.count
	fmt.Printf("\n  Writes: %d (%.0f%%) | Reads: %d (%.0f%%) | Total: %d\n",
		writeCount, float64(writeCount)/float64(totalCount)*100,
		readCount, float64(readCount)/float64(totalCount)*100,
		totalCount)
}

func randomOp(writeRatio float64, rng *rand.Rand) string {
	if rng.Float64() < writeRatio {
		if rng.Float64() < 0.5 {
			return "CreateUser"
		}
		return "CreateOrder"
	}
	if rng.Float64() < 0.5 {
		return "GetUser"
	}
	return "GetOrder"
}

func dial(addr, caFile, certFile, keyFile string) *grpc.ClientConn {
	var opts []grpc.DialOption
	if certFile != "" && keyFile != "" && caFile != "" {
		creds, err := tls.LoadClientConfig(caFile, certFile, keyFile)
		if err != nil {
			log.Fatalf("load TLS: %v", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}
	conn, err := grpc.Dial(addr, opts...)
	if err != nil {
		log.Fatalf("dial %s: %v", addr, err)
	}
	return conn
}
