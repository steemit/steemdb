package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"net/http"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TestColdIngestE2E tests the complete cold ingest workflow:
// 1. Start MongoDB test container
// 2. Start cold_ingest service
// 3. Start steemd container with ingest plugin
// 4. Wait for target height to be reached
// 5. Verify data written to MongoDB
// 6. Verify cold_ingest exits correctly
// 7. Verify meta collection is updated
func TestColdIngestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Create a context that can be cancelled for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal (Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		t.Logf("Received interrupt signal, cleaning up...")
		cancel()
	}()

	// Check if steemd image exists
	steemdImage := "steemd:with-ingest"
	if !dockerImageExists(t, steemdImage) {
		t.Skipf("Docker image %s not found, skipping E2E test", steemdImage)
	}

	// Test configuration
	testDB := "steemdb_e2e_test"
	targetHeight := uint32(1000) // Small target for testing
	safetyMargin := uint32(5)

	// Setup MongoDB test container
	mongoURI := getTestMongoURI()
	mongoClient := setupMongoTest(t, mongoURI, testDB)
	defer cleanupMongoTest(t, mongoClient, testDB)

	// Create temporary config file
	configPath := createTestConfig(t, mongoURI, testDB, targetHeight, safetyMargin)
	defer os.Remove(configPath)

	// Check and clean port 8080 if needed
	checkAndCleanPort(t, 8080)

	// Find cold_ingest binary
	coldIngestBin := findColdIngestBinary(t)

	// Start cold_ingest service
	ingestCmd := exec.Command(coldIngestBin, "-config", configPath)
	ingestCmd.Stdout = os.Stdout
	ingestCmd.Stderr = os.Stderr

	err := ingestCmd.Start()
	require.NoError(t, err, "Failed to start cold_ingest")

	// Ensure process is killed on test failure or cancellation
	defer func() {
		if ingestCmd.Process != nil {
			t.Logf("Killing cold_ingest process...")
			ingestCmd.Process.Kill()
			ingestCmd.Wait()
		}
	}()

	// Give the server a moment to start listening (MongoDB connection may take time)
	time.Sleep(2 * time.Second)

	// Wait for HTTP server to be ready (check batch endpoint)
	waitForHTTPServer(t, "http://localhost:8080/ingest/applied_ops", 30*time.Second)

	// Check if steemd container is already running
	// If not, automatically start it using the helper script
	steemdContainerID := checkSteemdContainer(t, steemdImage)
	containerStartedByTest := false
	if steemdContainerID == "" {
		// Try to find and run steem-test script
		testDir, _ := os.Getwd()
		steemTestDir := findSteemTestDir(testDir)

		if steemTestDir == "" {
			t.Fatalf("steemd container not found and steem-test script directory not found.\n"+
				"Please ensure test/steem-test/run.sh exists, or start the container manually:\n"+
				"  docker run -d --name steemd-ingest-test \\\n"+
				"    --add-host host.docker.internal:host-gateway \\\n"+
				"    %s \\\n"+
				"    /usr/local/steemd/bin/steemd \\\n"+
				"      --replay-blockchain \\\n"+
				"      --plugin ingest \\\n"+
				"      --ingest-endpoint http://host.docker.internal:8080/ingest/applied_ops \\\n"+
				"      --data-dir /var/steem", steemdImage)
		}

		// Start container using the helper script
		t.Logf("steemd container not found. Starting container using script: %s/run.sh", steemTestDir)
		runScript := filepath.Join(steemTestDir, "run.sh")

		// Set environment variables for the script
		startCmd := exec.Command("bash", runScript)
		startCmd.Env = append(os.Environ(),
			"STEEMD_IMAGE="+steemdImage,
			"INGEST_ENDPOINT=http://host.docker.internal:8080/ingest/applied_ops",
		)
		startCmd.Dir = steemTestDir
		startCmd.Stdout = os.Stdout
		startCmd.Stderr = os.Stderr

		err = startCmd.Run()
		if err != nil {
			t.Fatalf("Failed to start steemd container using script: %v", err)
		}

		containerStartedByTest = true
		t.Logf("Container start command executed. Waiting for container to appear...")

		// Wait for container to appear (with timeout)
		waitTimeout := 30 * time.Second
		waitDeadline := time.Now().Add(waitTimeout)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		containerFound := false
		for time.Now().Before(waitDeadline) && !containerFound {
			select {
			case <-ctx.Done():
				t.Fatal("Test cancelled while waiting for container")
			case <-ticker.C:
				steemdContainerID = checkSteemdContainer(t, steemdImage)
				if steemdContainerID != "" {
					containerFound = true
				}
			default:
				time.Sleep(500 * time.Millisecond)
			}
		}

		if steemdContainerID == "" {
			t.Fatalf("Container did not appear within %v after starting script", waitTimeout)
		}

		t.Logf("Container started successfully: %s", steemdContainerID)
	} else {
		t.Logf("Using existing steemd container: %s", steemdContainerID)
	}

	// Optionally stop container when test completes (if we started it)
	// Note: We don't stop it by default to allow inspection after test
	if containerStartedByTest {
		// Check if user wants to keep the container (via environment variable)
		keepContainer := os.Getenv("KEEP_STEEMD_CONTAINER") == "true"
		if !keepContainer {
			defer func() {
				testDir, _ := os.Getwd()
				steemTestDir := findSteemTestDir(testDir)
				if steemTestDir != "" {
					t.Logf("Stopping steemd container...")
					stopScript := filepath.Join(steemTestDir, "stop.sh")
					stopCmd := exec.Command("bash", stopScript)
					stopCmd.Dir = steemTestDir
					stopCmd.Stdout = os.Stdout
					stopCmd.Stderr = os.Stderr
					if err := stopCmd.Run(); err != nil {
						t.Logf("Warning: Failed to stop container: %v", err)
					}
				}
			}()
		}
	}

	// Wait for steemd to initialize (rebuild block_log.index if needed)
	// Typically takes about 1 minute for steemd container to start and begin replay
	// Use interruptible sleep so test can be cancelled with Ctrl+C
	t.Logf("Waiting for steemd to initialize (rebuilding block_log.index if needed)...")
	initDuration := 1 * time.Minute
	initTicker := time.NewTicker(10 * time.Second)
	defer initTicker.Stop()

	initDeadline := time.Now().Add(initDuration)
	for time.Now().Before(initDeadline) {
		select {
		case <-ctx.Done():
			t.Logf("Test cancelled, stopping initialization wait")
			return
		case <-initTicker.C:
			remaining := time.Until(initDeadline)
			t.Logf("Still waiting for steemd initialization... (remaining: %v)", remaining.Round(time.Second))
		default:
			time.Sleep(1 * time.Second)
		}
	}
	t.Logf("Steemd initialization wait complete. Replay should be starting now.")

	// Wait for cold_ingest to complete (it should exit when target height is reached)
	done := make(chan error, 1)
	go func() {
		done <- ingestCmd.Wait()
	}()

	// Monitor progress while waiting
	progressTicker := time.NewTicker(30 * time.Second)
	defer progressTicker.Stop()

	// Wait for completion with timeout
	// Note: Total timeout includes:
	// - steemd initialization (1 minute for container startup and index rebuild)
	// - replay time (varies by block_log size and target height)
	// - safety margin
	// For 1000 blocks, replay typically takes 10-20 minutes, so 60 minutes total should be sufficient
	timeout := 60 * time.Minute
	deadline := time.Now().Add(timeout)

	completed := false
	var ingestErr error

	for !completed {
		select {
		case ingestErr = <-done:
			completed = true
		case <-ctx.Done():
			t.Fatal("Test cancelled by user (Ctrl+C)")
		case <-progressTicker.C:
			// Check progress periodically
			remaining := time.Until(deadline)
			t.Logf("Still waiting for cold_ingest to complete... (remaining: %v, target: %d)",
				remaining.Round(time.Minute), targetHeight)

			// Check if we're receiving data by querying MongoDB
			var metaDoc bson.M
			metaColl := mongoClient.Database(testDB).Collection("meta")
			err := metaColl.FindOne(context.Background(), bson.M{"_id": "sync_state"}).Decode(&metaDoc)
			if err == nil {
				if maxBlock, ok := metaDoc["max_block"]; ok {
					t.Logf("Current max_block in database: %v", maxBlock)
				}
			}

			// Check operation count
			opsColl := mongoClient.Database(testDB).Collection("operations")
			opCount, err := opsColl.CountDocuments(context.Background(), bson.M{})
			if err == nil {
				t.Logf("Total operations in database: %d", opCount)
			}

			// Check blocks count
			blocksColl := mongoClient.Database(testDB).Collection("blocks")
			blockCount, err := blocksColl.CountDocuments(context.Background(), bson.M{})
			if err == nil {
				t.Logf("Total blocks in database: %d", blockCount)
			}

			// Check steemd container status and logs for errors
			if steemdContainerID != "" {
				// Check if container is still running
				checkCmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("id=%s", steemdContainerID), "--format", "{{.Status}}")
				if status, err := checkCmd.Output(); err == nil {
					statusStr := strings.TrimSpace(string(status))
					if statusStr == "" {
						t.Logf("WARNING: steemd container %s is not running!", steemdContainerID)
					} else {
						t.Logf("steemd container status: %s", statusStr)
					}
				}

				// Check for HTTP errors in steemd logs (last 50 lines)
				logsCmd := exec.Command("docker", "logs", "--tail", "50", steemdContainerID)
				if logs, err := logsCmd.Output(); err == nil {
					logsStr := string(logs)
					// Check for various error patterns
					if strings.Contains(logsStr, "HTTP send error") ||
						strings.Contains(logsStr, "HTTP error") ||
						strings.Contains(logsStr, "ingest") ||
						strings.Contains(logsStr, "plugin") {
						t.Logf("Recent steemd logs (last 50 lines):")
						t.Logf("%s", logsStr)
					}

					// Check if ingest plugin is loaded
					if strings.Contains(logsStr, "ingest plugin") || strings.Contains(logsStr, "Ingest plugin") {
						t.Logf("Ingest plugin appears to be loaded")
					} else {
						t.Logf("WARNING: No ingest plugin messages found in logs")
					}

					// Check if replay has started
					if strings.Contains(logsStr, "replay") || strings.Contains(logsStr, "Replay") {
						t.Logf("Replay appears to be in progress")
					}
				}
			}

			// Check if we're past the deadline
			if time.Now().After(deadline) {
				t.Fatalf("cold_ingest did not complete within timeout (%v). "+
					"This includes steemd initialization time. If replay is still in progress, "+
					"you may need to increase the timeout or check steemd logs. "+
					"Check container logs with: docker logs steemd-ingest-test",
					timeout)
			}
		case <-time.After(time.Until(deadline)):
			// Final timeout check
			t.Fatalf("cold_ingest did not complete within timeout (%v). "+
				"This includes steemd initialization time. If replay is still in progress, "+
				"you may need to increase the timeout or check steemd logs. "+
				"Check container logs with: docker logs steemd-ingest-test",
				timeout)
		}
	}

	// Check if cold_ingest exited successfully
	require.NoError(t, ingestErr, "cold_ingest should exit successfully")

	// Verify data in MongoDB
	verifyColdIngestData(t, mongoClient, testDB, targetHeight, safetyMargin)

	// Verify meta collection
	verifyMetaCollection(t, mongoClient, testDB, targetHeight, safetyMargin)
}

// TestColdIngestWithMockPlugin tests cold ingest with a mock plugin that sends operation JSON
// This is a faster alternative to the full E2E test
func TestColdIngestWithMockPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Test configuration
	testDB := "steemdb_e2e_mock_test"
	targetHeight := uint32(100) // Small target for testing
	safetyMargin := uint32(5)

	// Setup MongoDB test container
	mongoURI := getTestMongoURI()
	mongoClient := setupMongoTest(t, mongoURI, testDB)
	defer cleanupMongoTest(t, mongoClient, testDB)

	// Create temporary config file
	configPath := createTestConfig(t, mongoURI, testDB, targetHeight, safetyMargin)
	defer os.Remove(configPath)

	// Check and clean port 8080 if needed
	checkAndCleanPort(t, 8080)

	// Find cold_ingest binary
	coldIngestBin := findColdIngestBinary(t)

	// Start cold_ingest service
	ingestCmd := exec.Command(coldIngestBin, "-config", configPath)
	ingestCmd.Stdout = os.Stdout
	ingestCmd.Stderr = os.Stderr

	err := ingestCmd.Start()
	require.NoError(t, err, "Failed to start cold_ingest")

	// Ensure process is killed on test failure
	defer func() {
		if ingestCmd.Process != nil {
			ingestCmd.Process.Kill()
			ingestCmd.Wait()
		}
	}()

	// Give the server a moment to start listening (MongoDB connection may take time)
	time.Sleep(2 * time.Second)

	// Wait for HTTP server to be ready (check batch endpoint)
	waitForHTTPServer(t, "http://localhost:8080/ingest/applied_ops", 30*time.Second)

	// Send mock operations using batch endpoint
	go sendMockOperationsBatch(t, targetHeight)

	// Wait for cold_ingest to complete
	done := make(chan error, 1)
	go func() {
		done <- ingestCmd.Wait()
	}()

	// Wait for completion with timeout (5 minutes)
	select {
	case err := <-done:
		require.NoError(t, err, "cold_ingest should exit successfully")
	case <-time.After(5 * time.Minute):
		t.Fatal("cold_ingest did not complete within timeout")
	}

	// Verify data in MongoDB
	verifyColdIngestData(t, mongoClient, testDB, targetHeight, safetyMargin)

	// Verify meta collection
	verifyMetaCollection(t, mongoClient, testDB, targetHeight, safetyMargin)
}

// Helper functions

func getTestMongoURI() string {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		uri = "mongodb://admin:123456@127.0.0.1:27017/steemdb_test?authSource=admin"
	}
	return uri
}

func setupMongoTest(t *testing.T, uri, database string) *mongo.Client {
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err, "Failed to connect to MongoDB")

	// Ping to verify connection
	err = client.Ping(ctx, nil)
	require.NoError(t, err, "Failed to ping MongoDB")

	// Clean up test database
	db := client.Database(database)
	err = db.Drop(ctx)
	if err != nil && !strings.Contains(err.Error(), "ns not found") {
		t.Logf("Warning: Failed to drop test database: %v", err)
	}

	return client
}

func cleanupMongoTest(t *testing.T, client *mongo.Client, database string) {
	ctx := context.Background()
	if client != nil {
		db := client.Database(database)
		db.Drop(ctx)
		client.Disconnect(ctx)
	}
}

func createTestConfig(t *testing.T, mongoURI, database string, targetHeight, safetyMargin uint32) string {
	// Create temporary config file
	tmpFile, err := os.CreateTemp("", "test_config_*.yaml")
	require.NoError(t, err, "Failed to create temp config file")

	// Write config as YAML
	cfgYAML := fmt.Sprintf(`mongo:
  uri: "%s"
  database: "%s"
  min_pool_size: 10
  max_pool_size: 100
rpc:
  endpoint: "https://api.steemit.com"
  max_retry: 3
  timeout: "30s"
cold_start:
  target_height: %d
  safety_margin: %d
batch:
  size: 1000
  flush_interval: "1s"
ingest:
  listen_addr: ":8080"
  queue_size: 100000
log:
  level: "info"
  format: "text"
`, mongoURI, database, targetHeight, safetyMargin)

	_, err = tmpFile.WriteString(cfgYAML)
	require.NoError(t, err, "Failed to write config file")
	tmpFile.Close()

	return tmpFile.Name()
}

func findColdIngestBinary(t *testing.T) string {
	// Get the project root (steemdb-sync directory)
	testDir, err := os.Getwd()
	require.NoError(t, err, "Failed to get current directory")

	// Navigate to steemdb-sync root from test/e2e
	projectRoot := filepath.Join(testDir, "..", "..")
	projectRoot, err = filepath.Abs(projectRoot)
	require.NoError(t, err, "Failed to resolve project root")

	// Try to find cold_ingest binary in common locations
	// Priority: steemdb/bin/ > project root > relative paths > GOPATH
	binDir := filepath.Join(projectRoot, "..", "bin")
	possiblePaths := []string{
		filepath.Join(binDir, "cold_ingest"),      // steemdb/bin/cold_ingest (preferred)
		filepath.Join(projectRoot, "cold_ingest"), // steemdb-sync/cold_ingest (legacy)
		"./cold_ingest",
		"../cold_ingest",
		"../../cold_ingest",
		"../../../cold_ingest",
		filepath.Join(os.Getenv("GOPATH"), "bin", "cold_ingest"),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			absPath, err := filepath.Abs(path)
			if err == nil {
				return absPath
			}
		}
	}

	// Try to build it to steemdb/bin/
	os.MkdirAll(binDir, 0755)
	binaryPath := filepath.Join(binDir, "cold_ingest")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/cold_ingest")
	buildCmd.Dir = projectRoot
	if err := buildCmd.Run(); err == nil {
		return binaryPath
	}

	t.Fatal("Could not find or build cold_ingest binary")
	return ""
}

func waitForHTTPServer(t *testing.T, url string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	attempt := 0
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		attempt++

		// Try GET request first (will return 405 for POST-only endpoints)
		resp, err := client.Get(url)
		if err == nil {
			statusCode := resp.StatusCode
			resp.Body.Close()
			// 405 Method Not Allowed is OK for GET (endpoint only accepts POST)
			// 400 Bad Request is also OK (means server is up, just needs POST with body)
			if statusCode == 200 || statusCode == 405 || statusCode == 400 {
				if attempt > 1 {
					t.Logf("HTTP server is ready at %s (GET status: %d, attempt: %d)", url, statusCode, attempt)
				}
				return
			}
		} else if attempt%20 == 0 {
			// Log connection errors every 20 attempts for debugging
			t.Logf("GET connection error while checking %s (attempt %d): %v", url, attempt, err)
		}

		// Also try a minimal POST request to verify the endpoint works
		// For batch endpoint, send empty array (will return 400, but server is up)
		// For single endpoint, send empty object (will return 400, but server is up)
		var testBody string
		if strings.Contains(url, "/applied_ops") {
			testBody = "[]" // Empty array for batch endpoint
		} else {
			testBody = "{}" // Empty object for single endpoint
		}

		postResp, err := client.Post(url, "application/json", strings.NewReader(testBody))
		if err == nil {
			statusCode := postResp.StatusCode
			postResp.Body.Close()
			// 400 is expected for empty/invalid requests, means server is up
			if statusCode == 200 || statusCode == 400 {
				if attempt > 1 {
					t.Logf("HTTP server is ready at %s (POST status: %d, attempt: %d)", url, statusCode, attempt)
				}
				return
			}
		} else if attempt%20 == 0 {
			// Log connection errors every 20 attempts for debugging
			t.Logf("POST connection error while checking %s (attempt %d): %v", url, attempt, err)
		}

		<-ticker.C
	}
	t.Fatalf("HTTP server did not become ready at %s within timeout (%v) after %d attempts", url, timeout, attempt)
}

func dockerImageExists(t *testing.T, image string) bool {
	cmd := exec.Command("docker", "images", "-q", image)
	output, err := cmd.Output()
	return err == nil && len(strings.TrimSpace(string(output))) > 0
}

// checkSteemdContainer checks if a steemd container is currently running
// Returns container ID if found, empty string otherwise
func checkSteemdContainer(t *testing.T, image string) string {
	// Check for containers with common names
	containerNames := []string{
		"steemd-ingest-test",
		"steemd",
		"steemd-test",
	}

	for _, name := range containerNames {
		// Check if container exists and is running
		cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", name), "--format", "{{.ID}}")
		output, err := cmd.Output()
		if err == nil {
			containerID := strings.TrimSpace(string(output))
			if containerID != "" {
				return containerID
			}
		}
	}

	// Also check by image name
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("ancestor=%s", image), "--format", "{{.ID}}")
	output, err := cmd.Output()
	if err == nil {
		containerID := strings.TrimSpace(string(output))
		if containerID != "" {
			return containerID
		}
	}

	return ""
}

// waitForSteemdContainer waits for a steemd container to appear (user should start it manually)
// Returns container ID if found within timeout, empty string otherwise
func waitForSteemdContainer(t *testing.T, image string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		containerID := checkSteemdContainer(t, image)
		if containerID != "" {
			return containerID
		}

		select {
		case <-ticker.C:
			// Continue waiting
		case <-time.After(time.Until(deadline)):
			return ""
		}
	}

	return ""
}

func stopSteemdContainer(t *testing.T, containerID string) {
	if containerID == "" {
		return
	}

	// Stop container (with timeout)
	stopCmd := exec.Command("docker", "stop", "-t", "10", containerID)
	if err := stopCmd.Run(); err != nil {
		t.Logf("Warning: Failed to stop container %s: %v", containerID, err)
	}

	// Remove container
	rmCmd := exec.Command("docker", "rm", containerID)
	if err := rmCmd.Run(); err != nil {
		t.Logf("Warning: Failed to remove container %s: %v", containerID, err)
	}
}

// sendMockOperationsBatch sends operations using the batch endpoint
func sendMockOperationsBatch(t *testing.T, targetHeight uint32) {
	ingestURL := "http://localhost:8080/ingest/applied_ops"

	// Batch size: send operations in batches of 10
	batchSize := 10
	batch := make([]map[string]interface{}, 0, batchSize)

	// Generate and send mock operations
	for blockNum := uint32(1); blockNum <= targetHeight; blockNum++ {
		// Generate a few operations per block
		for opIndex := 0; opIndex < 3; opIndex++ {
			opJSON := generateMockOperation(blockNum, opIndex)
			batch = append(batch, opJSON)

			// Send batch when it reaches batchSize
			if len(batch) >= batchSize {
				sendBatchOperations(t, ingestURL, batch)
				batch = batch[:0] // Reset batch
			}
		}
	}

	// Send remaining operations
	if len(batch) > 0 {
		sendBatchOperations(t, ingestURL, batch)
	}
}

// sendMockOperations sends operations one by one (legacy, kept for compatibility)
func sendMockOperations(t *testing.T, targetHeight uint32) {
	ingestURL := "http://localhost:8080/ingest/applied_op"

	// Generate and send mock operations
	for blockNum := uint32(1); blockNum <= targetHeight; blockNum++ {
		// Send a few operations per block
		for opIndex := 0; opIndex < 3; opIndex++ {
			opJSON := generateMockOperation(blockNum, opIndex)
			sendOperationJSON(t, ingestURL, opJSON)
		}

		// Small delay to avoid overwhelming the server
		time.Sleep(10 * time.Millisecond)
	}
}

func generateMockOperation(blockNum uint32, opIndex int) map[string]interface{} {
	return map[string]interface{}{
		"block": map[string]interface{}{
			"num":       blockNum,
			"id":        fmt.Sprintf("000000%08x", blockNum),
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
		"transaction": map[string]interface{}{
			"id":    fmt.Sprintf("trx_%d_%d", blockNum, opIndex),
			"index": opIndex,
		},
		"operation": map[string]interface{}{
			"index": opIndex,
			"type":  "transfer",
			"value": map[string]interface{}{
				"from":   "alice",
				"to":     "bob",
				"amount": "1.000 STEEM",
				"memo":   fmt.Sprintf("test operation %d", opIndex),
			},
		},
		"virtual": false,
	}
}

// sendBatchOperations sends a batch of operations to the batch endpoint
func sendBatchOperations(t *testing.T, url string, operations []map[string]interface{}) {
	if len(operations) == 0 {
		return
	}

	jsonData, err := json.Marshal(operations)
	require.NoError(t, err, "Failed to marshal batch JSON")

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	require.NoError(t, err, "Failed to create HTTP request")

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("Warning: Failed to send batch (%d operations): %v", len(operations), err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Warning: HTTP %d for batch (%d operations): %s", resp.StatusCode, len(operations), string(body))
		return
	}

	// Parse response to check for partial failures
	var batchResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err == nil {
		if status, ok := batchResp["status"].(string); ok && status == "partial" {
			if errors, ok := batchResp["errors"].([]interface{}); ok && len(errors) > 0 {
				t.Logf("Warning: Batch had %d partial failures out of %d operations", len(errors), len(operations))
			}
		}
	}
}

// sendOperationJSON sends a single operation (legacy, kept for compatibility)
func sendOperationJSON(t *testing.T, url string, opJSON map[string]interface{}) {
	jsonData, err := json.Marshal(opJSON)
	require.NoError(t, err, "Failed to marshal operation JSON")

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	require.NoError(t, err, "Failed to create HTTP request")

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Logf("Warning: Failed to send operation: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Warning: HTTP %d: %s", resp.StatusCode, string(body))
	}
}

func verifyColdIngestData(t *testing.T, client *mongo.Client, database string, targetHeight, safetyMargin uint32) {
	ctx := context.Background()
	db := client.Database(database)

	// Verify blocks collection
	blocksColl := db.Collection("blocks")
	blockCount, err := blocksColl.CountDocuments(ctx, bson.M{})
	require.NoError(t, err, "Failed to count blocks")
	assert.Greater(t, blockCount, int64(0), "Should have blocks in database")

	// Verify operations collection
	opsColl := db.Collection("operations")
	opCount, err := opsColl.CountDocuments(ctx, bson.M{})
	require.NoError(t, err, "Failed to count operations")
	assert.Greater(t, opCount, int64(0), "Should have operations in database")

	// Verify max block is set correctly
	var metaDoc bson.M
	metaColl := db.Collection("meta")
	err = metaColl.FindOne(ctx, bson.M{"_id": "sync_state"}).Decode(&metaDoc)
	require.NoError(t, err, "Failed to find meta document")

	maxBlock, ok := metaDoc["max_block"].(int32)
	if !ok {
		maxBlock64, ok := metaDoc["max_block"].(int64)
		require.True(t, ok, "max_block should be a number")
		maxBlock = int32(maxBlock64)
	}

	expectedMaxBlock := int32(targetHeight - safetyMargin)
	assert.GreaterOrEqual(t, maxBlock, expectedMaxBlock, "max_block should be >= target_height - safety_margin")

	// Verify cold_start_done is set
	coldStartDone, ok := metaDoc["cold_start_done"].(bool)
	require.True(t, ok, "cold_start_done should be a boolean")
	assert.True(t, coldStartDone, "cold_start_done should be true")
}

func verifyMetaCollection(t *testing.T, client *mongo.Client, database string, targetHeight, safetyMargin uint32) {
	ctx := context.Background()
	db := client.Database(database)
	metaColl := db.Collection("meta")

	var metaDoc bson.M
	err := metaColl.FindOne(ctx, bson.M{"_id": "sync_state"}).Decode(&metaDoc)
	require.NoError(t, err, "Failed to find meta document")

	// Verify required fields
	assert.Contains(t, metaDoc, "max_block", "meta should have max_block field")
	assert.Contains(t, metaDoc, "cold_start_done", "meta should have cold_start_done field")
	assert.Contains(t, metaDoc, "updated_at", "meta should have updated_at field")

	// Verify cold_start_done is true
	coldStartDone, ok := metaDoc["cold_start_done"].(bool)
	require.True(t, ok, "cold_start_done should be a boolean")
	assert.True(t, coldStartDone, "cold_start_done should be true after cold start")
}

// checkAndCleanPort checks if a port is in use and attempts to clean it up
// This helps avoid "address already in use" errors from previous test runs
func checkAndCleanPort(t *testing.T, port int) {
	// Try to find process using the port using lsof (Linux/macOS)
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port))
	output, err := cmd.Output()
	if err != nil {
		// Port is not in use, or lsof is not available
		return
	}

	pids := strings.Fields(strings.TrimSpace(string(output)))
	if len(pids) == 0 {
		return
	}

	t.Logf("Port %d is in use by process(es): %v. Attempting to clean up...", port, pids)

	// Try to kill processes using the port
	for _, pid := range pids {
		killCmd := exec.Command("kill", "-9", pid)
		if err := killCmd.Run(); err != nil {
			t.Logf("Warning: Failed to kill process %s: %v", pid, err)
		} else {
			t.Logf("Killed process %s using port %d", pid, port)
		}
	}

	// Wait a bit for the port to be released
	time.Sleep(2 * time.Second)

	// Verify port is now free
	cmd = exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port))
	output, err = cmd.Output()
	if err == nil && len(strings.TrimSpace(string(output))) > 0 {
		t.Logf("Warning: Port %d is still in use after cleanup attempt", port)
	} else {
		t.Logf("Port %d is now free", port)
	}
}

// findSteemTestDir searches for the steem-test directory containing helper scripts
// Returns the absolute path if found, empty string otherwise
func findSteemTestDir(startDir string) string {
	// Try to find test/steem-test directory
	// Start from current directory and walk up
	dir := startDir
	for i := 0; i < 10; i++ { // Limit search depth
		// Try test/steem-test (new location)
		steemTestPath := filepath.Join(dir, "test", "steem-test", "run.sh")
		if _, err := os.Stat(steemTestPath); err == nil {
			return filepath.Join(dir, "test", "steem-test")
		}

		// Try test/test_data/steem-test (legacy location for backward compatibility)
		steemTestPathLegacy := filepath.Join(dir, "test", "test_data", "steem-test", "run.sh")
		if _, err := os.Stat(steemTestPathLegacy); err == nil {
			return filepath.Join(dir, "test", "test_data", "steem-test")
		}

		// Also try relative to steemdb-sync root
		steemTestPath2 := filepath.Join(dir, "steem-test", "run.sh")
		if _, err := os.Stat(steemTestPath2); err == nil {
			return filepath.Join(dir, "steem-test")
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached root
		}
		dir = parent
	}
	return ""
}
