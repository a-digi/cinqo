package resolver

import (
	"fmt"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/toolrepository/catalog"
)

// the case of the plan: Career needs Browser and PDF Tools 0.0.1 or newer.
func careerRepo() Source {
	return repo("main", "Cinqo tools",
		tl("career", ver("0.0.806", needs("browser"), needs("pdf_tools", "0.0.1"))),
		tl("browser", ver("0.0.245")),
		tl("pdf_tools", ver("0.0.124"), ver("0.0.50"), ver("0.0.1")),
	)
}

func TestNoRequirementsIsOneStep(t *testing.T) {
	p := mustPlan(t, ask("browser", []Source{careerRepo()}), "browser:install@0.0.245")
	s := p.Steps[0]
	if s.Name != "Tool" || s.RepositoryID != "main" || s.RepositoryName != "Cinqo tools" || s.SizeBytes != 100 || s.VersionID != "v0.0.245" || len(s.Marks) != 0 || len(s.RequiredBy) != 0 {
		t.Fatalf("%+v", s)
	}
	if len(p.Searched) != 1 || p.Searched[0].ID != "main" {
		t.Fatalf("searched: %+v", p.Searched)
	}
}

// S1: a missing dependency is installed first.
func TestMissingDependenciesAreInstalledFirst(t *testing.T) {
	p := mustPlan(t, ask("career", []Source{careerRepo()}), "browser:install@0.0.245 pdf_tools:install@0.0.124 career:install@0.0.806")
	if got := stepOf(p, "browser").RequiredBy; len(got) != 1 || got[0] != "career" {
		t.Fatalf("required by: %v", got)
	}
	if len(stepOf(p, "career").RequiredBy) != 0 {
		t.Fatal("nobody in the plan needs the tool that was asked for")
	}
}

// S3: an installed, enabled dependency in range is not on the list, and the walk does not go into it.
func TestAnInstalledDependencyInRangeIsLeftAloneAndNotWalked(t *testing.T) {
	broken := installed("browser", "1.0.0").needs(req("ghost")) // its own requirement is missing: not our business
	mustPlan(t, ask("career", []Source{careerRepo()}, broken, installed("pdf_tools", "0.0.9")), "career:install@0.0.806")
}

// S4: a disabled dependency in range is only enabled, and what it needs is checked (enabling re-checks it).
func TestADisabledDependencyIsEnabledAndItsOwnRequirementsAreWalked(t *testing.T) {
	src := repo("main", "Main",
		tl("career", ver("1.0.0", needs("browser"))),
		tl("helper", ver("2.0.0")),
	)
	p := mustPlan(t, ask("career", []Source{src}, installed("browser", "1.0.0").disabled().needs(req("helper"))), "helper:install@2.0.0 browser:enable career:install@1.0.0")
	if s := stepOf(p, "browser"); !hasMark(s, MarkWasDisabled) || s.Action != ActionEnable || s.Version != "1.0.0" || s.InstalledVersion != "1.0.0" {
		t.Fatalf("%+v", s)
	}
}

// Version choice (decision 4): the current version if it fits, else the newest that does.
func TestWhichVersionOfADependencyIsChosen(t *testing.T) {
	src := func(current string, versions ...string) []Source {
		var vs []catalog.Version
		for _, v := range versions {
			vs = append(vs, ver(v))
		}
		d := tl("dep", vs...)
		d.CurrentVersionID = "v" + current
		return []Source{repo("main", "Main", tl("app", ver("1.0.0", needs("dep", "2.0.0"))), d)}
	}
	// the current version fits: it wins even when a newer one is listed
	mustPlan(t, ask("app", src("2.5.0", "3.0.0", "2.5.0", "2.0.0")), "dep:install@2.5.0 app:install@1.0.0")
	// the current version is too old: the newest that fits
	mustPlan(t, ask("app", src("1.0.0", "1.0.0", "3.0.0", "2.5.0")), "dep:install@3.0.0 app:install@1.0.0")
	// numbers, not text
	mustPlan(t, ask("app", src("2.0.0", "2.0.0", "10.0.0", "9.0.0")), "dep:install@2.0.0 app:install@1.0.0")
	mustPlan(t, ask("app", src("1.0.0", "1.0.0", "10.0.0", "9.0.0")), "dep:install@10.0.0 app:install@1.0.0")
}

// S5: an installed dependency that is too old is updated; dependents are listed; running ones restart.
func TestAnOutdatedDependencyIsUpdatedAndItsDependentsAreListed(t *testing.T) {
	notes := installed("notes", "1.0.0").needs(req("pdf_tools", "0.0.1"))
	p := mustPlan(t, ask("career", []Source{careerRepo()}, installed("browser", "0.0.245"), installed("pdf_tools", "0.0.0").running(), notes),
		"pdf_tools:update@0.0.124 career:install@0.0.806")
	s := stepOf(p, "pdf_tools")
	if s.InstalledVersion != "0.0.0" || len(s.Dependents) != 1 || s.Dependents[0] != "notes" || !hasMark(s, MarkRestarts) {
		t.Fatalf("%+v", s)
	}
	// not running, or nobody uses it: no restart mark; a disabled dependent is not listed
	p = mustPlan(t, ask("career", []Source{careerRepo()}, installed("browser", "0.0.245"), installed("pdf_tools", "0.0.0"), notes), "pdf_tools:update@0.0.124 career:install@0.0.806")
	if hasMark(stepOf(p, "pdf_tools"), MarkRestarts) {
		t.Fatal("a tool that is not running does not restart")
	}
	p = mustPlan(t, ask("career", []Source{careerRepo()}, installed("browser", "0.0.245"), installed("pdf_tools", "0.0.0").running(), notes.disabled()), "pdf_tools:update@0.0.124 career:install@0.0.806")
	if len(stepOf(p, "pdf_tools").Dependents) != 0 || hasMark(stepOf(p, "pdf_tools"), MarkRestarts) {
		t.Fatalf("a disabled dependent does not restart: %+v", stepOf(p, "pdf_tools"))
	}
}

func TestADisabledOutdatedDependencyIsUpdatedAndEnabledAfterwards(t *testing.T) {
	p := mustPlan(t, ask("career", []Source{careerRepo()}, installed("browser", "0.0.245"), installed("pdf_tools", "0.0.0").disabled()), "pdf_tools:update@0.0.124 career:install@0.0.806")
	if s := stepOf(p, "pdf_tools"); !s.EnableAfter || !hasMark(s, MarkWasDisabled) {
		t.Fatalf("an update keeps the tool disabled, so it must be enabled after: %+v", s)
	}
	if stepOf(p, "career").EnableAfter {
		t.Fatal("only a tool that was disabled")
	}
}

// S6: an update that would put an installed tool outside its range is blocked, naming it.
func TestUpdatingADependencyThatAnInstalledToolPinsIsBlocked(t *testing.T) {
	src := repo("main", "Main",
		tl("career", ver("1.0.0", needs("pdf_tools", "2.0.0"))),
		tl("pdf_tools", ver("2.5.0"), ver("2.0.0"), ver("1.5.0")),
	)
	pinned := installed("notes", "1.0.0").needs(req("pdf_tools", "1.0.0", "1.9.9"))
	p := mustPlan(t, ask("career", []Source{src}, installed("pdf_tools", "1.5.0"), pinned), "!would_break_installed(pdf_tools)")
	pr := problemOf(p, "pdf_tools")
	if !strings.Contains(pr.Message, "notes") || !strings.Contains(pr.Message, "1.0.0 to 1.9.9") || !strings.Contains(pr.Message, "updating") || len(pr.RequiredBy) != 1 || pr.RequiredBy[0] != "career" {
		t.Fatalf("%+v", pr)
	}
	// the same for a fresh install of something installed tools already pin
	p = mustPlan(t, ask("career", []Source{src}, pinned), "!would_break_installed(pdf_tools)")
	if !strings.Contains(problemOf(p, "pdf_tools").Message, "installing") {
		t.Fatalf("%+v", p.Problems)
	}
	// a pin the new version still meets does not block, and the pinned tool is listed
	wide := installed("notes", "1.0.0").needs(req("pdf_tools", "1.0.0", "2.9.9"))
	p = mustPlan(t, ask("career", []Source{src}, installed("pdf_tools", "1.5.0"), wide), "pdf_tools:update@2.5.0 career:install@1.0.0")
	if got := stepOf(p, "pdf_tools").Dependents; len(got) != 1 || got[0] != "notes" {
		t.Fatalf("%v", got)
	}
	// the newest version is outside the pin, an older one inside it is chosen
	mid := installed("notes", "1.0.0").needs(req("pdf_tools", "2.0.0", "2.2.0"))
	mustPlan(t, ask("career", []Source{src}, installed("pdf_tools", "1.5.0"), mid), "pdf_tools:update@2.0.0 career:install@1.0.0")
}

// A tool that is itself part of the plan drops its old pin: its new version's requirements count instead.
func TestAToolThatIsUpdatedInThePlanNoLongerPins(t *testing.T) {
	src := repo("main", "Main",
		tl("notes", ver("2.0.0", needs("pdf_tools", "2.0.0"))),
		tl("pdf_tools", ver("2.5.0")),
	)
	old := installed("notes", "1.0.0").needs(req("pdf_tools", "1.0.0", "1.9.9"))
	mustPlan(t, ask("notes", []Source{src}, installed("pdf_tools", "1.5.0"), old), "pdf_tools:update@2.5.0 notes:update@2.0.0")
}

// S7: never downgrades.
func TestNothingIsDowngraded(t *testing.T) {
	src := repo("main", "Main", tl("career", ver("1.0.0", needs("browser", "1.0.0", "2.0.0"))), tl("browser", ver("2.0.0"), ver("3.0.0")))
	p := mustPlan(t, ask("career", []Source{src}, installed("browser", "3.0.0")), "!needs_downgrade(browser)")
	if !strings.Contains(problemOf(p, "browser").Message, "3.0.0 is installed") || !strings.Contains(problemOf(p, "browser").Message, "downgraded") {
		t.Fatalf("%+v", p.Problems)
	}
	// exactly the maximum is fine
	mustPlan(t, ask("career", []Source{src}, installed("browser", "2.0.0")), "career:install@1.0.0")
}

// S8 and S9: several tools ask for the same dependency.
func TestSeveralToolsAskingForTheSameDependency(t *testing.T) {
	src := repo("main", "Main",
		tl("app", ver("1.0.0", needs("b"), needs("c"))),
		tl("b", ver("1.0.0", needs("d", "1.0.0"))),
		tl("c", ver("1.0.0", needs("d", "2.0.0"))),
		tl("d", ver("3.0.0"), ver("2.0.0"), ver("1.0.0")),
	)
	// a diamond: d once, the highest minimum wins, d before b and c, app last
	mustPlan(t, ask("app", []Source{src}), "d:install@3.0.0 b:install@1.0.0 c:install@1.0.0 app:install@1.0.0")

	// the root and an installed tool both constrain the dependency: one version must fit both
	two := repo("main", "Main", tl("career", ver("1.0.0", needs("browser", "1.2.0"))), tl("browser", ver("1.6.0"), ver("1.3.0"), ver("1.0.0")))
	notes := installed("notes", "1.0.0").needs(req("browser", "1.0.0", "1.5.0"))
	mustPlan(t, ask("career", []Source{two}, notes), "browser:install@1.3.0 career:install@1.0.0")

	// a maximum and a minimum that cannot meet (S8)
	clash := repo("main", "Main",
		tl("app", ver("1.0.0", needs("b"), needs("c"))),
		tl("b", ver("1.0.0", needs("d", "2.0.0"))),
		tl("c", ver("1.0.0", needs("d", "1.0.0", "1.5.0"))),
		tl("d", ver("2.5.0"), ver("1.2.0")),
	)
	p := mustPlan(t, ask("app", []Source{clash}), "!conflicting_requirements(d)")
	if m := problemOf(p, "d").Message; !strings.Contains(m, "b needs d 2.0.0") || !strings.Contains(m, "c works with it only up to 1.5.0") {
		t.Fatalf("%s", m)
	}
	if got := problemOf(p, "d").RequiredBy; len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("%v", got)
	}
}

// S10: nothing offers it.
func TestADependencyNoRepositoryOffersIsReportedWithWhereItWasLooked(t *testing.T) {
	src := repo("main", "Main", tl("career", ver("1.0.0", needs("ghost"))))
	other := repo("other", "Other")
	p := mustPlan(t, ask("career", []Source{src, other}), "!not_in_any_repository(ghost)")
	if m := problemOf(p, "ghost").Message; !strings.Contains(m, "Main, Other") || !strings.Contains(m, "install it from another source") {
		t.Fatalf("%s", m)
	}
	if len(p.Searched) != 2 || p.Searched[0].ID != "main" {
		t.Fatalf("%+v", p.Searched)
	}
	// offered, but no version in range
	few := repo("main", "Main", tl("career", ver("1.0.0", needs("pdf_tools", "2.0.0"))), tl("pdf_tools", ver("1.5.0"), ver("1.0.0")))
	p = mustPlan(t, ask("career", []Source{few}), "!no_version_in_range(pdf_tools)")
	if m := problemOf(p, "pdf_tools").Message; !strings.Contains(m, "2.0.0 or newer") || !strings.Contains(m, "newest offered is 1.5.0") {
		t.Fatalf("%s", m)
	}
	// a repository that was never loaded is left out of the search
	unloaded := Source{ID: "never", Name: "Never loaded"}
	p = mustPlan(t, ask("career", []Source{src, unloaded}), "!not_in_any_repository(ghost)")
	if strings.Contains(problemOf(p, "ghost").Message, "Never loaded") || len(p.Searched) != 1 {
		t.Fatalf("%+v", p)
	}
}

// S11: a version that cannot run here.
func TestAVersionThatCannotRunHereIsNotChosen(t *testing.T) {
	needsNewApp := ver("2.0.0")
	needsNewApp.MinAppVersion = "9.0.0"
	src := repo("main", "Main", tl("career", ver("1.0.0", needs("dep"))), tl("dep", needsNewApp))
	p := mustPlan(t, ask("career", []Source{src}), "!not_installable_here(dep)")
	if !strings.Contains(problemOf(p, "dep").Message, "app version 9.0.0") {
		t.Fatalf("%s", problemOf(p, "dep").Message)
	}
	// an older version that does run is chosen instead
	old := ver("1.0.0")
	src = repo("main", "Main", tl("career", ver("1.0.0", needs("dep"))), tl("dep", needsNewApp, old))
	mustPlan(t, ask("career", []Source{src}), "dep:install@1.0.0 career:install@1.0.0")
	// no package for this machine
	linuxOnly := ver("2.0.0")
	linuxOnly.Packages = []catalog.Package{{OS: "linux", Arch: "amd64", URL: "https://x/l.zip", SHA256: "s"}}
	src = repo("main", "Main", tl("career", ver("1.0.0", needs("dep"))), tl("dep", linuxOnly))
	p = mustPlan(t, ask("career", []Source{src}), "!not_installable_here(dep)")
	if !strings.Contains(problemOf(p, "dep").Message, "no package for darwin/arm64") {
		t.Fatalf("%s", problemOf(p, "dep").Message)
	}
	// the tool that was asked for is checked the same way
	asked := repo("main", "Main", tl("career", needsNewApp))
	mustPlan(t, ask("career", []Source{asked}), "!not_installable_here(career)")
}

// S2: only another repository has it.
func TestADependencyFromAnotherRepositoryIsMarked(t *testing.T) {
	main := repo("main", "Main", tl("career", ver("1.0.0", needs("browser"))))
	other := repo("other", "Other", tl("browser", ver("2.0.0")))
	p := mustPlan(t, ask("career", []Source{main, other}), "browser:install@2.0.0 career:install@1.0.0")
	if s := stepOf(p, "browser"); !hasMark(s, MarkOtherRepository) || s.RepositoryID != "other" || s.RepositoryName != "Other" {
		t.Fatalf("%+v", s)
	}
	if hasMark(stepOf(p, "career"), MarkOtherRepository) {
		t.Fatal("the tool that was asked for comes from the chosen repository")
	}
	other.Local = true
	p = mustPlan(t, ask("career", []Source{main, other}), "browser:install@2.0.0 career:install@1.0.0")
	if !hasMark(stepOf(p, "browser"), MarkLocalNetwork) {
		t.Fatalf("%+v", stepOf(p, "browser"))
	}
}

// Decision 8: where a dependency is looked for first.
func TestRepositoriesAreSearchedInTheOrderOfTrust(t *testing.T) {
	dep := func(v string) catalog.Tool { return tl("browser", ver(v)) }
	main := repo("main", "Main", tl("career", ver("1.0.0", needs("browser"))), dep("1.0.0"))
	zeta := repo("zeta", "Zeta", dep("2.0.0"))
	alpha := repo("alpha", "Alpha", dep("3.0.0"))
	// the chosen repository first, even though others have newer versions
	mustPlan(t, ask("career", []Source{main, zeta, alpha}), "browser:install@1.0.0 career:install@1.0.0")
	// without it there: the repository the tool is installed from, then the others by name
	noBrowser := repo("main", "Main", tl("career", ver("1.0.0", needs("browser", "1.5.0"))))
	mustPlan(t, ask("career", []Source{noBrowser, zeta, alpha}), "browser:install@3.0.0 career:install@1.0.0")
	installedFromZeta := installed("browser", "1.0.0").from("zeta")
	p := mustPlan(t, ask("career", []Source{noBrowser, zeta, alpha}, installedFromZeta), "browser:update@2.0.0 career:install@1.0.0")
	if stepOf(p, "browser").RepositoryID != "zeta" {
		t.Fatalf("an update stays on the repository the tool came from: %+v", stepOf(p, "browser"))
	}
	// a recorded source that is no longer registered is ignored
	mustPlan(t, ask("career", []Source{noBrowser, zeta, alpha}, installed("browser", "1.0.0").from("removed")), "browser:update@3.0.0 career:install@1.0.0")
	if got := Resolve(ask("career", []Source{noBrowser, zeta, alpha})).Searched; len(got) != 3 || got[0].ID != "main" || got[1].ID != "alpha" || got[2].ID != "zeta" {
		t.Fatalf("searched order: %+v", got)
	}
}

// S12: the publisher changed, or two repositories disagree about the same version.
func TestAChangedPublisherAndConflictingCopiesAreBlocked(t *testing.T) {
	root := tl("career", ver("1.0.0", needs("browser", "2.0.0")))
	byEve := withAuthors(ver("2.0.0"), "Eve")
	src := repo("main", "Main", root, tl("browser", byEve))
	p := mustPlan(t, ask("career", []Source{src}, installed("browser", "1.0.0").by("Ada")), "!publisher_changed(browser)")
	if m := problemOf(p, "browser").Message; !strings.Contains(m, "Eve") || !strings.Contains(m, "Ada") {
		t.Fatalf("%s", m)
	}
	// the same publisher (in any order), or no authors on either side, is fine
	ada := withAuthors(withAuthors(ver("2.0.0"), "Ada"), "Grace")
	mustPlan(t, ask("career", []Source{repo("main", "Main", root, tl("browser", ada))}, installed("browser", "1.0.0").by("Grace", "Ada")), "browser:update@2.0.0 career:install@1.0.0")
	mustPlan(t, ask("career", []Source{src}, installed("browser", "1.0.0")), "browser:update@2.0.0 career:install@1.0.0")
	mustPlan(t, ask("career", []Source{repo("main", "Main", root, tl("browser", ver("2.0.0")))}, installed("browser", "1.0.0").by("Ada")), "browser:update@2.0.0 career:install@1.0.0")
	// a fresh install has nothing to compare with
	mustPlan(t, ask("career", []Source{src}), "browser:install@2.0.0 career:install@1.0.0")
	// the tool that was asked for is checked when it is an update
	p = mustPlan(t, ask("browser", []Source{repo("main", "Main", tl("browser", byEve))}, installed("browser", "1.0.0").by("Ada")), "!publisher_changed(browser)")

	// two repositories list the same version with different packages
	a := repo("main", "Main", root, tl("browser", withSHA(ver("2.0.0"), "aaa")))
	b := repo("other", "Other", tl("browser", withSHA(ver("2.0.0"), "bbb")))
	p = mustPlan(t, ask("career", []Source{a, b}), "!conflicting_copies(browser)")
	if m := problemOf(p, "browser").Message; !strings.Contains(m, "Main") || !strings.Contains(m, "Other") {
		t.Fatalf("%s", m)
	}
	// the same package in both is fine; so is a different version
	same := repo("other", "Other", tl("browser", withSHA(ver("2.0.0"), "aaa")))
	mustPlan(t, ask("career", []Source{a, same}), "browser:install@2.0.0 career:install@1.0.0")
	diff := repo("other", "Other", tl("browser", withSHA(ver("2.1.0"), "bbb")))
	mustPlan(t, ask("career", []Source{a, diff}), "browser:install@2.0.0 career:install@1.0.0")
}

// S13: cycles, depth and size.
func TestCyclesAreReportedWithTheirPath(t *testing.T) {
	two := repo("main", "Main", tl("a", ver("1.0.0", needs("b"))), tl("b", ver("1.0.0", needs("a"))))
	p := mustPlan(t, ask("a", []Source{two}), "!cycle(a)")
	if m := problemOf(p, "a").Message; !strings.Contains(m, "a -> b -> a") {
		t.Fatalf("%s", m)
	}
	four := repo("main", "Main",
		tl("a", ver("1.0.0", needs("b"))), tl("b", ver("1.0.0", needs("c"))), tl("c", ver("1.0.0", needs("d"))), tl("d", ver("1.0.0", needs("b"))))
	p = mustPlan(t, ask("a", []Source{four}), "!cycle(b)")
	if m := problemOf(p, "b").Message; !strings.Contains(m, "b -> c -> d -> b") {
		t.Fatalf("%s", m)
	}
}

func chain(n int) []Source {
	var tools []catalog.Tool
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("t%02d", i)
		if i == n-1 {
			tools = append(tools, tl(id, ver("1.0.0")))
		} else {
			tools = append(tools, tl(id, ver("1.0.0", needs(fmt.Sprintf("t%02d", i+1)))))
		}
	}
	return []Source{repo("main", "Main", tools...)}
}

func TestDepthAndStepCaps(t *testing.T) {
	p := Resolve(ask("t00", chain(MaxDepth)))
	if !p.OK || len(p.Steps) != MaxDepth || p.Steps[0].ToolID != fmt.Sprintf("t%02d", MaxDepth-1) || p.Steps[MaxDepth-1].ToolID != "t00" {
		t.Fatalf("a chain of exactly %d is followed: %s", MaxDepth, summary(p))
	}
	p = mustPlan(t, ask("t00", chain(MaxDepth+1)), "!too_deep(t00)")
	if !strings.Contains(problemOf(p, "t00").Message, "17 levels") {
		t.Fatalf("%s", problemOf(p, "t00").Message)
	}
	// wide, not deep: exactly the cap is fine, one more is refused
	wide := func(n int) []Source {
		var reqs []catalog.Requirement
		tools := []catalog.Tool{}
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("w%02d", i)
			reqs = append(reqs, needs(id))
			tools = append(tools, tl(id, ver("1.0.0")))
		}
		return []Source{repo("main", "Main", append(tools, tl("root", ver("1.0.0", reqs...)))...)}
	}
	if p := Resolve(ask("root", wide(MaxSteps-1))); !p.OK || len(p.Steps) != MaxSteps {
		t.Fatalf("exactly the cap: %s", summary(p))
	}
	mustPlan(t, ask("root", wide(MaxSteps)), "!too_many(root)")
}

// S14: a repository that could not be refreshed.
func TestAStaleRepositoryIsMarkedAndNeverTheOnlySourceOfADependency(t *testing.T) {
	main := repo("main", "Main", tl("career", ver("1.0.0", needs("browser"))))
	stale := repo("old", "Old", tl("browser", ver("2.0.0")))
	stale.Stale, stale.RefreshedAt = true, "2026-11-01"
	p := mustPlan(t, ask("career", []Source{main, stale}), "!stale_only_source(browser)")
	if m := problemOf(p, "browser").Message; !strings.Contains(m, "Old") || !strings.Contains(m, "2026-11-01") {
		t.Fatalf("%s", m)
	}
	// another repository offers it fresh: the order of trust still decides, and a stale pick is marked
	fresh := repo("zzz", "Zfresh", tl("browser", ver("2.0.0")))
	p = mustPlan(t, ask("career", []Source{main, stale, fresh}), "browser:install@2.0.0 career:install@1.0.0")
	if s := stepOf(p, "browser"); !hasMark(s, MarkStaleRepository) || s.RepositoryID != "old" {
		t.Fatalf("%+v", s)
	}
	// the chosen repository itself being stale is only marked
	staleMain := repo("main", "Main", tl("career", ver("1.0.0", needs("browser"))), tl("browser", ver("1.0.0")))
	staleMain.Stale = true
	p = mustPlan(t, ask("career", []Source{staleMain}), "browser:install@1.0.0 career:install@1.0.0")
	if !hasMark(stepOf(p, "browser"), MarkStaleRepository) || !hasMark(stepOf(p, "career"), MarkStaleRepository) {
		t.Fatal("both come from the stale file and say so")
	}
	if !p.Searched[0].Stale {
		t.Fatalf("%+v", p.Searched)
	}
}

// The tool that was asked for.
func TestTheRequestedToolItself(t *testing.T) {
	src := []Source{repo("main", "Main", tl("career", ver("2.0.0"), ver("1.0.0"), ver("0.5.0")))}
	mustPlan(t, ask("career", src, installed("career", "1.0.0")), "career:update@2.0.0")
	mustPlan(t, ask("career", src, installed("career", "2.0.0")), "!already_installed(career)")
	mustPlan(t, ask("career", src, installed("career", "3.0.0")), "!older_than_installed(career)")
	mustPlan(t, ask("career", src, installed("career", "weird")), "!installed_version_unreadable(career)")
	mustPlan(t, ask("nope", src), "!not_in_any_repository(nope)")
	// a version asked for by id
	in := ask("career", src)
	in.Request.VersionID = "v1.0.0"
	mustPlan(t, in, "career:install@1.0.0")
	in.Request.VersionID = "v9"
	mustPlan(t, in, "!no_version_in_range(career)")
	// an unknown repository, or one that was never loaded
	in = ask("career", src)
	in.Request.RepositoryID = "missing"
	mustPlan(t, in, "!not_in_any_repository(career)")
	mustPlan(t, ask("career", []Source{{ID: "never", Name: "N"}}), "!not_in_any_repository(career)")
	// an update of the root restarts what uses it
	users := installed("notes", "1.0.0").needs(req("career"))
	p := mustPlan(t, ask("career", src, installed("career", "1.0.0").running(), users), "career:update@2.0.0")
	if s := p.Steps[0]; !hasMark(s, MarkRestarts) || len(s.Dependents) != 1 || s.Dependents[0] != "notes" {
		t.Fatalf("%+v", s)
	}
}

// Requirements the downloaded manifest named replace the repository file's.
func TestDiscoveredRequirementsReplaceTheFilesHint(t *testing.T) {
	src := repo("main", "Main", tl("career", ver("1.0.0", needs("browser"))), tl("browser", ver("1.0.0")), tl("extra", ver("1.0.0")))
	in := ask("career", []Source{src})
	in.Discovered = map[string][]Requirement{"career@1.0.0": {req("extra"), req("browser")}}
	mustPlan(t, in, "browser:install@1.0.0 extra:install@1.0.0 career:install@1.0.0")
	// a file with no `requires` at all is completed the same way (discovery)
	bare := repo("main", "Main", tl("career", ver("1.0.0")), tl("browser", ver("1.0.0")))
	in = ask("career", []Source{bare})
	in.Discovered = map[string][]Requirement{"career@1.0.0": {req("browser")}}
	mustPlan(t, in, "browser:install@1.0.0 career:install@1.0.0")
	// the manifest saying "nothing" wins over the file
	in = ask("career", []Source{src})
	in.Discovered = map[string][]Requirement{"career@1.0.0": {}}
	mustPlan(t, in, "career:install@1.0.0")
}

// Dependencies of dependencies, at any depth.
func TestRequirementsOfRequirementsAreFollowed(t *testing.T) {
	src := repo("main", "Main",
		tl("t1", ver("1.0.0", needs("t2"))), tl("t2", ver("1.0.0", needs("t3", "1.0.0"))), tl("t3", ver("1.0.0", needs("t4"))), tl("t4", ver("1.0.0")))
	mustPlan(t, ask("t1", []Source{src}), "t4:install@1.0.0 t3:install@1.0.0 t2:install@1.0.0 t1:install@1.0.0")
	// a satisfied middle tool cuts the walk below it
	mustPlan(t, ask("t1", []Source{src}, installed("t3", "1.0.0")), "t2:install@1.0.0 t1:install@1.0.0")
}

// The same input gives the same plan, whatever order the lists come in.
func TestThePlanDoesNotDependOnTheOrderOfTheInput(t *testing.T) {
	a := repo("main", "Main",
		tl("app", ver("1.0.0", needs("b"), needs("c"), needs("e"))),
		tl("b", ver("1.0.0", needs("d"))), tl("c", ver("1.0.0", needs("d"))), tl("d", ver("2.0.0"), ver("1.0.0")), tl("e", ver("1.0.0")))
	z := repo("zeta", "Zeta", tl("d", ver("3.0.0")))
	m := repo("mid", "Mid", tl("e", ver("5.0.0")))
	insts := []InstalledTool{installed("x", "1.0.0").needs(req("d", "1.0.0")), installed("y", "1.0.0"), installed("e", "0.1.0")}
	want := summary(Resolve(Input{Installed: insts, Sources: []Source{a, z, m}, Env: machine, Request: Request{RepositoryID: "main", ToolID: "app"}}))
	for _, perm := range [][]int{{0, 2, 1}, {1, 0, 2}, {2, 1, 0}} {
		srcs := []Source{a, z, m}
		in := []InstalledTool{insts[perm[0]], insts[perm[1]], insts[perm[2]]}
		got := summary(Resolve(Input{Installed: in, Sources: []Source{srcs[perm[0]], srcs[perm[1]], srcs[perm[2]]}, Env: machine, Request: Request{RepositoryID: "main", ToolID: "app"}}))
		if got != want {
			t.Fatalf("order of the input changed the plan:\n %s\n %s", got, want)
		}
	}
	if want == "" || strings.HasPrefix(want, "!") {
		t.Fatalf("the scenario itself must make a plan: %s", want)
	}
}

// Every step comes after everything it needs and each tool appears once, for many random acyclic graphs.
func TestRandomGraphsAreOrderedAndEachToolOnce(t *testing.T) {
	seed := uint64(12345)
	next := func(n int) int {
		seed = seed*6364136223846793005 + 1442695040888963407
		return int((seed >> 33) % uint64(n))
	}
	for trial := 0; trial < 300; trial++ {
		n := 2 + next(12)
		var tools []catalog.Tool
		deps := map[string][]string{}
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("n%02d", i)
			var reqs []catalog.Requirement
			// only tools with a higher index: acyclic by construction
			for j := i + 1; j < n; j++ {
				if next(3) == 0 {
					reqs = append(reqs, needs(fmt.Sprintf("n%02d", j)))
					deps[id] = append(deps[id], fmt.Sprintf("n%02d", j))
				}
			}
			tools = append(tools, tl(id, ver("1.0.0", reqs...)))
		}
		p := Resolve(ask("n00", []Source{repo("main", "Main", tools...)}))
		if !p.OK {
			t.Fatalf("trial %d: %+v", trial, p.Problems)
		}
		pos := map[string]int{}
		for i, s := range p.Steps {
			if _, dup := pos[s.ToolID]; dup {
				t.Fatalf("trial %d: %s appears twice", trial, s.ToolID)
			}
			pos[s.ToolID] = i
		}
		// reachable from the root
		reach := map[string]bool{"n00": true}
		var walk func(string)
		walk = func(id string) {
			for _, d := range deps[id] {
				if !reach[d] {
					reach[d] = true
					walk(d)
				}
			}
		}
		walk("n00")
		if len(pos) != len(reach) {
			t.Fatalf("trial %d: %d steps for %d reachable tools", trial, len(pos), len(reach))
		}
		for id := range reach {
			for _, d := range deps[id] {
				if pos[d] >= pos[id] {
					t.Fatalf("trial %d: %s (step %d) must come after %s (step %d)", trial, id, pos[id], d, pos[d])
				}
			}
		}
		if p.Steps[len(p.Steps)-1].ToolID != "n00" {
			t.Fatalf("trial %d: the tool that was asked for is last", trial)
		}
	}
}

// What the plan lists for the UI.
func TestStepDetailsForTheDialog(t *testing.T) {
	p := mustPlan(t, ask("career", []Source{careerRepo()}), "browser:install@0.0.245 pdf_tools:install@0.0.124 career:install@0.0.806")
	for _, s := range p.Steps {
		if s.RepositoryID != "main" || s.SizeBytes != 100 || s.Name != "Tool" || s.Action != ActionInstall || s.InstalledVersion != "" {
			t.Errorf("%+v", s)
		}
	}
	if got := sorted(stepOf(p, "pdf_tools").RequiredBy); len(got) != 1 || got[0] != "career" {
		t.Fatalf("%v", got)
	}
}

// The plan settles over several rounds: a pin is dropped once the pinning tool joins the plan, and that only becomes known in a later round.
func TestAPinIsDroppedWhenThePinningToolJoinsThePlanLater(t *testing.T) {
	src := repo("main", "Main",
		tl("career", ver("1.0.0", needs("notes", "2.0.0"), needs("pdf_tools", "2.0.0"))),
		tl("notes", ver("2.0.0", needs("pdf_tools", "2.0.0"))),
		tl("pdf_tools", ver("2.5.0")),
	)
	old := installed("notes", "1.0.0").needs(req("pdf_tools", "1.0.0", "1.9.9"))
	mustPlan(t, ask("career", []Source{src}, installed("pdf_tools", "1.5.0"), old), "pdf_tools:update@2.5.0 notes:update@2.0.0 career:install@1.0.0")
	// when notes is not updated (it is not required), the pin holds
	src2 := repo("main", "Main", tl("career", ver("1.0.0", needs("pdf_tools", "2.0.0"))), tl("pdf_tools", ver("2.5.0")))
	mustPlan(t, ask("career", []Source{src2}, installed("pdf_tools", "1.5.0"), old), "!would_break_installed(pdf_tools)")
}

// A disabled installed tool still pins: enabling it later would fail against a version outside its range.
func TestADisabledInstalledToolStillPins(t *testing.T) {
	src := repo("main", "Main", tl("career", ver("1.0.0", needs("pdf_tools", "2.0.0"))), tl("pdf_tools", ver("2.5.0")))
	pinned := installed("notes", "1.0.0").disabled().needs(req("pdf_tools", "1.0.0", "1.9.9"))
	mustPlan(t, ask("career", []Source{src}, installed("pdf_tools", "1.5.0"), pinned), "!would_break_installed(pdf_tools)")
}

// A loop through the tool that was asked for, and a dependency that needs the root in a range it is not in.
func TestLoopsThroughTheRequestedTool(t *testing.T) {
	loop := repo("main", "Main", tl("a", ver("1.0.0", needs("b"))), tl("b", ver("1.0.0", needs("a"))))
	p := mustPlan(t, ask("a", []Source{loop}), "!cycle(a)")
	if !strings.Contains(problemOf(p, "a").Message, "a -> b -> a") {
		t.Fatalf("%s", problemOf(p, "a").Message)
	}
	wrong := repo("main", "Main", tl("a", ver("1.0.0", needs("b"))), tl("b", ver("1.0.0", needs("a", "2.0.0"))))
	p = mustPlan(t, ask("a", []Source{wrong}), "!conflicting_requirements(a)")
	if m := problemOf(p, "a").Message; !strings.Contains(m, "b needs a 2.0.0 or newer") || !strings.Contains(m, "a 1.0.0 would be installed") {
		t.Fatalf("%s", m)
	}
	// a range the root fits is only the loop
	fits := repo("main", "Main", tl("a", ver("1.0.0", needs("b"))), tl("b", ver("1.0.0", needs("a", "1.0.0"))))
	mustPlan(t, ask("a", []Source{fits}), "!cycle(a)")
}

func TestResolveDoesNotChangeItsInput(t *testing.T) {
	src := []Source{careerRepo(), repo("other", "Other", tl("extra", ver("1.0.0")))}
	inst := []InstalledTool{installed("pdf_tools", "0.0.0").running().needs(req("x", "1", "2")), installed("notes", "1.0.0").needs(req("pdf_tools", "0.0.1"))}
	in := Input{Installed: inst, Sources: src, Env: machine, Request: Request{RepositoryID: "main", ToolID: "career"}, Discovered: map[string][]Requirement{"career@0.0.806": {req("browser")}}}
	before := fmt.Sprintf("%+v|%+v|%+v", in.Installed, in.Sources[0].Catalog.Tools, in.Discovered)
	Resolve(in)
	Resolve(in)
	if after := fmt.Sprintf("%+v|%+v|%+v", in.Installed, in.Sources[0].Catalog.Tools, in.Discovered); after != before {
		t.Fatalf("the input changed:\n%s\n%s", before, after)
	}
}

// The problems of one plan are listed by tool, and a plan with problems has no steps.
func TestAPlanWithProblemsHasNoStepsAndListsEveryProblem(t *testing.T) {
	src := repo("main", "Main", tl("app", ver("1.0.0", needs("ghost"), needs("also_ghost"), needs("few", "9.0.0"))), tl("few", ver("1.0.0")))
	p := Resolve(ask("app", []Source{src}))
	if p.OK || len(p.Steps) != 0 {
		t.Fatalf("%+v", p)
	}
	if got := summary(p); got != "!not_in_any_repository(also_ghost) !no_version_in_range(few) !not_in_any_repository(ghost)" {
		t.Fatalf("every problem, sorted by tool: %s", got)
	}
}

// Two tools with different maximums: the lowest wins (and the highest minimum, tested above).
func TestTheLowestMaximumWins(t *testing.T) {
	src := repo("main", "Main",
		tl("app", ver("1.0.0", needs("b"), needs("c"))),
		tl("b", ver("1.0.0", needs("d", "1.0.0", "1.5.0"))),
		tl("c", ver("1.0.0", needs("d", "1.0.0", "2.0.0"))),
		tl("d", ver("2.0.0"), ver("1.5.0"), ver("1.0.0")),
	)
	mustPlan(t, ask("app", []Source{src}), "d:install@1.5.0 b:install@1.0.0 c:install@1.0.0 app:install@1.0.0")
	// the order the requirements are declared in does not matter
	flipped := repo("main", "Main",
		tl("app", ver("1.0.0", needs("c"), needs("b"))),
		tl("b", ver("1.0.0", needs("d", "1.0.0", "1.5.0"))),
		tl("c", ver("1.0.0", needs("d", "1.0.0", "2.0.0"))),
		tl("d", ver("2.0.0"), ver("1.5.0"), ver("1.0.0")),
	)
	mustPlan(t, ask("app", []Source{flipped}), "d:install@1.5.0 b:install@1.0.0 c:install@1.0.0 app:install@1.0.0")
}

// The publisher is the whole set of authors: one more or one fewer is a different publisher.
func TestPublishersWithADifferentNumberOfAuthorsDiffer(t *testing.T) {
	root := tl("career", ver("1.0.0", needs("browser", "2.0.0")))
	two := withAuthors(withAuthors(ver("2.0.0"), "Ada"), "Grace")
	mustPlan(t, ask("career", []Source{repo("main", "Main", root, tl("browser", two))}, installed("browser", "1.0.0").by("Ada")), "!publisher_changed(browser)")
	one := withAuthors(ver("2.0.0"), "Ada")
	mustPlan(t, ask("career", []Source{repo("main", "Main", root, tl("browser", one))}, installed("browser", "1.0.0").by("Ada", "Grace")), "!publisher_changed(browser)")
}

// The confirmation shows who made what it is about to install.
func TestStepsCarryTheAuthorsAndTheLicence(t *testing.T) {
	v := withAuthors(withAuthors(ver("1.0.0"), "Ada"), "Grace")
	v.License = "MIT"
	src := repo("main", "Main", tl("career", ver("1.0.0", needs("browser"), needs("notes"))), tl("browser", v), tl("notes", ver("1.0.0")))
	p := mustPlan(t, ask("career", []Source{src}, installed("notes", "1.0.0").disabled().by("Eve")), "browser:install@1.0.0 notes:enable career:install@1.0.0")
	if b := stepOf(p, "browser"); len(b.Authors) != 2 || b.Authors[0] != "Ada" || b.Authors[1] != "Grace" || b.License != "MIT" {
		t.Fatalf("%+v", b)
	}
	if n := stepOf(p, "notes"); len(n.Authors) != 1 || n.Authors[0] != "Eve" {
		t.Fatalf("an enable step shows what the installed tool declared: %+v", n)
	}
	if c := stepOf(p, "career"); len(c.Authors) != 0 || c.License != "" {
		t.Fatalf("nothing is invented: %+v", c)
	}
}

// S6 also holds for the tool that was asked for: updating Browser directly, past what an installed Career works with, is blocked, naming Career.
func TestUpdatingTheRequestedToolPastWhatAnInstalledToolWorksWithIsBlocked(t *testing.T) {
	src := repo("main", "Main", tl("browser", ver("2.0.0")))
	career := installed("career", "1.0.0").needs(req("browser", "1.0.0", "1.5.0"))
	p := mustPlan(t, ask("browser", []Source{src}, installed("browser", "1.0.0"), career), "!would_break_installed(browser)")
	if m := problemOf(p, "browser").Message; !strings.Contains(m, "career") || !strings.Contains(m, "updating") || !strings.Contains(m, "1.0.0 to 1.5.0") {
		t.Fatal(m)
	}
	// inside the range it is fine, and it still lists the dependents
	ok := repo("main", "Main", tl("browser", ver("1.4.0")))
	mustPlan(t, ask("browser", []Source{ok}, installed("browser", "1.0.0"), career), "browser:update@1.4.0")
}
