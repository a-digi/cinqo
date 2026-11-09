//go:build !unix

package chromeproc

// On a system without these readers (Windows, plan 224 Q7) nothing is tracked beyond the list itself.
type realProc struct{}

func (realProc) Alive(int) bool         { return true }
func (realProc) StartTime(int) string   { return "" }
func (realProc) CommandLine(int) string { return "" }
func (realProc) Parents() map[int]int   { return nil }
func (realProc) Terminate(int) bool     { return false }
func (realProc) Kill(int) bool          { return false }
func (realProc) List() []ProcInfo       { return nil }
