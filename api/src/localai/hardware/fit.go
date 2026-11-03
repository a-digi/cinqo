package hardware

// Our own rule of thumb for "does this model fit this machine", not a figure from a model's authors: the model file
// plus a quarter for the context cache and the runtime, plus a fixed reserve for the operating system and Cinqo itself.
// It is meant to be measured on real runs (the spike and the live proof of the plan) and adjusted then.
const (
	overheadNumerator   = 5 // the file size times 5/4
	overheadDenominator = 4
	reservedBytes       = 3 << 30 // 3 GiB left for the system and the app
)

// MinRAMBytes is the total memory a machine should have to run a model file of this size.
func MinRAMBytes(modelSizeBytes int64) uint64 {
	if modelSizeBytes <= 0 {
		return 0
	}
	size := uint64(modelSizeBytes)
	return size*overheadNumerator/overheadDenominator + reservedBytes
}

// Fits reports whether the machine has the memory for a model file of this size: nil when the memory is unknown.
func (i Info) Fits(modelSizeBytes int64) *bool {
	if i.RAMBytes == 0 {
		return nil
	}
	fits := i.RAMBytes >= MinRAMBytes(modelSizeBytes)
	return &fits
}
