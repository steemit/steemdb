# Data Validation Framework Usage Guide

This guide explains how to use the data validation framework to compare data between the new Go implementation and the legacy Python implementation.

## Overview

The validation framework allows you to:
- Compare block data between new and legacy databases
- Validate operation counts per block
- Compare collection document counts
- Generate detailed validation reports

## Prerequisites

1. **Two MongoDB instances**:
   - New database: Running the optimized Go sync service
   - Legacy database: Running the original Python sync service

2. **Both databases should have synchronized data** for the block range you want to validate

## Building the Validation Tool

First, build the validation tool:

```bash
cd steemdb-sync
mkdir -p ../bin
go build -o ../bin/validate ./cmd/validate
```

## Basic Usage

### Command Syntax

```bash
../bin/validate <new_mongodb_uri> <legacy_mongodb_uri> <start_block> [end_block] [sample_rate]
```

### Parameters

- `new_mongodb_uri`: MongoDB connection string for the new Go implementation database
- `legacy_mongodb_uri`: MongoDB connection string for the legacy Python implementation database
- `start_block`: Starting block number to validate
- `end_block`: (Optional) Ending block number. Defaults to `start_block + 100`
- `sample_rate`: (Optional) Percentage of blocks to sample (0.01 = 1%). Defaults to 0.01 (1%)

### Examples

#### Example 1: Validate a specific block range with default 1% sampling

```bash
../bin/validate \
  mongodb://localhost:27017 \
  mongodb://localhost:27018 \
  1000000 \
  1000100
```

This will:
- Validate blocks 1,000,000 to 1,000,100
- Sample 1% of blocks (approximately 1 block)
- Compare block metadata, transaction counts, and operation counts

#### Example 2: Validate with custom sample rate (10%)

```bash
../bin/validate \
  mongodb://localhost:27017 \
  mongodb://localhost:27018 \
  1000000 \
  1000100 \
  0.1
```

This will sample 10% of blocks (approximately 10 blocks) for validation.

#### Example 3: Full validation (100% sampling)

```bash
../bin/validate \
  mongodb://localhost:27017 \
  mongodb://localhost:27018 \
  1000000 \
  1000100 \
  1.0
```

This will validate all blocks in the range (100% sampling).

#### Example 4: Validate using Docker containers

If your databases are in Docker containers:

```bash
../bin/validate \
  mongodb://mongo:27017 \
  mongodb://legacy-mongo:27017 \
  1000000 \
  1000100
```

Or if running from host machine:

```bash
../bin/validate \
  mongodb://localhost:27017 \
  mongodb://localhost:27018 \
  1000000 \
  1000100
```

## What Gets Validated

### 1. Block Metadata

The framework compares:
- **Block number** (`_id`)
- **Timestamp** (`_ts`)
- **Witness** (block producer)
- **Previous block hash**
- **Transaction count**

### 2. Operation Counts

For each validated block, the framework compares operation counts in:
- `vote` collection
- `transfer` collection
- `comment` collection
- `author_reward` collection
- `curation_reward` collection

### 3. Collection Document Counts

The framework compares total document counts across all collections:
- `block_30d`
- `vote`
- `transfer`
- `comment`
- `author_reward`
- `curation_reward`

## Output Format

### Validation Report

The tool generates a report with:

```
Validation Report
==================
Total Blocks: 100
Validated Blocks: 1 (1.00%)
Valid Blocks: 1
Invalid Blocks: 0
Duration: 2.5s

Discrepancies:
  Block 1000050: 2 discrepancies
    - Field: timestamp, New: 2020-03-21T13:04:57Z, Legacy: 2020-03-21T13:04:58Z, Match: false
    - Field: transaction_count, New: 5, Legacy: 6, Match: false
```

### Collection Count Comparison

```
Collection Count Discrepancies:
  - Field: document_count_vote, New: 1234567, Legacy: 1234568, Match: false
```

Or if all counts match:

```
All collection counts match!
```

## Exit Codes

- `0`: Validation passed (no discrepancies found)
- `1`: Validation failed (discrepancies found or errors occurred)

## Programmatic Usage

You can also use the validation framework programmatically in your Go code:

```go
package main

import (
    "context"
    "github.com/steemdb/sync/internal/database"
    "github.com/steemdb/sync/internal/validation"
    "github.com/steemdb/sync/internal/utils"
    "go.mongodb.org/mongo-driver/mongo"
)

func main() {
    // Initialize databases
    newDB := // ... initialize new database
    legacyDB := // ... initialize legacy database
    logger := // ... initialize logger
    
    // Create validator with 1% sample rate
    validator := validation.NewValidator(newDB, legacyDB, logger, 0.01)
    
    // Validate block range
    ctx := context.Background()
    report, err := validator.ValidateBlockRange(ctx, 1000000, 1000100)
    if err != nil {
        // Handle error
    }
    
    // Check results
    if report.InvalidBlocks > 0 {
        // Handle discrepancies
        for _, result := range report.Discrepancies {
            if !result.Valid {
                // Process discrepancies
            }
        }
    }
    
    // Validate collection counts
    countDiscrepancies, err := validator.ValidateCollectionCounts(ctx)
    // Process count discrepancies
}
```

## Validation Methods

### ValidateBlockRange

Validates a range of blocks using random sampling:

```go
report, err := validator.ValidateBlockRange(ctx, startBlock, endBlock)
```

### ValidateBlock

Validates a single specific block:

```go
result := validator.ValidateBlock(ctx, blockNum)
```

### ValidateCollectionCounts

Compares total document counts across collections:

```go
discrepancies, err := validator.ValidateCollectionCounts(ctx)
```

## Best Practices

1. **Start with small ranges**: Test with a small block range first (e.g., 100 blocks)

2. **Use appropriate sample rates**:
   - For quick checks: 0.01 (1%)
   - For thorough validation: 0.1 (10%)
   - For complete validation: 1.0 (100%)

3. **Validate after major changes**: Run validation after implementing optimizations or bug fixes

4. **Compare at different stages**: Validate both historical blocks and recent blocks

5. **Monitor collection counts**: Regularly check collection counts to ensure overall data consistency

## Troubleshooting

### Connection Issues

If you get connection errors:
- Verify MongoDB URIs are correct
- Check network connectivity
- Ensure MongoDB instances are running
- Verify authentication credentials if required

### Missing Blocks

If blocks are missing in one database:
- Check if sync services are running
- Verify block ranges are synchronized
- Check for sync errors in logs

### Discrepancies Found

If discrepancies are found:
1. Review the detailed discrepancy report
2. Check if the difference is expected (e.g., due to different processing logic)
3. Investigate the specific blocks with discrepancies
4. Compare operation-level data if needed

## Integration with CI/CD

You can integrate validation into your CI/CD pipeline:

```bash
#!/bin/bash
# validate.sh

NEW_DB="mongodb://new-db:27017"
LEGACY_DB="mongodb://legacy-db:27017"
START_BLOCK=1000000
END_BLOCK=1000100

../bin/validate $NEW_DB $LEGACY_DB $START_BLOCK $END_BLOCK 0.1

if [ $? -ne 0 ]; then
    echo "Validation failed!"
    exit 1
fi
```

## Performance Considerations

- **Sampling rate**: Lower sample rates (0.01) are faster but less thorough
- **Block range**: Smaller ranges validate faster
- **Database load**: Validation reads from both databases, consider running during low-traffic periods
- **Network latency**: If databases are on different networks, validation may be slower

## Next Steps

After validation:
1. Review any discrepancies found
2. Investigate root causes
3. Fix issues if found
4. Re-run validation to confirm fixes
5. Gradually increase validation scope as confidence grows

