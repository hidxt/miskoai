package maintenance

// SyncPrivateDirectory uses the existing platform durability boundary. Windows
// checks private metadata only; Linux directory fsync needs native evidence.
func SyncPrivateDirectory(dir string) error { return syncDirectory(dir) }
