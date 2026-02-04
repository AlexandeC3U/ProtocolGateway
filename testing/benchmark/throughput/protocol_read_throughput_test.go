//go:build benchmark
// +build benchmark

// Package throughput provides benchmark tests for protocol read throughput.
package throughput

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Simulated Protocol Client Interface
// =============================================================================

// ProtocolClient simulates a protocol client for throughput testing.
type ProtocolClient interface {
	Read(ctx context.Context, address string) (interface{}, error)
	ReadBatch(ctx context.Context, addresses []string) ([]interface{}, error)
}

// =============================================================================
// Simulated Protocol Implementations
// =============================================================================

// ModbusSimClient simulates Modbus read operations.
type ModbusSimClient struct {
	baseLatency time.Duration
	readCount   atomic.Int64
}

func (c *ModbusSimClient) Read(ctx context.Context, address string) (interface{}, error) {
	time.Sleep(c.baseLatency)
	c.readCount.Add(1)
	return uint16(12345), nil
}

func (c *ModbusSimClient) ReadBatch(ctx context.Context, addresses []string) ([]interface{}, error) {
	// Modbus batch read has fixed overhead + small per-register cost
	latency := c.baseLatency + time.Duration(len(addresses))*2*time.Microsecond
	time.Sleep(latency)
	c.readCount.Add(int64(len(addresses)))

	results := make([]interface{}, len(addresses))
	for i := range addresses {
		results[i] = uint16(12345 + i)
	}
	return results, nil
}

// OPCUASimClient simulates OPC UA read operations.
type OPCUASimClient struct {
	baseLatency time.Duration
	readCount   atomic.Int64
}

func (c *OPCUASimClient) Read(ctx context.Context, address string) (interface{}, error) {
	time.Sleep(c.baseLatency)
	c.readCount.Add(1)
	return float64(123.456), nil
}

func (c *OPCUASimClient) ReadBatch(ctx context.Context, addresses []string) ([]interface{}, error) {
	// OPC UA has higher base overhead but efficient batching
	latency := c.baseLatency + time.Duration(len(addresses))*10*time.Microsecond
	time.Sleep(latency)
	c.readCount.Add(int64(len(addresses)))

	results := make([]interface{}, len(addresses))
	for i := range addresses {
		results[i] = float64(123.456 + float64(i))
	}
	return results, nil
}

// S7SimClient simulates S7 read operations.
type S7SimClient struct {
	baseLatency time.Duration
	readCount   atomic.Int64
}

func (c *S7SimClient) Read(ctx context.Context, address string) (interface{}, error) {
	time.Sleep(c.baseLatency)
	c.readCount.Add(1)
	return int32(12345), nil
}

func (c *S7SimClient) ReadBatch(ctx context.Context, addresses []string) ([]interface{}, error) {
	// S7 has moderate overhead with PDU size limits
	latency := c.baseLatency + time.Duration(len(addresses))*5*time.Microsecond
	time.Sleep(latency)
	c.readCount.Add(int64(len(addresses)))

	results := make([]interface{}, len(addresses))
	for i := range addresses {
		results[i] = int32(12345 + i)
	}
	return results, nil
}

// =============================================================================
// Single Read Throughput Benchmarks
// =============================================================================

// BenchmarkModbusSingleRead benchmarks single Modbus register reads.
func BenchmarkModbusSingleRead(b *testing.B) {
	client := &ModbusSimClient{baseLatency: 500 * time.Microsecond}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = client.Read(ctx, "40001")
	}

	b.ReportMetric(float64(client.readCount.Load())/b.Elapsed().Seconds(), "reads/sec")
}

// BenchmarkOPCUASingleRead benchmarks single OPC UA node reads.
func BenchmarkOPCUASingleRead(b *testing.B) {
	client := &OPCUASimClient{baseLatency: 2 * time.Millisecond}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = client.Read(ctx, "ns=2;s=Tag1")
	}

	b.ReportMetric(float64(client.readCount.Load())/b.Elapsed().Seconds(), "reads/sec")
}

// BenchmarkS7SingleRead benchmarks single S7 data block reads.
func BenchmarkS7SingleRead(b *testing.B) {
	client := &S7SimClient{baseLatency: 1 * time.Millisecond}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = client.Read(ctx, "DB1.DBD0")
	}

	b.ReportMetric(float64(client.readCount.Load())/b.Elapsed().Seconds(), "reads/sec")
}

// =============================================================================
// Batch Read Throughput Benchmarks
// =============================================================================

// BenchmarkModbusBatchRead benchmarks Modbus batch reads.
func BenchmarkModbusBatchRead(b *testing.B) {
	client := &ModbusSimClient{baseLatency: 500 * time.Microsecond}
	ctx := context.Background()

	batchSizes := []int{1, 10, 50, 100}

	for _, size := range batchSizes {
		addresses := make([]string, size)
		for i := range addresses {
			addresses[i] = "40001"
		}

		b.Run(itoa(size)+"tags", func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, _ = client.ReadBatch(ctx, addresses)
			}

			tagsPerSec := float64(size*b.N) / b.Elapsed().Seconds()
			b.ReportMetric(tagsPerSec, "tags/sec")
		})
	}
}

// BenchmarkOPCUABatchRead benchmarks OPC UA batch reads.
func BenchmarkOPCUABatchRead(b *testing.B) {
	client := &OPCUASimClient{baseLatency: 2 * time.Millisecond}
	ctx := context.Background()

	batchSizes := []int{1, 10, 50, 100}

	for _, size := range batchSizes {
		addresses := make([]string, size)
		for i := range addresses {
			addresses[i] = "ns=2;s=Tag1"
		}

		b.Run(itoa(size)+"nodes", func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, _ = client.ReadBatch(ctx, addresses)
			}

			nodesPerSec := float64(size*b.N) / b.Elapsed().Seconds()
			b.ReportMetric(nodesPerSec, "nodes/sec")
		})
	}
}

// BenchmarkS7BatchRead benchmarks S7 batch reads.
func BenchmarkS7BatchRead(b *testing.B) {
	client := &S7SimClient{baseLatency: 1 * time.Millisecond}
	ctx := context.Background()

	batchSizes := []int{1, 10, 50, 100}

	for _, size := range batchSizes {
		addresses := make([]string, size)
		for i := range addresses {
			addresses[i] = "DB1.DBD0"
		}

		b.Run(itoa(size)+"items", func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, _ = client.ReadBatch(ctx, addresses)
			}

			itemsPerSec := float64(size*b.N) / b.Elapsed().Seconds()
			b.ReportMetric(itemsPerSec, "items/sec")
		})
	}
}

// =============================================================================
// Concurrent Read Throughput Benchmarks
// =============================================================================

// BenchmarkModbusConcurrentRead benchmarks concurrent Modbus reads.
func BenchmarkModbusConcurrentRead(b *testing.B) {
	client := &ModbusSimClient{baseLatency: 500 * time.Microsecond}
	ctx := context.Background()

	concurrencies := []int{1, 2, 4, 8}

	for _, c := range concurrencies {
		b.Run(itoa(c)+"workers", func(b *testing.B) {
			b.ReportAllocs()
			b.SetParallelism(c)

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = client.Read(ctx, "40001")
				}
			})

			b.ReportMetric(float64(client.readCount.Load())/b.Elapsed().Seconds(), "reads/sec")
		})
	}
}

// BenchmarkOPCUAConcurrentRead benchmarks concurrent OPC UA reads.
func BenchmarkOPCUAConcurrentRead(b *testing.B) {
	client := &OPCUASimClient{baseLatency: 2 * time.Millisecond}
	ctx := context.Background()

	concurrencies := []int{1, 2, 4, 8}

	for _, c := range concurrencies {
		b.Run(itoa(c)+"workers", func(b *testing.B) {
			b.ReportAllocs()
			b.SetParallelism(c)

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = client.Read(ctx, "ns=2;s=Tag1")
				}
			})

			b.ReportMetric(float64(client.readCount.Load())/b.Elapsed().Seconds(), "reads/sec")
		})
	}
}

// =============================================================================
// Protocol Comparison Benchmarks
// =============================================================================

// BenchmarkProtocolThroughput_Comparison compares throughput across protocols.
func BenchmarkProtocolThroughput_Comparison(b *testing.B) {
	ctx := context.Background()

	protocols := []struct {
		name   string
		client ProtocolClient
	}{
		{"Modbus", &ModbusSimClient{baseLatency: 500 * time.Microsecond}},
		{"OPC_UA", &OPCUASimClient{baseLatency: 2 * time.Millisecond}},
		{"S7", &S7SimClient{baseLatency: 1 * time.Millisecond}},
	}

	for _, proto := range protocols {
		b.Run(proto.name+"_Single", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = proto.client.Read(ctx, "addr")
			}
		})

		b.Run(proto.name+"_Batch50", func(b *testing.B) {
			addresses := make([]string, 50)
			for i := range addresses {
				addresses[i] = "addr"
			}

			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = proto.client.ReadBatch(ctx, addresses)
			}
		})
	}
}

// =============================================================================
// Sustained Throughput Tests
// =============================================================================

// TestSustainedThroughput_1Second tests sustained throughput over 1 second.
func TestSustainedThroughput_1Second(t *testing.T) {
	duration := 1 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	protocols := []struct {
		name   string
		client ProtocolClient
	}{
		{"Modbus", &ModbusSimClient{baseLatency: 500 * time.Microsecond}},
		{"OPC_UA", &OPCUASimClient{baseLatency: 2 * time.Millisecond}},
		{"S7", &S7SimClient{baseLatency: 1 * time.Millisecond}},
	}

	for _, proto := range protocols {
		t.Run(proto.name, func(t *testing.T) {
			var count atomic.Int64
			start := time.Now()

			for {
				select {
				case <-ctx.Done():
					elapsed := time.Since(start)
					throughput := float64(count.Load()) / elapsed.Seconds()
					t.Logf("%s: %d reads in %v (%.0f reads/sec)",
						proto.name, count.Load(), elapsed, throughput)
					return
				default:
					_, _ = proto.client.Read(ctx, "addr")
					count.Add(1)
				}
			}
		})
	}
}

// TestSustainedThroughput_Concurrent tests sustained concurrent throughput.
func TestSustainedThroughput_Concurrent(t *testing.T) {
	duration := 1 * time.Second
	workers := 4

	client := &ModbusSimClient{baseLatency: 500 * time.Microsecond}

	var count atomic.Int64
	var wg sync.WaitGroup

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					_, _ = client.Read(ctx, "40001")
					count.Add(1)
				}
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)
	throughput := float64(count.Load()) / elapsed.Seconds()

	t.Logf("Concurrent Modbus (%d workers): %d reads in %v (%.0f reads/sec)",
		workers, count.Load(), elapsed, throughput)
}

// =============================================================================
// Helper Functions
// =============================================================================

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
