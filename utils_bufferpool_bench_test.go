package httpcache

import (
	"bytes"
	"testing"
)

// BenchmarkBufferPoolVsNew compares buffer pool performance with direct allocation
func BenchmarkBufferPoolVsNew(b *testing.B) {
	b.Run("WithPool", func(b *testing.B) {
		testData := []byte("test data for benchmark")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := getBuffer()
			buf.Write(testData)
			_ = buf.Bytes()
			putBuffer(buf)
		}
	})

	b.Run("WithoutPool", func(b *testing.B) {
		testData := []byte("test data for benchmark")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := new(bytes.Buffer)
			buf.Write(testData)
			_ = buf.Bytes()
		}
	})

	b.Run("WithBytesNewBuffer", func(b *testing.B) {
		testData := []byte("test data for benchmark")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := bytes.NewBuffer(testData)
			_ = buf.Bytes()
		}
	})
}

// BenchmarkBufferPoolSizes benchmarks buffer pool with different data sizes
func BenchmarkBufferPoolSizes(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"100B", 100},
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"32KB", 32 * 1024},
		{"64KB", 64 * 1024}, // At the pool limit
	}

	for _, s := range sizes {
		data := make([]byte, s.size)
		for i := range data {
			data[i] = byte(i % 256)
		}

		b.Run(s.name+"_WithPool", func(b *testing.B) {
			b.SetBytes(int64(s.size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buf := getBuffer()
				buf.Write(data)
				_ = buf.Bytes()
				putBuffer(buf)
			}
		})

		b.Run(s.name+"_WithoutPool", func(b *testing.B) {
			b.SetBytes(int64(s.size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buf := new(bytes.Buffer)
				buf.Write(data)
				_ = buf.Bytes()
			}
		})
	}
}

// BenchmarkBufferPoolConcurrent benchmarks concurrent buffer pool usage
func BenchmarkBufferPoolConcurrent(b *testing.B) {
	testData := []byte("concurrent benchmark test data")

	b.Run("WithPool", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				buf := getBuffer()
				buf.Write(testData)
				_ = buf.Bytes()
				putBuffer(buf)
			}
		})
	})

	b.Run("WithoutPool", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				buf := new(bytes.Buffer)
				buf.Write(testData)
				_ = buf.Bytes()
			}
		})
	})
}

// BenchmarkBufferPoolRealWorldScenario simulates a real-world cache scenario
func BenchmarkBufferPoolRealWorldScenario(b *testing.B) {
	// Simulate a typical HTTP response (small JSON payload)
	responseData := []byte(`{"status":"success","data":{"id":123,"name":"test","items":[1,2,3,4,5]}}`)

	b.Run("CachedResponseWithPool", func(b *testing.B) {
		b.SetBytes(int64(len(responseData)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := getBuffer()
			buf.Write(responseData)
			// Simulate reading from buffer (like http.ReadResponse does)
			_ = buf.Bytes()
			putBuffer(buf)
		}
	})

	b.Run("CachedResponseWithoutPool", func(b *testing.B) {
		b.SetBytes(int64(len(responseData)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := new(bytes.Buffer)
			buf.Write(responseData)
			_ = buf.Bytes()
		}
	})
}

// BenchmarkBufferPoolMultipleOperations tests multiple write operations
func BenchmarkBufferPoolMultipleOperations(b *testing.B) {
	chunks := [][]byte{
		[]byte("HTTP/1.1 200 OK\r\n"),
		[]byte("Content-Type: application/json\r\n"),
		[]byte("Content-Length: 42\r\n"),
		[]byte("\r\n"),
		[]byte(`{"status":"ok"}`),
	}

	b.Run("WithPool", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := getBuffer()
			for _, chunk := range chunks {
				buf.Write(chunk)
			}
			_ = buf.Bytes()
			putBuffer(buf)
		}
	})

	b.Run("WithoutPool", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := new(bytes.Buffer)
			for _, chunk := range chunks {
				buf.Write(chunk)
			}
			_ = buf.Bytes()
		}
	})
}

// BenchmarkBufferPoolMemoryAllocation benchmarks memory allocation patterns
func BenchmarkBufferPoolMemoryAllocation(b *testing.B) {
	data := make([]byte, 4096) // 4KB typical page size

	b.Run("WithPool", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := getBuffer()
			buf.Write(data)
			putBuffer(buf)
		}
	})

	b.Run("WithoutPool", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := new(bytes.Buffer)
			buf.Write(data)
		}
	})
}

// BenchmarkNewGatewayTimeoutResponse benchmarks the gateway timeout response creation
func BenchmarkNewGatewayTimeoutResponse(b *testing.B) {
	// This benchmarks the actual usage in newGatewayTimeoutResponse
	b.Run("WithPool", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := getBuffer()
			buf.WriteString("HTTP/1.1 504 Gateway Timeout\r\n\r\n")
			_ = buf.Bytes()
			putBuffer(buf)
		}
	})

	b.Run("WithoutPool", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var buf bytes.Buffer
			buf.WriteString("HTTP/1.1 504 Gateway Timeout\r\n\r\n")
			_ = buf.Bytes()
		}
	})
}
