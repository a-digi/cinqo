package main

import (
	"net/http"

	"career-tool-backend/instruction"
	"career-tool-backend/instruction/katana"
	katanaservice "career-tool-backend/instruction/katana/service"
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

// registerInstructionRoutes serves the engines' internal HTTP routes on Career's own loopback port. Not listed in manifest.json, so core
// never forwards them; each one also needs the tool's service token.
func registerInstructionRoutes(mux *http.ServeMux) {
	mux.HandleFunc(katanaservice.Prefix, katanaservice.Handler)
	mux.HandleFunc("/portal-links/katana/start-over", katanaservice.StartOverHandler) // the screen's Start over (listed in manifest.json)
}
