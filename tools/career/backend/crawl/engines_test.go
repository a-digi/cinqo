package crawl

import (
	"career-tool-backend/instruction"
	"career-tool-backend/instruction/katana"
	"career-tool-backend/instruction/rod"
)

// The crawl decides how a link's job list is crawled by asking the registered instruction engines (plan 192); the production binary
// registers them in main (engines.go), so the tests of this package do the same.
func init() {
	instruction.Register(rod.New())
	instruction.Register(katana.New())
}
