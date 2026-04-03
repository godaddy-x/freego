package snowflake

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestInitialZeroTime 验证初始化时间为0的安全性
func TestInitialZeroTime(t *testing.T) {
	node, _ := NewNode(1)
	// 首次生成应成功，且时间戳 > Epoch
	id := node.Generate()
	if id.Time() <= Epoch {
		t.Errorf("ID time %d <= Epoch %d", id.Time(), Epoch)
	}
}

// TestMaxSequenceOverflow 验证4096个ID后正确等待
func TestMaxSequenceOverflow(t *testing.T) {
	node, _ := NewNode(1)
	baseTime := node.GetNow()
	node.time = baseTime
	node.step = 4095 // 最后一个序列号

	// 生成第4096个ID（step=0，触发等待）
	id := node.Generate()
	if node.time <= baseTime {
		t.Errorf("time did not advance after sequence overflow")
	}
	if id.Step() != 0 {
		t.Errorf("expected step=0 after overflow, got %d", id.Step())
	}
}

// TestConcurrentGenerateInt64For5s 并发生成 int64 ID（持续5秒）并校验重复
func TestConcurrentGenerateInt64For5s(t *testing.T) {
	node, err := NewNode(1)
	if err != nil {
		t.Fatalf("new node failed: %v", err)
	}

	const workers = 8
	var total int64
	var duplicate int64

	// 按毫秒时间戳记录 step 位图（每个时间戳最多4096个step）
	// bitmap[ms][idx] 的每一位表示某个 step 是否出现过
	seen := make(map[int64][64]uint64, 8192)
	var seenMu sync.Mutex

	start := time.Now()
	stopAt := start.Add(5 * time.Second)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for time.Now().Before(stopAt) {
				gen := node.Generate()
				id := gen.Int64()
				if id <= 0 {
					t.Errorf("invalid id: %d", id)
					return
				}

				ts := gen.Time()
				step := gen.Step()
				word := step / 64
				bit := uint(step % 64)
				mask := uint64(1) << bit

				seenMu.Lock()
				bm := seen[ts]
				if bm[word]&mask != 0 {
					atomic.AddInt64(&duplicate, 1)
				} else {
					bm[word] |= mask
					seen[ts] = bm
				}
				seenMu.Unlock()

				atomic.AddInt64(&total, 1)
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	totalCount := atomic.LoadInt64(&total)
	dupCount := atomic.LoadInt64(&duplicate)
	qps := float64(totalCount) / elapsed.Seconds()
	t.Logf("concurrent generate done: workers=%d elapsed=%s total=%d duplicate=%d qps=%.2f id/s", workers, elapsed.Round(time.Millisecond), totalCount, dupCount, qps)

	if dupCount > 0 {
		t.Fatalf("duplicate ids detected: %d", dupCount)
	}
}
