//go:build !darwin

package main

// The portable fallback reserves full disk capacity and refuses overwrite.
// Reflink support can replace it without changing the snapshot contract.
func snapshotDisk(src, dst string) error { return copyExclusive(src, dst) }
