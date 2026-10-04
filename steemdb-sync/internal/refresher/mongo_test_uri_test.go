package refresher

import (
	"os"
)

// mongoTestURI returns the integration-test MongoDB URI, or "" when unset.
func mongoTestURI() string {
	return os.Getenv("STEEMDB_TEST_MONGO_URI")
}
