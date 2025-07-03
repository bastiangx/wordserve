//go:build test

package mem

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"github.com/bastiangx/typr-lib/pkg/suggest"
	"github.com/charmbracelet/log"
)

func init() {
	log.SetLevel(log.ErrorLevel)
}

var testPrefixes = []string{
	"a", "ab", "abc", "abcd",
	"h", "he", "hel", "hell", "hello",
	"w", "wo", "wor", "worl", "world",
	"p", "pr", "pro", "prog", "program",
	"t", "th", "the", "ther", "there",
	"c", "co", "com", "comp", "computer",
}

var longPatterns = [][]string{
	{"a", "ab", "abc", "abcd", "abcde"},
	{"h", "he", "hel", "hell", "hello"},
	{"w", "wo", "wor", "worl", "world"},
	{"p", "pr", "pro", "prog", "progr", "progra", "program"},
	{"t", "th", "the", "ther", "there"},
	{"c", "co", "com", "comp", "compu", "comput", "computer"},
	{"i", "in", "int", "inte", "inter", "intern", "interna", "internat", "internati", "internatio", "internation", "internationa", "international"},
	{"d", "de", "dev", "deve", "devel", "develo", "develop", "developm", "developme", "developmen", "development"},
}

func TestMemoryLeakBasic(t *testing.T) {
	iterations := []int{100, 500, 1000, 2500, 5000}
	
	for _, iterCount := range iterations {
		t.Run(fmt.Sprintf("iterations_%d", iterCount), func(t *testing.T) {
			runBasicMemoryTest(t, iterCount, testPrefixes)
		})
	}
}

func TestMemoryLeakConcurrent(t *testing.T) {
	configs := []struct {
		workers int
		iterationsPerWorker int
	}{
		{workers: 1, iterationsPerWorker: 1000},
		{workers: 2, iterationsPerWorker: 500},
		{workers: 4, iterationsPerWorker: 250},
		{workers: 8, iterationsPerWorker: 125},
	}
	
	for _, config := range configs {
		t.Run(fmt.Sprintf("workers_%d_iter_%d", config.workers, config.iterationsPerWorker), func(t *testing.T) {
			runConcurrentMemoryTest(t, config.workers, config.iterationsPerWorker)
		})
	}
}

func TestMemoryStabilityLongRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long-running memory stability test in short mode")
	}
	
	cycles := 50
	opsPerCycle := 200
	
	runLongRunMemoryTest(t, cycles, opsPerCycle)
}

func runBasicMemoryTest(t *testing.T, iterations int, prefixes []string) {
	completer := suggest.NewLazyCompleter("../../data", 10000, 50000)
	if err := completer.Initialize(); err != nil {
		t.Fatalf("completer initialization failed: %v", err)
	}
	defer completer.Stop()

	var baseline runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&baseline)
	baselineGoroutines := runtime.NumGoroutine()

	for i := 0; i < iterations; i++ {
		for _, prefix := range prefixes {
			suggestions := completer.Complete(prefix, 10)
			_ = suggestions
		}
	}

	var final runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&final)
	finalGoroutines := runtime.NumGoroutine()

	memDelta := int64(final.Alloc - baseline.Alloc)
	goroutineDelta := finalGoroutines - baselineGoroutines
	totalOps := iterations * len(prefixes)
	memPerOp := float64(memDelta) / float64(totalOps)

	t.Logf("iterations=%d ops=%d mem_delta=%d bytes mem_per_op=%.2f goroutine_delta=%d",
		iterations, totalOps, memDelta, memPerOp, goroutineDelta)

	if memPerOp > 1000 {
		t.Errorf("excessive memory usage per operation: %.2f bytes", memPerOp)
	}

	if goroutineDelta > 2 {
		t.Errorf("goroutine leak detected: %d goroutines leaked", goroutineDelta)
	}
}

func runConcurrentMemoryTest(t *testing.T, workers, iterationsPerWorker int) {
	memFile, err := os.Create("concurrent_memory.prof")
	if err != nil {
		t.Fatalf("profile file creation failed: %v", err)
	}
	defer func() {
		memFile.Close()
		os.Remove("concurrent_memory.prof")
	}()

	completer := suggest.NewLazyCompleter("../../data", 10000, 50000)
	if err := completer.Initialize(); err != nil {
		t.Fatalf("completer initialization failed: %v", err)
	}
	defer completer.Stop()

	var baseline runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&baseline)
	baselineGoroutines := runtime.NumGoroutine()

	var wg sync.WaitGroup
	totalOps := 0

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for iter := 0; iter < iterationsPerWorker; iter++ {
				for _, pattern := range longPatterns {
					for _, prefix := range pattern {
						suggestions := completer.Complete(prefix, 10)
						_ = suggestions
						totalOps++
					}
				}
			}
		}()
	}

	wg.Wait()

	var final runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&final)
	finalGoroutines := runtime.NumGoroutine()

	memDelta := int64(final.Alloc - baseline.Alloc)
	goroutineDelta := finalGoroutines - baselineGoroutines
	memPerOp := float64(memDelta) / float64(totalOps)

	t.Logf("workers=%d iter_per_worker=%d total_ops=%d mem_delta=%d bytes mem_per_op=%.2f goroutine_delta=%d",
		workers, iterationsPerWorker, totalOps, memDelta, memPerOp, goroutineDelta)

	if err := pprof.WriteHeapProfile(memFile); err != nil {
		t.Errorf("heap profile write failed: %v", err)
	}

	if memPerOp > 1000 {
		t.Errorf("excessive memory usage per operation: %.2f bytes", memPerOp)
	}

	if goroutineDelta > 3 {
		t.Errorf("goroutine leak detected: %d goroutines leaked", goroutineDelta)
	}
}

func runLongRunMemoryTest(t *testing.T, cycles, opsPerCycle int) {
	memFile, err := os.Create("longrun_stability.prof")
	if err != nil {
		t.Fatalf("profile file creation failed: %v", err)
	}
	defer func() {
		memFile.Close()
		os.Remove("longrun_stability.prof")
	}()

	completer := suggest.NewLazyCompleter("../../data", 10000, 50000)
	if err := completer.Initialize(); err != nil {
		t.Fatalf("completer initialization failed: %v", err)
	}
	defer completer.Stop()

	var baseline runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&baseline)
	baselineGoroutines := runtime.NumGoroutine()

	totalOps := 0
	maxMemDelta := int64(0)

	for cycle := 0; cycle < cycles; cycle++ {
		for op := 0; op < opsPerCycle; op++ {
			pattern := longPatterns[op%len(longPatterns)]
			prefix := pattern[op%len(pattern)]
			suggestions := completer.Complete(prefix, 10)
			_ = suggestions
			totalOps++
		}

		if cycle%10 == 0 {
			var m runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m)

			memDelta := int64(m.Alloc - baseline.Alloc)
			goroutineDelta := runtime.NumGoroutine() - baselineGoroutines
			memPerOp := float64(memDelta) / float64(totalOps)

			if memDelta > maxMemDelta {
				maxMemDelta = memDelta
			}

			t.Logf("cycle=%d ops=%d mem_delta=%d bytes mem_per_op=%.2f goroutine_delta=%d",
				cycle, totalOps, memDelta, memPerOp, goroutineDelta)
		}

		if cycle%20 == 0 && cycle > 0 {
			completer.ForceCleanup()
		}

		time.Sleep(5 * time.Millisecond)
	}

	var final runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&final)
	finalGoroutines := runtime.NumGoroutine()

	finalMemDelta := int64(final.Alloc - baseline.Alloc)
	finalGoroutineDelta := finalGoroutines - baselineGoroutines
	finalMemPerOp := float64(finalMemDelta) / float64(totalOps)

	t.Logf("final_summary: cycles=%d total_ops=%d mem_delta=%d bytes mem_per_op=%.2f goroutine_delta=%d max_mem_delta=%d",
		cycles, totalOps, finalMemDelta, finalMemPerOp, finalGoroutineDelta, maxMemDelta)

	if err := pprof.WriteHeapProfile(memFile); err != nil {
		t.Errorf("heap profile write failed: %v", err)
	}

	if finalMemPerOp > 500 {
		t.Errorf("excessive memory usage per operation: %.2f bytes", finalMemPerOp)
	}

	if finalGoroutineDelta > 2 {
		t.Errorf("goroutine leak detected: %d goroutines leaked", finalGoroutineDelta)
	}

	if maxMemDelta > 10*1024*1024 {
		t.Errorf("excessive peak memory usage: %d bytes", maxMemDelta)
	}
}
