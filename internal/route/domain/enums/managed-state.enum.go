package enums

// ManagedState says how much authority we have over a generated file.
type ManagedState string

const (
	// StateManaged: we wrote it and it still matches the hash we recorded.
	StateManaged ManagedState = "managed"
	// StateAdopted: we wrote it, but a human edited it since. We stop touching it.
	StateAdopted ManagedState = "adopted"
	// StateUnmanaged: we never wrote it. Listed, never modified.
	StateUnmanaged ManagedState = "unmanaged"
)
