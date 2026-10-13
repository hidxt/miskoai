package privatefs

import "os"

// OpenRead returns a checked read-only handle. The caller owns Close; the file
// itself remains owned by its persistent owner. Recheck identity after use.
func OpenRead(path string, maxBytes int64) (*os.File, error) { return checkedOpen(path, maxBytes) }
