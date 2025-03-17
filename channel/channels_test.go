package channel_test

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jkaveri/goflux/channel"
)

func TestJoinReads(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		inputs   [][]int
		expected []int
	}{
		{
			name:     "empty channels",
			inputs:   [][]int{{}, {}},
			expected: []int(nil),
		},
		{
			name:     "single value per channel",
			inputs:   [][]int{{1}, {2}},
			expected: []int{1, 2},
		},
		{
			name:     "multiple values per channel",
			inputs:   [][]int{{1, 3}, {2, 4}},
			expected: []int{1, 2, 3, 4},
		},
		{
			name:     "nil channels slice",
			inputs:   nil,
			expected: []int(nil),
		},
		{
			name:     "large number of channels",
			inputs:   make([][]int, 100), // 100 empty channels
			expected: []int(nil),
		},
		{
			name: "uneven channel sizes",
			inputs: [][]int{
				{1, 2, 3, 4, 5},  // 5 elements
				{6},              // 1 element
				{7, 8},           // 2 elements
				{9, 10, 11},      // 3 elements
				{12, 13, 14, 15}, // 4 elements
			},
			expected: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channels := make([]chan int, len(tt.inputs))
			for i := range channels {
				channels[i] = make(chan int)
			}

			// Start goroutine to send values
			for i, values := range tt.inputs {
				go func(ch chan int, vals []int) {
					for _, v := range vals {
						ch <- v
					}
					close(ch)
				}(channels[i], values)
			}

			// Join channels
			merged := channel.JoinReads(channels...)

			// Collect results
			var result []int
			for v := range merged {
				result = append(result, v)
			}

			// Sort results for deterministic comparison
			sort.Ints(result)
			sort.Ints(tt.expected)

			assert.Equal(t, len(tt.expected), len(result), "result length mismatch")
			assert.Equal(t, tt.expected, result, "result values mismatch")
		})
	}
}

func TestJoinWrites(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		value     int
		numChans  int
		expectVal int
		testClose bool
	}{
		{
			name:      "single channel",
			value:     42,
			numChans:  1,
			expectVal: 42,
			testClose: false,
		},
		{
			name:      "multiple channels",
			value:     100,
			numChans:  3,
			expectVal: 100,
			testClose: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			channels := make([]chan int, tt.numChans)
			for i := range channels {
				channels[i] = make(chan int)
			}

			writer := channel.JoinWrites(channels...)

			var wg sync.WaitGroup
			var mu sync.Mutex // Add mutex for synchronizing access to received slice
			wg.Add(tt.numChans)

			received := make([]int, 0, tt.numChans)

			// Start readers
			for _, ch := range channels {
				go func(c chan int) {
					defer wg.Done()
					v := <-c
					mu.Lock() // Lock before modifying shared slice
					received = append(received, v)
					mu.Unlock() // Unlock after modifying
				}(ch)
			}

			// Write value
			writer <- tt.value

			// Wait for all readers
			wg.Wait()

			assert.Equal(t, tt.numChans, len(received), "number of received values mismatch")

			close(writer)

			time.Sleep(10 * time.Millisecond)

			for _, c := range channels {
				select {
				case <-c:
				default:
					assert.Fail(t, "channel should be closed")
				}
			}
		})
	}
}

func TestSplit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      []int
		numOutputs int
	}{
		{
			name:       "split to 2 channels",
			input:      []int{1, 2, 3},
			numOutputs: 2,
		},
		{
			name:       "split to 3 channels",
			input:      []int{42},
			numOutputs: 3,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := make(chan int)
			outputs := channel.Split(input, tt.numOutputs)

			assert.Equal(t, tt.numOutputs, len(outputs), "number of output channels mismatch")

			// Start goroutine to send input values
			go func() {
				for _, v := range tt.input {
					input <- v
				}
				close(input)
			}()

			// Collect results from each output channel
			var wg sync.WaitGroup
			var mu sync.Mutex
			wg.Add(tt.numOutputs)
			results := make([][]int, tt.numOutputs)

			for i, ch := range outputs {
				i := i // Capture loop variable
				ch := ch
				go func() {
					defer wg.Done()
					var received []int
					for v := range ch {
						received = append(received, v)
					}
					mu.Lock()
					results[i] = received
					mu.Unlock()
				}()
			}

			wg.Wait()

			// Verify each output channel received all values
			for i, received := range results {
				assert.Equal(t, len(tt.input), len(received),
					fmt.Sprintf("output channel %d: received values length mismatch", i))
				assert.Equal(t, tt.input, received,
					fmt.Sprintf("output channel %d: received values mismatch", i))
			}
		})
	}
}

func TestTee(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []int
		expected []int
	}{
		{
			name:     "single value",
			input:    []int{42},
			expected: []int{42},
		},
		{
			name:     "multiple values",
			input:    []int{1, 2, 3, 4, 5},
			expected: []int{1, 2, 3, 4, 5},
		},
		{
			name:     "empty input",
			input:    []int{},
			expected: []int{},
		},
		{
			name:     "large numbers",
			input:    []int{1000000, -1000000},
			expected: []int{1000000, -1000000},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := make(chan int)
			out1, out2 := channel.Tee(input)

			// Start goroutine to send input values
			go func() {
				for _, v := range tt.input {
					input <- v
				}
				close(input)
			}()

			// Test both output channels receive all values
			var wg sync.WaitGroup
			var mu sync.Mutex
			wg.Add(2)

			results := make([][]int, 2)

			// Read from first output channel
			go func() {
				defer wg.Done()
				received := make([]int, 0, len(tt.expected))
				for v := range out1 {
					received = append(received, v)
				}
				mu.Lock()
				results[0] = received
				mu.Unlock()
			}()

			// Read from second output channel
			go func() {
				defer wg.Done()
				received := make([]int, 0, len(tt.expected))
				for v := range out2 {
					received = append(received, v)
				}
				mu.Lock()
				results[1] = received
				mu.Unlock()
			}()

			wg.Wait()

			// Verify both channels received all values in order
			for i, result := range results {
				assert.Equal(t, len(tt.expected), len(result), fmt.Sprintf("output channel %d: received values length mismatch", i+1))
				assert.Equal(t, tt.expected, result, fmt.Sprintf("output channel %d: received values mismatch", i+1))
			}
		})
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     []int
		predicate func(int) bool
		expected  []int
	}{
		{
			name:  "filter even numbers",
			input: []int{1, 2, 3, 4, 5, 6},
			predicate: func(n int) bool {
				return n%2 == 0
			},
			expected: []int{2, 4, 6},
		},
		{
			name:  "filter positive numbers",
			input: []int{-2, -1, 0, 1, 2},
			predicate: func(n int) bool {
				return n > 0
			},
			expected: []int{1, 2},
		},
		{
			name:  "filter none",
			input: []int{1, 2, 3},
			predicate: func(n int) bool {
				return false
			},
			expected: []int{},
		},
		{
			name:  "filter all",
			input: []int{1, 2, 3},
			predicate: func(n int) bool {
				return true
			},
			expected: []int{1, 2, 3},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := make(chan int)
			filtered := channel.Filter(input, tt.predicate)

			go func() {
				for _, v := range tt.input {
					input <- v
				}
				close(input)
			}()

			result := make([]int, 0)
			for v := range filtered {
				result = append(result, v)
			}

			assert.Equal(t, len(tt.expected), len(result), "filtered result length mismatch")
			assert.Equal(t, tt.expected, result, "filtered values mismatch")
		})
	}
}

func TestMap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     []int
		transform func(int) string
		expected  []string
	}{
		{
			name:  "convert to strings",
			input: []int{1, 2, 3},
			transform: func(n int) string {
				return fmt.Sprintf("value-%d", n)
			},
			expected: []string{"value-1", "value-2", "value-3"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := make(chan int)
			mapped := channel.Map(input, tt.transform)

			go func() {
				for _, v := range tt.input {
					input <- v
				}
				close(input)
			}()

			var result []string
			for v := range mapped {
				result = append(result, v)
			}

			assert.Equal(t, len(tt.expected), len(result), "mapped result length mismatch")
			assert.Equal(t, tt.expected, result, "mapped values mismatch")
		})
	}
}

func TestBatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     []int
		batchSize int
		expected  [][]int
	}{
		{
			name:      "batch size 2",
			input:     []int{1, 2, 3, 4, 5},
			batchSize: 2,
			expected:  [][]int{{1, 2}, {3, 4}, {5}},
		},
		{
			name:      "batch size equals length",
			input:     []int{1, 2, 3},
			batchSize: 3,
			expected:  [][]int{{1, 2, 3}},
		},
		{
			name:      "zero batch size",
			input:     []int{1, 2, 3},
			batchSize: 0,
			expected:  [][]int{{1}, {2}, {3}},
		},
		{
			name:      "negative batch size",
			input:     []int{1, 2, 3},
			batchSize: -1,
			expected:  [][]int{{1}, {2}, {3}},
		},
		{
			name:      "batch size larger than input",
			input:     []int{1, 2, 3},
			batchSize: 10,
			expected:  [][]int{{1, 2, 3}},
		},
		{
			name:      "empty input",
			input:     []int{},
			batchSize: 2,
			expected:  [][]int{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := make(chan int)
			batched := channel.Batch(input, tt.batchSize)

			go func() {
				for _, v := range tt.input {
					input <- v
				}
				close(input)
			}()

			result := make([][]int, 0)
			for batch := range batched {
				result = append(result, batch)
			}

			assert.Equal(t, len(tt.expected), len(result), "number of batches mismatch")
			for i := range result {
				assert.Equal(t, len(tt.expected[i]), len(result[i]), "batch %d size mismatch", i)
				assert.Equal(t, tt.expected[i], result[i], "batch %d values mismatch", i)
			}
		})
	}
}

func TestReduce(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []int
		initial  int
		reducer  func(int, int) int
		expected int
	}{
		{
			name:    "sum numbers",
			input:   []int{1, 2, 3, 4, 5},
			initial: 0,
			reducer: func(acc, v int) int {
				return acc + v
			},
			expected: 15,
		},
		{
			name:    "multiply numbers",
			input:   []int{1, 2, 3, 4},
			initial: 1,
			reducer: func(acc, v int) int {
				return acc * v
			},
			expected: 24,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := make(chan int)
			reduced := channel.Reduce(input, tt.initial, tt.reducer)

			go func() {
				for _, v := range tt.input {
					input <- v
				}
				close(input)
			}()

			result := <-reduced
			assert.Equal(t, tt.expected, result, "reduced value mismatch")
		})
	}
}

func TestWithTimeout(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		sendAfter    time.Duration
		timeout      time.Duration
		defaultValue int
		expectValue  int
		shouldPanic  bool
	}{
		{
			name:         "receive before timeout",
			sendAfter:    50 * time.Millisecond,
			timeout:      100 * time.Millisecond,
			defaultValue: -1,
			expectValue:  42,
			shouldPanic:  false,
		},
		{
			name:         "timeout occurs",
			sendAfter:    100 * time.Millisecond,
			timeout:      50 * time.Millisecond,
			defaultValue: -1,
			expectValue:  -1,
			shouldPanic:  false,
		},
		{
			name:         "zero timeout",
			sendAfter:    0,
			timeout:      0,
			defaultValue: -1,
			expectValue:  42,
			shouldPanic:  true,
		},
		{
			name:         "negative timeout",
			sendAfter:    10 * time.Millisecond,
			timeout:      -1 * time.Second,
			defaultValue: -1,
			expectValue:  -1,
			shouldPanic:  true,
		},
		{
			name:         "very long timeout",
			sendAfter:    10 * time.Millisecond,
			timeout:      24 * time.Hour,
			defaultValue: -1,
			expectValue:  42,
			shouldPanic:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := make(chan int)

			if tt.shouldPanic {
				assert.Panics(t, func() {
					_ = channel.WithTimeout(input, tt.timeout, tt.defaultValue)
				})

				return
			}

			timeout := channel.WithTimeout(input, tt.timeout, tt.defaultValue)

			go func() {
				time.Sleep(tt.sendAfter)
				input <- 42
				close(input)
			}()

			result := <-timeout
			assert.Equal(t, tt.expectValue, result, "timeout result mismatch")
		})
	}
}

func TestRateLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []int
		interval time.Duration
	}{
		{
			name:     "basic rate limiting",
			input:    []int{1, 2, 3},
			interval: 50 * time.Millisecond,
		},
		{
			name:     "empty input",
			input:    []int{},
			interval: 10 * time.Millisecond,
		},
		{
			name:     "single value",
			input:    []int{42},
			interval: 10 * time.Millisecond,
		},
		{
			name:     "longer sequence",
			input:    []int{1, 2, 3, 4, 5},
			interval: 20 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := make(chan int)

			// Create rate-limited channel
			limited := channel.RateLimit(input, tt.interval)

			// Collect results and measure intervals
			resultChan := make(chan int, len(tt.input))
			intervalChan := make(chan time.Duration, len(tt.input))

			go func() {
				var lastTime time.Time

				for v := range limited {
					now := time.Now()
					if !lastTime.IsZero() {
						interval := now.Sub(lastTime)
						intervalChan <- interval
					}
					lastTime = now
					resultChan <- v
				}

				close(resultChan)
				close(intervalChan)
			}()

			// Start goroutine to send input values
			go func() {
				for _, v := range tt.input {
					input <- v
				}
				close(input)
			}()

			// Verify results
			result := make([]int, 0)
			for v := range resultChan {
				result = append(result, v)
			}

			intervals := make([]time.Duration, 0)
			for interval := range intervalChan {
				intervals = append(intervals, interval)
			}

			assert.ElementsMatch(t, tt.input, result, "values should be received in order")

			// Verify rate limiting for tests with multiple values
			if len(intervals) > 0 {
				// Allow for some timing variance in the test environment
				allowedVariance := tt.interval / 2
				for _, interval := range intervals {
					assert.InDelta(t, tt.interval.Milliseconds(), interval.Milliseconds(),
						float64(allowedVariance.Milliseconds()),
						"intervals between values should approximately match specified interval")
				}
			}
		})
	}
}
