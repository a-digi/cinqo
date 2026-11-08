package katana_test

import (
	"testing"

	"career-tool-backend/instruction"
	"career-tool-backend/instruction/enginetest"
	"career-tool-backend/instruction/katana"
	"career-tool-backend/portal"
)

func TestContract(t *testing.T) { enginetest.Run(t, katana.New()) }

func TestManifest(t *testing.T) {
	p, err := enginetest.CheckManifest("../../../manifest.json", []instruction.Engine{katana.New()})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range p {
		t.Error(s)
	}
}

// The method names are stored in the database; the neutral constants must stay equal to the portal's.
func TestMethodNamesMatchThePortal(t *testing.T) {
	if instruction.MethodLinks != portal.CrawlMethodLinks || instruction.MethodInstructions != portal.CrawlMethodInstructions {
		t.Fatal("instruction.Method* and portal.CrawlMethod* differ")
	}
	if katana.New().ListingMethod() != portal.CrawlMethodLinks {
		t.Error("Katana crawls with the link crawler's rule")
	}
}

func TestJudgements(t *testing.T) {
	e := katana.New()
	if !e.HasListing(instruction.LinkFacts{HasRule: true}) || e.HasListing(instruction.LinkFacts{HasAIInstructions: true}) {
		t.Error("Katana counts the rule as the listing, not the AI instructions")
	}
	if !e.HasListing(instruction.LinkFacts{RequiresLogin: true, HasAIInstructions: true}) || e.HasListing(instruction.LinkFacts{RequiresLogin: true, HasRule: true}) {
		t.Error("a login link is instructed by its AI instructions")
	}
	if p := e.JobDetailPolicy(instruction.LinkFacts{}); p.WrittenBy != instruction.WrittenByPicker || p.AutoAIRequest || !p.NeedsCrawledJobs {
		t.Errorf("policy: %+v", p)
	}
	if p := e.JobDetailPolicy(instruction.LinkFacts{RequiresLogin: true}); p.WrittenBy != instruction.WrittenByAI || !p.AutoAIRequest || p.NeedsCrawledJobs {
		t.Errorf("login policy: %+v", p)
	}
	adj := e.(instruction.PipelineAdjuster)
	f := instruction.PipelineFacts{DetailAIRequested: true, DetailFailedRuns: 2}
	adj.AdjustPipeline(instruction.LinkFacts{}, &f)
	if f.DetailAIRequested || f.DetailFailedRuns != 0 {
		t.Errorf("facts not adjusted: %+v", f)
	}
	f = instruction.PipelineFacts{DetailAIRequested: true, DetailFailedRuns: 2}
	adj.AdjustPipeline(instruction.LinkFacts{RequiresLogin: true}, &f)
	if !f.DetailAIRequested || f.DetailFailedRuns != 2 {
		t.Errorf("a login link must keep its facts: %+v", f)
	}
}
