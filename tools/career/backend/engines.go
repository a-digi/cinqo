package main

import (
	"career-tool-backend/instruction"
	"career-tool-backend/instruction/katana"
	"career-tool-backend/instruction/rod"
)

// instructionEngines is the ONE place that names the instruction engines (plan 192). Removing an engine means deleting its line here and
// its package under instruction/.
func instructionEngines() []instruction.Engine {
	return []instruction.Engine{
		rod.New(),
		katana.New(),
	}
}

func registerInstructionEngines() {
	for _, e := range instructionEngines() {
		instruction.Register(e)
	}
}
