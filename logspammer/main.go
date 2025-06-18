package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"
)

var (
	logRate   = flag.Int("rate", 1000, "Logs per second to generate")
	logSize   = flag.Int("size", 100, "Size of each log line in bytes")
	useStderr = flag.Bool("stderr", false, "Write to stderr instead of stdout")
)

func main() {
	flag.Parse()

	// Use stderr if requested, otherwise stdout
	output := os.Stdout
	if *useStderr {
		output = os.Stderr
	}

	// Create a logger that writes to the selected output
	logger := log.New(output, "", 0)

	// Calculate sleep time between logs
	sleepTime := time.Second / time.Duration(*logRate)

	// Generate random bytes for log content
	rand.Seed(time.Now().UnixNano())
	logContent := make([]byte, *logSize)
	for i := range logContent {
		logContent[i] = byte(rand.Intn(26) + 97) // Random lowercase letters
	}

	// Log counter
	counter := 0
	startTime := time.Now()

	// Main loop
	for {
		counter++
		logger.Printf("Log line %d: %s", counter, string(logContent))

		// Print stats every second
		if counter%*logRate == 0 {
			elapsed := time.Since(startTime)
			rate := float64(counter) / elapsed.Seconds()
			fmt.Fprintf(os.Stderr, "Stats: Generated %d logs in %.2fs (%.2f logs/sec)\n",
				counter, elapsed.Seconds(), rate)
		}

		time.Sleep(sleepTime)
	}
}
