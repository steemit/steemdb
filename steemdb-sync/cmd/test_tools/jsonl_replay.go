package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func runJsonlReplay(args []string) {
	fs := flag.NewFlagSet("jsonl_replay", flag.ExitOnError)
	var (
		jsonlFile   = fs.String("file", "", "Path to JSONL file (required)")
		endpoint    = fs.String("endpoint", "http://localhost:8080/ingest/applied_ops", "Ingest endpoint URL (batch endpoint)")
		batchSize   = fs.Int("batch-size", 100, "Number of operations per batch (0 = use single endpoint)")
		rate        = fs.Int("rate", 0, "Operations per second (0 = no limit)")
		startBlock  = fs.Uint("start-block", 0, "Start from block number (0 = from beginning)")
		endBlock    = fs.Uint("end-block", 0, "End at block number (0 = to end)")
		verbose     = fs.Bool("verbose", false, "Verbose output")
		showErrors  = fs.Bool("show-errors", true, "Show HTTP errors")
	)
	fs.Parse(args)

	if *jsonlFile == "" {
		fs.Usage()
		log.Fatal("Error: -file is required")
	}

	// Open file
	file, err := os.Open(*jsonlFile)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	// Create HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Rate limiting
	var ticker *time.Ticker
	if *rate > 0 {
		ticker = time.NewTicker(time.Second / time.Duration(*rate))
		defer ticker.Stop()
	}

	// Statistics
	var (
		totalSent    int
		totalSuccess int
		totalErrors  int
		lastBlock    uint32
		startTime    = time.Now()
	)

	// Progress reporting
	progressTicker := time.NewTicker(5 * time.Second)
	defer progressTicker.Stop()
	go func() {
		for range progressTicker.C {
			if totalSent > 0 {
				elapsed := time.Since(startTime)
				opsPerSec := float64(totalSent) / elapsed.Seconds()
				log.Printf("Progress: sent=%d, success=%d, errors=%d, last_block=%d, ops/sec=%.2f",
					totalSent, totalSuccess, totalErrors, lastBlock, opsPerSec)
			}
		}
	}()

	// Determine if we should use batch endpoint
	useBatch := *batchSize > 0 && strings.Contains(*endpoint, "/applied_ops")

	// For batch mode, collect operations
	batch := make([]map[string]interface{}, 0, *batchSize)

	// For single mode, use the original endpoint or fallback to single endpoint
	singleEndpoint := *endpoint
	if useBatch && strings.Contains(*endpoint, "/applied_ops") {
		// Keep batch endpoint
	} else if *batchSize > 0 {
		// User wants batch but endpoint is single, switch to batch endpoint
		singleEndpoint = strings.Replace(*endpoint, "/applied_op", "/applied_ops", 1)
		useBatch = true
	} else {
		// Use single endpoint
		useBatch = false
	}

	// Read file line by line
	scanner := bufio.NewScanner(file)
	lineNum := 0

	sendBatch := func(ops []map[string]interface{}) {
		if len(ops) == 0 {
			return
		}

		// Rate limiting for batch
		if ticker != nil {
			<-ticker.C
		}

		jsonData, err := json.Marshal(ops)
		if err != nil {
			log.Printf("Error marshaling batch: %v", err)
			totalErrors += len(ops)
			return
		}

		req, err := http.NewRequest("POST", singleEndpoint, strings.NewReader(string(jsonData)))
		if err != nil {
			log.Printf("Error creating request: %v", err)
			totalErrors += len(ops)
			return
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			if *showErrors {
				log.Printf("Error sending batch (%d operations): %v", len(ops), err)
			}
			totalErrors += len(ops)
			return
		}

		// Read and parse response
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if *showErrors {
				log.Printf("HTTP error for batch (%d operations): status %d", len(ops), resp.StatusCode)
			}
			totalErrors += len(ops)
		} else {
			// Parse batch response
			var batchResp map[string]interface{}
			if err := json.Unmarshal(body, &batchResp); err == nil {
				if processed, ok := batchResp["processed"].(float64); ok {
					processedInt := int(processed)
					totalSuccess += processedInt
					totalErrors += len(ops) - processedInt

					if status, ok := batchResp["status"].(string); ok && status == "partial" {
						if errors, ok := batchResp["errors"].([]interface{}); ok {
							if *showErrors && len(errors) > 0 {
								log.Printf("Batch had %d partial failures", len(errors))
							}
						}
					}
				} else {
					totalSuccess += len(ops)
				}
			} else {
				totalSuccess += len(ops)
			}
		}

		totalSent += len(ops)
	}

	sendSingle := func(opJSON map[string]interface{}) {
		// Rate limiting
		if ticker != nil {
			<-ticker.C
		}

		jsonData, err := json.Marshal(opJSON)
		if err != nil {
			log.Printf("Error marshaling operation: %v", err)
			totalErrors++
			return
		}

		// Use single endpoint (fallback to /applied_op if batch endpoint was specified)
		singleURL := strings.Replace(*endpoint, "/applied_ops", "/applied_op", 1)

		req, err := http.NewRequest("POST", singleURL, strings.NewReader(string(jsonData)))
		if err != nil {
			log.Printf("Error creating request: %v", err)
			totalErrors++
			return
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			if *showErrors {
				blockNum, _ := opJSON["block"].(map[string]interface{})["num"].(float64)
				log.Printf("Error sending request (block %d): %v", uint32(blockNum), err)
			}
			totalErrors++
			return
		}

		// Read response body (to avoid connection leaks)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if *showErrors {
				blockNum, _ := opJSON["block"].(map[string]interface{})["num"].(float64)
				log.Printf("HTTP error (block %d): status %d", uint32(blockNum), resp.StatusCode)
			}
			totalErrors++
		} else {
			totalSuccess++
			if block, ok := opJSON["block"].(map[string]interface{}); ok {
				if blockNum, ok := block["num"].(float64); ok {
					lastBlock = uint32(blockNum)
				}
			}
		}

		totalSent++
	}

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Parse JSON
		var opJSON map[string]interface{}
		if err := json.Unmarshal([]byte(line), &opJSON); err != nil {
			if *verbose {
				log.Printf("Warning: Failed to parse line %d: %v", lineNum, err)
			}
			continue
		}

		// Extract block number
		block, ok := opJSON["block"].(map[string]interface{})
		if !ok {
			if *verbose {
				log.Printf("Warning: Invalid block format at line %d", lineNum)
			}
			continue
		}

		blockNum, ok := block["num"].(float64)
		if !ok {
			if *verbose {
				log.Printf("Warning: Invalid block number at line %d", lineNum)
			}
			continue
		}

		blockNumUint := uint32(blockNum)

		// Check block range filters
		if *startBlock > 0 && blockNumUint < uint32(*startBlock) {
			continue
		}
		if *endBlock > 0 && blockNumUint > uint32(*endBlock) {
			break // Stop if we've passed the end block
		}

		if useBatch {
			// Add to batch
			batch = append(batch, opJSON)

			// Send batch when it reaches batchSize
			if len(batch) >= *batchSize {
				sendBatch(batch)
				batch = batch[:0] // Reset batch
			}
		} else {
			// Send single operation
			sendSingle(opJSON)
		}

		// Verbose output
		if *verbose && totalSent%1000 == 0 {
			log.Printf("Sent %d operations, last block: %d", totalSent, lastBlock)
		}
	}

	// Send remaining batch
	if useBatch && len(batch) > 0 {
		sendBatch(batch)
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading file: %v", err)
	}

	// Final statistics
	elapsed := time.Since(startTime)
	opsPerSec := float64(totalSent) / elapsed.Seconds()

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total operations sent: %d\n", totalSent)
	fmt.Printf("Successful: %d\n", totalSuccess)
	fmt.Printf("Errors: %d\n", totalErrors)
	fmt.Printf("Last block: %d\n", lastBlock)
	fmt.Printf("Time elapsed: %v\n", elapsed.Round(time.Second))
	fmt.Printf("Average rate: %.2f ops/sec\n", opsPerSec)
}
