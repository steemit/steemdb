package processor

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// cursorID is the _id of the processor cursor document in the status collection.
const cursorID = "processor_height"

// Cursor manages the processor's consumption position.
// The position is the last block number that has been fully processed.
// It is stored as a single document in the status collection — single-process
// sequential writes, no locking needed.
type Cursor struct {
	statusCol *mongo.Collection
}

// NewCursor creates a new Cursor backed by the status collection.
func NewCursor(db *mongo.Database) *Cursor {
	return &Cursor{
		statusCol: db.Collection("status"),
	}
}

// Get returns the current processed block height, or 0 if not initialized.
func (c *Cursor) Get(ctx context.Context) (uint32, error) {
	var doc struct {
		ID    string `bson:"_id"`
		Value uint32 `bson:"value"`
	}
	err := c.statusCol.FindOne(ctx, bson.M{"_id": cursorID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return 0, nil
	}
	if err != nil {
		return 0, errors.Wrap(err, "failed to read processor cursor")
	}
	return doc.Value, nil
}

// Advance atomically sets the cursor to blockNum.
// This is the commit point — once advanced, the block is considered fully processed.
func (c *Cursor) Advance(ctx context.Context, blockNum uint32) error {
	filter := bson.M{"_id": cursorID}
	update := bson.M{
		"$set": bson.M{
			"value": blockNum,
		},
	}
	opts := options.Update().SetUpsert(true)

	_, err := c.statusCol.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return errors.Wrapf(err, "failed to advance processor cursor to %d", blockNum)
	}
	return nil
}
