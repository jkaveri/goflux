package channel

import (
	"sync"
	"time"
)

// JoinReads combines multiple read-only channels of type T into a single channel.
// It is similar to Joins but specifically designed to work with read-only channels (<-chan T).
// The function reads from all input channels concurrently and forwards all received values
// to the returned channel. The returned channel is closed only after all input
// channels have been closed and fully drained.
//
// Example:
//
//	ch1 := make(chan int)
//	ch2 := make(chan int)
//	merged := JoinReads(ch1, ch2)
//
//	// Now merged will receive values from both ch1 and ch2
//	go func() {
//		ch1 <- 1
//		ch2 <- 2
//		close(ch1)
//		close(ch2)
//	}()
//
//	for v := range merged {
//		fmt.Println(v) // Will print "1" and "2" in any order
//	}
//
// Parameters:
//   - channels: Variable number of read-only channels of type T
//
// Returns:
//   - A channel of type T that receives all values from input channels
func JoinReads[T any](channels ...chan T) <-chan T {
	out := make(chan T, len(channels))

	go func() {
		var wg sync.WaitGroup

		wg.Add(len(channels))

		for _, c := range channels {
			go func(c chan T) {
				defer wg.Done()

				for v := range c {
					out <- v
				}
			}(c)
		}

		// Wait until all channels are drained, then close output.
		go func() {
			wg.Wait()
			close(out)
		}()
	}()

	return out
}

// JoinWrites combines multiple write-only channels of type T into a single write-only channel.
// Values written to the returned channel will be forwarded to all output channels concurrently.
// The returned channel will be closed when all output channels are closed or if any write operation fails.
//
// Example:
//
//	ch1 := make(chan int)
//	ch2 := make(chan int)
//	writer := JoinWrites(ch1, ch2)
//
//	// Both ch1 and ch2 will receive the value
//	writer <- 42
//
//	// Reading from both channels
//	go func() {
//		fmt.Println(<-ch1) // Will print 42
//		fmt.Println(<-ch2) // Will print 42
//	}()
//
// Parameters:
//   - channels: Variable number of write-only channels of type T
//
// Returns:
//   - A write-only channel of type T that forwards values to all output channels
func JoinWrites[T any](channels ...chan T) chan<- T {
	input := make(chan T, len(channels))
	closeChan := make(chan struct{})

	go func() {
		for v := range input {
			// Fan-out to all output channels
			for _, ch := range channels {
				go func(c chan T) {
					select {
					case c <- v:
					case <-closeChan:
					}
				}(ch)
			}
		}

		// Close all output channels when input is closed
		close(closeChan)

		for _, ch := range channels {
			close(ch)
		}
	}()

	return input
}

// Joins combines multiple channels of type T into a single read-write channel.
// The returned channel can be used to both read and write values.
//
// Parameters:
//   - channels: Variable number of channels of type T
//
// Returns:
//   - A read-write channel of type T that can be used to both read and write values
func Joins[T any](channels ...chan T) (read <-chan T, write chan<- T) {
	return JoinReads(channels...), JoinWrites(channels...)
}

// Split creates n new channels and forwards values from the input channel to all of them.
// The function returns a slice of n read-only channels.
// All channels are closed when the input channel is closed.
//
// Example:
//
//	input := make(chan int)
//	outputs := Split(input, 3)
//
//	go func() {
//		input <- 42
//		close(input)
//	}()
//
//	// All output channels will receive 42
//	for _, ch := range outputs {
//		fmt.Println(<-ch) // Will print "42" three times
//	}
//
// Parameters:
//   - input: The source channel
//   - n: Number of output channels to create
//
// Returns:
//   - A slice of n read-only channels that each receive all values
func Split[T any](input chan T, n int) []<-chan T {
	outputs := make([]chan T, n)
	for i := 0; i < n; i++ {
		outputs[i] = make(chan T)
	}

	go func() {
		defer func() {
			for _, ch := range outputs {
				close(ch)
			}
		}()

		for v := range input {
			for _, ch := range outputs {
				ch <- v
			}
		}
	}()

	// Convert to read-only channels
	result := make([]<-chan T, n)
	for i, ch := range outputs {
		result[i] = ch
	}

	return result
}

// Tee duplicates the input channel into two identical output channels.
// Both output channels will receive the same values.
// Both channels are closed when the input channel is closed.
//
// Example:
//
//	input := make(chan int)
//	out1, out2 := Tee(input)
//
//	go func() {
//		input <- 42
//		close(input)
//	}()
//
//	// Both channels receive the same value
//	fmt.Println(<-out1) // Will print "42"
//	fmt.Println(<-out2) // Will print "42"
//
// Real-world Example: Processing payments while logging
//
//	type Payment struct {
//		ID     string
//		Amount float64
//	}
//
//	// Channel for incoming payments
//	payments := make(chan Payment)
//
//	// Split into processing and logging streams
//	processStream, logStream := Tee(payments)
//
//	// Process payments
//	go func() {
//		for payment := range processStream {
//			processPayment(payment)  // Handle the payment
//		}
//	}()
//
//	// Log all payments for audit
//	go func() {
//		for payment := range logStream {
//			logToAuditSystem(payment)  // Log for compliance
//		}
//	}()
//
//	// Send a payment
//	payments <- Payment{ID: "PAY-123", Amount: 100.00}
//
// Parameters:
//   - input: The source channel
//
// Returns:
//   - Two read-only channels that each receive all values
func Tee[T any](input chan T) (<-chan T, <-chan T) {
	out1 := make(chan T)
	out2 := make(chan T)

	go func() {
		defer func() {
			close(out1)
			close(out2)
		}()

		for v := range input {
			out1 <- v
			out2 <- v
		}
	}()

	return out1, out2
}

// Filter creates a new channel that only forwards values that satisfy the given predicate.
// The output channel is closed when the input channel is closed.
//
// Example: Filtering valid transactions
//
//	type Transaction struct {
//		ID     string
//		Amount float64
//		Status string
//	}
//
//	// Channel receiving all transactions
//	transactions := make(chan Transaction)
//
//	// Filter only completed transactions with amount > 100
//	validTransactions := Filter(transactions, func(t Transaction) bool {
//		return t.Status == "completed" && t.Amount > 100
//	})
//
//	go func() {
//		transactions <- Transaction{ID: "1", Amount: 50, Status: "completed"}
//		transactions <- Transaction{ID: "2", Amount: 150, Status: "completed"}
//		transactions <- Transaction{ID: "3", Amount: 200, Status: "failed"}
//		close(transactions)
//	}()
//
//	for t := range validTransactions {
//		// Only processes Transaction{ID: "2", Amount: 150, Status: "completed"}
//		processTransaction(t)
//	}
//
// Parameters:
//   - input: The source channel
//   - predicate: Function that returns true for values that should be included
//
// Returns:
//   - A channel that only receives values that satisfy the predicate
func Filter[T any](input chan T, predicate func(T) bool) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		for v := range input {
			if predicate(v) {
				out <- v
			}
		}
	}()

	return out
}

// Map transforms values from the input channel using the given transform function.
// The output channel is closed when the input channel is closed.
//
// Example: Converting API responses to domain models
//
//	type APIUser struct { ID string; Name string }
//	type User struct { ID int; DisplayName string }
//
//	// Channel receiving API responses
//	apiUsers := make(chan APIUser)
//
//	// Transform API users to domain users
//	users := Map(apiUsers, func(api APIUser) User {
//		return User{
//			ID: parseInt(api.ID),
//			DisplayName: strings.Title(api.Name),
//		}
//	})
//
//	go func() {
//		apiUsers <- APIUser{ID: "1", Name: "john doe"}
//		apiUsers <- APIUser{ID: "2", Name: "jane smith"}
//		close(apiUsers)
//	}()
//
//	for user := range users {
//		fmt.Printf("User: %+v\n") // User: {ID:1 DisplayName:"John Doe"}
//	}
//
// Parameters:
//   - input: The source channel
//   - transform: Function that transforms values of type T to type R
//
// Returns:
//   - A channel that receives transformed values
func Map[T any, R any](input chan T, transform func(T) R) <-chan R {
	out := make(chan R)

	go func() {
		defer close(out)

		for v := range input {
			out <- transform(v)
		}
	}()

	return out
}

// WithTimeout wraps a channel with a timeout for read operations.
// If the timeout is reached before a value is available, the default value is returned.
//
// Example:
//
//	input := make(chan int)
//	timeout := WithTimeout(input, time.Second, -1)
//
//	go func() {
//		time.Sleep(2 * time.Second)
//		input <- 42
//		close(input)
//	}()
//
//	fmt.Println(<-timeout) // Will print "-1" after 1 second
//	fmt.Println(<-timeout) // Will print "42" when ready
//
// Parameters:
//   - input: The source channel
//   - timeout: Duration to wait before returning default value
//   - defaultValue: Value to return when timeout occurs
//
// Returns:
//   - A channel that implements timeout behavior
func WithTimeout[T any](input chan T, timeout time.Duration, defaultValue T) <-chan T {
	if timeout <= 0 {
		panic("timeout must be greater than 0")
	}

	out := make(chan T)

	go func() {
		defer close(out)

		for {
			select {
			case v, ok := <-input:
				if !ok {
					return
				}
				out <- v
			case <-time.After(timeout):
				out <- defaultValue
			}
		}
	}()

	return out
}

// Batch collects values from the input channel into slices of the specified size.
// The last batch may contain fewer elements if the channel is closed before filling a complete batch.
//
// Example: Processing orders in batches for bulk database insertion
//
//	type Order struct {
//		ID      string
//		UserID  string
//		Amount  float64
//	}
//
//	// Channel receiving individual orders
//	orders := make(chan Order)
//
//	// Process orders in batches of 10 for bulk DB insertion
//	orderBatches := Batch(orders, 10)
//
//	go func() {
//		// Simulate orders coming in
//		for i := 0; i < 25; i++ {
//			orders <- Order{
//				ID:     fmt.Sprintf("ord_%d", i),
//				UserID: fmt.Sprintf("user_%d", i%5),
//				Amount: float64(i * 10),
//			}
//		}
//		close(orders)
//	}()
//
//	for batch := range orderBatches {
//		// Each batch will contain up to 10 orders
//		db.BulkInsert(batch) // Insert 10 orders at once
//	}
//
// Parameters:
//   - input: The source channel
//   - batchSize: The size of each batch
//
// Returns:
//   - A channel that receives slices of values
func Batch[T any](input chan T, batchSize int) <-chan []T {
	if batchSize <= 0 {
		batchSize = 1
	}

	out := make(chan []T)

	go func() {
		defer close(out)

		batch := make([]T, 0, batchSize)

		for v := range input {
			batch = append(batch, v)
			if len(batch) >= batchSize {
				out <- batch
				batch = make([]T, 0, batchSize)
			}
		}

		if len(batch) > 0 {
			out <- batch
		}
	}()

	return out
}

// RateLimit creates a new channel that forwards values from the input channel
// at a specified rate defined by the interval between operations.
//
// Example: Rate limiting API requests
//
//	type APIRequest struct {
//		URL     string
//		Method  string
//		Payload []byte
//	}
//
//	// Channel of API requests to be processed
//	requests := make(chan APIRequest)
//
//	// Limit to one request every 200ms (5 requests per second)
//	limitedRequests := RateLimit(requests, 200*time.Millisecond)
//
//	go func() {
//		// Simulate burst of requests
//		for i := 0; i < 20; i++ {
//			requests <- APIRequest{
//				URL:    fmt.Sprintf("/api/resource/%d", i),
//				Method: "POST",
//			}
//		}
//		close(requests)
//	}()
//
//	for req := range limitedRequests {
//		// Requests will be processed at most 5 times per second
//		response := httpClient.Do(req)
//		processResponse(response)
//	}
//
// Parameters:
//   - input: The source channel
//   - interval: The minimum time interval between operations
//
// Returns:
//   - A rate-limited channel
func RateLimit[T any](input chan T, interval time.Duration) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for v := range input {
			<-ticker.C // Wait for next tick
			out <- v
		}
	}()

	return out
}

// Reduce applies a reducing function to all values from the input channel
// and sends the final result to the output channel when the input channel is closed.
//
// Example: Calculating total revenue from multiple payment sources
//
//	type Payment struct {
//		Source string  // e.g., "stripe", "paypal"
//		Amount float64
//		Currency string
//	}
//
//	type Revenue struct {
//		TotalUSD     float64
//		PaymentCount int
//	}
//
//	// Channel receiving payments from different sources
//	payments := make(chan Payment)
//
//	// Calculate total revenue and payment count
//	revenue := Reduce(payments, Revenue{}, func(acc Revenue, p Payment) Revenue {
//		return Revenue{
//			TotalUSD:     acc.TotalUSD + convertToUSD(p.Amount, p.Currency),
//			PaymentCount: acc.PaymentCount + 1,
//		}
//	})
//
//	go func() {
//		payments <- Payment{Source: "stripe", Amount: 100, Currency: "USD"}
//		payments <- Payment{Source: "paypal", Amount: 85, Currency: "EUR"}
//		payments <- Payment{Source: "stripe", Amount: 200, Currency: "USD"}
//		close(payments)
//	}()
//
//	result := <-revenue
//	fmt.Printf("Total Revenue: $%.2f from %d payments\n",
//		result.TotalUSD, result.PaymentCount)
//
// Parameters:
//   - input: The source channel
//   - initial: The initial value for the reduction
//   - reducer: Function that combines the accumulator with each value
//
// Returns:
//   - A channel that receives the final reduced value
func Reduce[T any, R any](input chan T, initial R, reducer func(R, T) R) <-chan R {
	out := make(chan R, 1) // Buffer of 1 to hold the final result

	go func() {
		defer close(out)

		result := initial
		for v := range input {
			result = reducer(result, v)
		}
		out <- result
	}()

	return out
}
