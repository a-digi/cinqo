package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/toolrepository/fetch"
	"github.com/a-digi/cinqo/src/toolrepository/resolver"
)

var linux = resolver.Env{OS: "linux", Arch: "amd64"}

// pkgZip builds a package whose manifest says what it says.
func pkgZip(t *testing.T, manifest string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("manifest.json")
	w.Write([]byte(manifest))
	w, _ = zw.Create("frontend/bundle.js")
	w.Write([]byte("// js"))
	zw.Close()
	return buf.Bytes()
}

func man(slug, version, requires string) string {
	if requires != "" {
		requires = `,"requires_tools":` + requires
	}
	return fmt.Sprintf(`{"slug":%q,"name":%q,"version":%q,"authors":[{"name":"A"}],"license":"MIT"%s}`, slug, slug, version, requires)
}

// serving collects the packages a test repository lists, and serves them.
type serving struct {
	files map[string][]byte
	calls []string
	err   map[string]error
	tools []string
}

func newServing() *serving { return &serving{files: map[string][]byte{}, err: map[string]error{}} }

// add lists a tool at one version; fileRequires is what the repository FILE says it needs (empty: nothing), data the package.
func (sv *serving) add(id, version, fileRequires string, data []byte) {
	url := "https://example.com/pkgs/" + id + "-" + version + ".zip"
	sv.files[url] = data
	req := ""
	if fileRequires != "" {
		req = `"requires":` + fileRequires + `,`
	}
	sv.tools = append(sv.tools, fmt.Sprintf(`{"id":%q,"currentVersionId":"v1","versions":[{"versionId":"v1","name":%q,"version":%q,"authors":[{"name":"A"}],"license":"MIT",%s
	  "packages":[{"os":"any","arch":"any","url":%q,"sha256":%q,"sizeBytes":%d}]}]}`, id, id, version, req, url, hexSum(data), len(data)))
}

func (sv *serving) file(name string) string {
	return fmt.Sprintf(`{"formatVersion":2,"name":%q,"authors":[{"name":"R"}],"tools":[%s]}`, name, strings.Join(sv.tools, ","))
}

func (sv *serving) Get(_ context.Context, url string, _ int64, _ fetch.LocalPolicy) (*fetch.Result, error) {
	sv.calls = append(sv.calls, url)
	if err := sv.err[url]; err != nil {
		return nil, err
	}
	b, ok := sv.files[url]
	if !ok {
		return nil, errors.New("the server answered with status 404")
	}
	return &fetch.Result{Body: b}, nil
}

// setup registers one repository with the tools added so far and returns the service and the repository id.
func setup(t *testing.T, sv *serving) (*Service, string) {
	t.Helper()
	f := &fakeFetcher{files: map[string]string{link: sv.file("Main")}}
	s := newService(t, f)
	repo, err := s.Add(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	s.WithDownloader(sv)
	return s, repo.ID
}

func verify(t *testing.T, s *Service, id, tool string, installed ...resolver.InstalledTool) (*Verified, error) {
	t.Helper()
	return s.VerifyPlan(context.Background(), id, tool, "", installed, linux)
}

func order(p resolver.Plan) string {
	var out []string
	for _, st := range p.Steps {
		out = append(out, fmt.Sprintf("%s:%s@%s", st.ToolID, st.Action, st.Version))
	}
	return strings.Join(out, " ")
}

// careerServing is Career needing Browser and PDF tools >= 0.0.1, the file and the manifests agreeing.
func careerServing(t *testing.T) *serving {
	sv := newServing()
	sv.add("career", "0.0.806", `[{"id":"browser"},{"id":"pdf_tools","minVersion":"0.0.1"}]`, pkgZip(t, man("career", "0.0.806", `[{"slug":"browser"},{"slug":"pdf_tools","min_version":"0.0.1"}]`)))
	sv.add("browser", "0.0.245", "", pkgZip(t, man("browser", "0.0.245", "")))
	sv.add("pdf_tools", "0.0.124", "", pkgZip(t, man("pdf_tools", "0.0.124", "")))
	return sv
}

func TestEveryPackageOfThePlanIsDownloadedAndChecked(t *testing.T) {
	sv := careerServing(t)
	s, id := setup(t, sv)
	v, err := verify(t, s, id, "career")
	if err != nil || !v.Plan.OK {
		t.Fatalf("%v %+v", err, v)
	}
	if got := order(v.Plan); got != "browser:install@0.0.245 pdf_tools:install@0.0.124 career:install@0.0.806" {
		t.Fatalf("%s", got)
	}
	if len(sv.calls) != 3 {
		t.Fatalf("each package once: %v", sv.calls)
	}
	if v.Changed || len(v.Notes) != 0 || order(v.First) != order(v.Plan) {
		t.Fatalf("the files and the manifests agree: changed=%v %v", v.Changed, v.Notes)
	}
	if len(v.Packages) != 3 {
		t.Fatalf("%v", v.Packages)
	}
	c := v.Packages["career"]
	if c.Manifest.Slug != "career" || c.Version != "0.0.806" || c.VersionID != "v1" || c.RepositoryID != id || len(c.Data) == 0 ||
		c.Install.ToolID != "career" || c.Install.Version != "0.0.806" || c.Install.SHA256 != hexSum(c.Data) {
		t.Fatalf("%+v", c)
	}
}

func TestOnlyWhatIsNeededIsDownloaded(t *testing.T) {
	sv := careerServing(t)
	s, id := setup(t, sv)
	// browser is installed and fine: not downloaded; pdf_tools is disabled: only enabled, nothing to download
	v, err := verify(t, s, id, "career",
		resolver.InstalledTool{ID: "browser", Version: "0.0.245", Enabled: true},
		resolver.InstalledTool{ID: "pdf_tools", Version: "0.0.124", Enabled: false})
	if err != nil || order(v.Plan) != "pdf_tools:enable@0.0.124 career:install@0.0.806" {
		t.Fatalf("%v %v", err, order(v.Plan))
	}
	if len(sv.calls) != 1 || !strings.Contains(sv.calls[0], "career") {
		t.Fatalf("only Career is downloaded: %v", sv.calls)
	}
	if v.Packages["pdf_tools"] != nil || v.Packages["career"] == nil || len(v.Packages) != 1 {
		t.Fatalf("an enable step has no package: %v", v.Packages)
	}
}

// A repository file that says nothing about what a tool needs (older, or written by hand) is completed from the packages' own manifests.
func TestRequirementsAreDiscoveredFromThePackagesWhenTheFileSaysNothing(t *testing.T) {
	sv := newServing()
	sv.add("career", "1.0.0", "", pkgZip(t, man("career", "1.0.0", `[{"slug":"browser"}]`)))
	sv.add("browser", "1.0.0", "", pkgZip(t, man("browser", "1.0.0", `[{"slug":"pdf_tools","min_version":"1.0.0"}]`)))
	sv.add("pdf_tools", "1.0.0", "", pkgZip(t, man("pdf_tools", "1.0.0", "")))
	s, id := setup(t, sv)
	v, err := verify(t, s, id, "career")
	if err != nil {
		t.Fatal(err)
	}
	if order(v.First) != "career:install@1.0.0" {
		t.Fatalf("the file alone makes a one-step plan: %s", order(v.First))
	}
	if got := order(v.Plan); got != "pdf_tools:install@1.0.0 browser:install@1.0.0 career:install@1.0.0" || !v.Changed || len(v.Packages) != 3 {
		t.Fatalf("a three-level chain found by reading the packages: %s changed=%v", got, v.Changed)
	}
	if len(sv.calls) != 3 {
		t.Fatalf("each package once, however many rounds it took: %v", sv.calls)
	}
	if len(v.Notes) != 2 || !strings.Contains(v.Notes[0], "the package is used") {
		t.Fatalf("where the package and the file disagree is noted: %v", v.Notes)
	}
}

func TestAManifestThatNeedsMoreOrLessThanTheFileSaysChangesThePlan(t *testing.T) {
	// more: the file says browser, the package says browser and extra
	more := newServing()
	more.add("app", "1.0.0", `[{"id":"browser"}]`, pkgZip(t, man("app", "1.0.0", `[{"slug":"browser"},{"slug":"extra"}]`)))
	more.add("browser", "1.0.0", "", pkgZip(t, man("browser", "1.0.0", "")))
	more.add("extra", "1.0.0", "", pkgZip(t, man("extra", "1.0.0", "")))
	s, id := setup(t, more)
	v, err := verify(t, s, id, "app")
	if err != nil || order(v.Plan) != "browser:install@1.0.0 extra:install@1.0.0 app:install@1.0.0" || !v.Changed || order(v.First) != "browser:install@1.0.0 app:install@1.0.0" {
		t.Fatalf("%v %s | %s", err, order(v.Plan), order(v.First))
	}
	// less: the file says browser, the package says nothing: the plan shrinks and the unneeded package is not part of the result
	less := newServing()
	less.add("app", "1.0.0", `[{"id":"browser"}]`, pkgZip(t, man("app", "1.0.0", `[]`)))
	less.add("browser", "1.0.0", "", pkgZip(t, man("browser", "1.0.0", "")))
	s, id = setup(t, less)
	v, err = verify(t, s, id, "app")
	if err != nil || order(v.Plan) != "app:install@1.0.0" || !v.Changed || len(v.Packages) != 1 || v.Packages["browser"] != nil {
		t.Fatalf("%v %s %v", err, order(v.Plan), v.Packages)
	}
	// the same: nothing changed
	same := careerServing(t)
	s, id = setup(t, same)
	if v, _ := verify(t, s, id, "career"); v.Changed {
		t.Fatal("the same requirements are not a change")
	}
}

func TestAManifestThatNeedsSomethingNobodyOffersMakesAPlanWithProblems(t *testing.T) {
	sv := newServing()
	sv.add("app", "1.0.0", "", pkgZip(t, man("app", "1.0.0", `[{"slug":"ghost"}]`)))
	s, id := setup(t, sv)
	v, err := verify(t, s, id, "app")
	if err != nil || v.Plan.OK || len(v.Packages) != 0 || !v.Changed || len(v.Plan.Problems) != 1 || v.Plan.Problems[0].ToolID != "ghost" {
		t.Fatalf("%v %+v", err, v)
	}
}

func TestAPlanThatCannotBeMadeDownloadsNothing(t *testing.T) {
	sv := newServing()
	sv.add("app", "1.0.0", `[{"id":"ghost"}]`, pkgZip(t, man("app", "1.0.0", `[{"slug":"ghost"}]`)))
	s, id := setup(t, sv)
	v, err := verify(t, s, id, "app")
	if err != nil || v.Plan.OK || len(sv.calls) != 0 || len(v.Packages) != 0 || v.Changed {
		t.Fatalf("%v calls=%v %+v", err, sv.calls, v)
	}
	if v.First.OK || len(v.First.Problems) != 1 {
		t.Fatalf("%+v", v.First)
	}
}

// A package that is not what the repository promised stops everything.
func TestAPackageThatIsNotWhatTheRepositoryListedIsRefused(t *testing.T) {
	cases := map[string]struct {
		data []byte
		want string
	}{
		"another tool":        {pkgZip(t, man("other", "1.0.0", "")), `is the tool "other"`},
		"another version":     {pkgZip(t, man("app", "2.0.0", "")), "is version 2.0.0"},
		"no manifest":         {zipWithout(t), "no manifest.json"},
		"not a zip":           {[]byte("plain"), "cannot be read"},
		"requires itself":     {pkgZip(t, man("app", "1.0.0", `[{"slug":"app"}]`)), "cannot be used"},
		"bound the wrong way": {pkgZip(t, man("app", "1.0.0", `[{"slug":"x","min_version":"2.0.0","max_version":"1.0.0"}]`)), "cannot be used"},
		"another publisher":   {pkgZip(t, strings.Replace(man("app", "1.0.0", ""), `"name":"A"`, `"name":"Eve"`, 1)), "authors"},
		"another licence":     {pkgZip(t, strings.Replace(man("app", "1.0.0", ""), `"MIT"`, `"GPL-3.0"`, 1)), "license"},
	}
	for name, c := range cases {
		sv := newServing()
		sv.add("app", "1.0.0", "", c.data)
		s, id := setup(t, sv)
		v, err := verify(t, s, id, "app")
		if !errors.Is(err, ErrPackageMismatch) || v != nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: v=%v err=%v, want ErrPackageMismatch mentioning %q", name, v, err, c.want)
		}
	}
}

func zipWithout(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("frontend/bundle.js")
	w.Write([]byte("x"))
	zw.Close()
	return buf.Bytes()
}

func TestADownloadThatFailsStopsTheWholePhase(t *testing.T) {
	// the second package cannot be fetched
	sv := careerServing(t)
	s, id := setup(t, sv)
	sv.err["https://example.com/pkgs/pdf_tools-0.0.124.zip"] = errors.New("the server could not be reached")
	v, err := verify(t, s, id, "career")
	if !errors.Is(err, ErrDownload) || v != nil || !strings.Contains(err.Error(), "could not be reached") {
		t.Fatalf("%v %v", v, err)
	}
	// a package that does not match its checksum
	sv = careerServing(t)
	s, id = setup(t, sv)
	sv.files["https://example.com/pkgs/browser-0.0.245.zip"] = pkgZip(t, man("browser", "0.0.245", `[]`)) // different bytes than the file listed
	if v, err := verify(t, s, id, "career"); !errors.Is(err, ErrDownload) || v != nil {
		t.Fatalf("%v %v", v, err)
	}
	// 404
	sv = careerServing(t)
	s, id = setup(t, sv)
	delete(sv.files, "https://example.com/pkgs/career-0.0.806.zip")
	if _, err := verify(t, s, id, "career"); !errors.Is(err, ErrDownload) {
		t.Fatalf("%v", err)
	}
}

func TestThePlansTotalDownloadIsBounded(t *testing.T) {
	old := MaxPlanDownloadBytes
	defer func() { MaxPlanDownloadBytes = old }()
	sv := careerServing(t)
	s, id := setup(t, sv)
	// the listed sizes already exceed it: nothing is downloaded
	MaxPlanDownloadBytes = 100
	v, err := verify(t, s, id, "career")
	if !errors.Is(err, ErrTooLarge) || v != nil || len(sv.calls) != 0 {
		t.Fatalf("%v %v calls=%v", v, err, sv.calls)
	}
	// exactly the listed total is fine
	var total int64
	for _, b := range sv.files {
		total += int64(len(b))
	}
	MaxPlanDownloadBytes = total
	if _, err := verify(t, s, id, "career"); err != nil {
		t.Fatalf("exactly the limit is allowed: %v", err)
	}
	MaxPlanDownloadBytes = total - 1
	if _, err := verify(t, s, id, "career"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("one byte more: %v", err)
	}
	// sizes the repository does not list are still counted as they arrive
	free := newServing()
	free.add("app", "1.0.0", `[{"id":"b"}]`, pkgZip(t, man("app", "1.0.0", `[{"slug":"b"}]`)))
	free.add("b", "1.0.0", "", pkgZip(t, man("b", "1.0.0", "")))
	for i := range free.tools {
		free.tools[i] = strings.ReplaceAll(free.tools[i], `"sizeBytes"`, `"unusedField"`)
	}
	s, id = setup(t, free)
	MaxPlanDownloadBytes = 10
	if _, err := verify(t, s, id, "app"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("unlisted sizes are counted when the bytes arrive: %v", err)
	}
}

// The depth cap applies to what the manifests reveal too.
func TestAChainOnlyTheManifestsRevealIsStillBounded(t *testing.T) {
	sv := newServing()
	n := resolver.MaxDepth + 1
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("t%02d", i)
		req := ""
		if i < n-1 {
			req = fmt.Sprintf(`[{"slug":"t%02d"}]`, i+1)
		}
		sv.add(id, "1.0.0", "", pkgZip(t, man(id, "1.0.0", req)))
	}
	s, id := setup(t, sv)
	v, err := verify(t, s, id, "t00")
	if err != nil || v.Plan.OK || v.Plan.Problems[0].Kind != resolver.ProblemTooDeep {
		t.Fatalf("%v %+v", err, v)
	}
}

func TestVerifyAnswersLikeThePlanForWhatDoesNotExist(t *testing.T) {
	sv := careerServing(t)
	s, id := setup(t, sv)
	if _, err := s.VerifyPlan(context.Background(), "nope", "career", "", nil, linux); !errors.Is(err, ErrNotFound) {
		t.Errorf("%v", err)
	}
	if _, err := s.VerifyPlan(context.Background(), id, "nope", "", nil, linux); !errors.Is(err, ErrToolNotFound) {
		t.Errorf("%v", err)
	}
	if _, err := s.VerifyPlan(context.Background(), id, "career", "zzz", nil, linux); !errors.Is(err, ErrVersionMissing) {
		t.Errorf("%v", err)
	}
	if len(sv.calls) != 0 {
		t.Fatalf("nothing is downloaded for a request that does not make sense: %v", sv.calls)
	}
}

// An update of an installed tool is verified like an install.
func TestAnUpdateStepIsDownloadedAndVerifiedToo(t *testing.T) {
	sv := careerServing(t)
	s, id := setup(t, sv)
	v, err := verify(t, s, id, "career",
		resolver.InstalledTool{ID: "browser", Version: "0.0.245", Enabled: true},
		resolver.InstalledTool{ID: "pdf_tools", Version: "0.0.0", Enabled: true})
	if err != nil || order(v.Plan) != "pdf_tools:update@0.0.124 career:install@0.0.806" || v.Packages["pdf_tools"] == nil || v.Packages["pdf_tools"].Manifest.Version != "0.0.124" {
		t.Fatalf("%v %s", err, order(v.Plan))
	}
}

// what the install pipeline is told to expect: what the repository said about the version, and whether the repository is on a local address
func TestTheInstallExpectationCarriesWhatTheRepositoryClaimedAndWhereItCameFrom(t *testing.T) {
	sv := careerServing(t)
	f := &fakeFetcher{files: map[string]string{link: sv.file("Main")}, local: true}
	s := newService(t, f)
	repo, err := s.Add(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	s.WithDownloader(sv)
	v, err := verify(t, s, repo.ID, "career")
	if err != nil {
		t.Fatal(err)
	}
	c := v.Packages["career"].Install
	if !c.Local || c.Info.License != "MIT" || len(c.Info.Authors) != 1 || c.Info.Authors[0].Name != "A" {
		t.Fatalf("%+v", c)
	}
	// a repository that is not local says so
	s, id := setup(t, careerServing(t))
	v, _ = verify(t, s, id, "career")
	if v.Packages["career"].Install.Local {
		t.Fatal("a public repository's packages are not downloaded as local ones")
	}
}

// the same tools, a different version of one of them: that is a change too
func TestADifferentVersionOfTheSameToolIsAChange(t *testing.T) {
	sv := newServing()
	sv.add("app", "1.0.0", `[{"id":"browser"}]`, pkgZip(t, man("app", "1.0.0", `[{"slug":"browser","min_version":"2.0.0"}]`)))
	// browser lists two versions: 1.0.0 is current, 2.0.0 is newer
	b1, b2 := pkgZip(t, man("browser", "1.0.0", "")), pkgZip(t, man("browser", "2.0.0", ""))
	sv.files["https://example.com/pkgs/browser-1.0.0.zip"], sv.files["https://example.com/pkgs/browser-2.0.0.zip"] = b1, b2
	sv.tools = append(sv.tools, fmt.Sprintf(`{"id":"browser","currentVersionId":"v1","versions":[
	  {"versionId":"v1","name":"browser","version":"1.0.0","authors":[{"name":"A"}],"license":"MIT","packages":[{"os":"any","arch":"any","url":"https://example.com/pkgs/browser-1.0.0.zip","sha256":%q}]},
	  {"versionId":"v2","name":"browser","version":"2.0.0","authors":[{"name":"A"}],"license":"MIT","packages":[{"os":"any","arch":"any","url":"https://example.com/pkgs/browser-2.0.0.zip","sha256":%q}]}]}`, hexSum(b1), hexSum(b2)))
	s, id := setup(t, sv)
	v, err := verify(t, s, id, "app")
	if err != nil {
		t.Fatal(err)
	}
	if order(v.First) != "browser:install@1.0.0 app:install@1.0.0" || order(v.Plan) != "browser:install@2.0.0 app:install@1.0.0" || !v.Changed {
		t.Fatalf("%s | %s changed=%v", order(v.First), order(v.Plan), v.Changed)
	}
	if v.Packages["browser"].Version != "2.0.0" {
		t.Fatalf("the package of the version that is planned: %+v", v.Packages["browser"])
	}
}

// the notes say plainly where the two disagree, also when the plan stays the same
func TestTheNotesSayWhereThePackageAndTheFileDisagree(t *testing.T) {
	sv := newServing()
	sv.add("app", "1.0.0", `[{"id":"browser","minVersion":"1.0.0"}]`, pkgZip(t, man("app", "1.0.0", `[{"slug":"browser","min_version":"1.0.0","max_version":"2.0.0"}]`)))
	sv.add("browser", "1.0.0", "", pkgZip(t, man("browser", "1.0.0", "")))
	s, id := setup(t, sv)
	v, err := verify(t, s, id, "app")
	if err != nil || v.Changed || len(v.Notes) != 1 {
		t.Fatalf("the plan is the same, the disagreement is only noted: %v %v %v", err, v.Changed, v.Notes)
	}
	if n := v.Notes[0]; !strings.Contains(n, "app 1.0.0") || !strings.Contains(n, "browser 1.0.0 or newer") || !strings.Contains(n, "browser 1.0.0 to 2.0.0") || !strings.Contains(n, "the package is used") {
		t.Fatalf("%s", n)
	}
	// a file that says nothing against a package that needs something, and the other way round
	bare := newServing()
	bare.add("app", "1.0.0", "", pkgZip(t, man("app", "1.0.0", `[{"slug":"browser"}]`)))
	bare.add("browser", "1.0.0", "", pkgZip(t, man("browser", "1.0.0", "")))
	s, id = setup(t, bare)
	v, _ = verify(t, s, id, "app")
	if len(v.Notes) != 1 || !strings.Contains(v.Notes[0], "needing nothing, the package says browser") {
		t.Fatalf("%v", v.Notes)
	}
	none := newServing()
	none.add("app", "1.0.0", `[{"id":"browser"},{"id":"x","maxVersion":"3.0.0"}]`, pkgZip(t, man("app", "1.0.0", `[]`)))
	none.add("browser", "1.0.0", "", pkgZip(t, man("browser", "1.0.0", "")))
	none.add("x", "1.0.0", "", pkgZip(t, man("x", "1.0.0", "")))
	s, id = setup(t, none)
	v, _ = verify(t, s, id, "app")
	if len(v.Notes) != 1 || !strings.Contains(v.Notes[0], "browser, x 3.0.0 or older") || !strings.Contains(v.Notes[0], "the package says nothing") {
		t.Fatalf("%v", v.Notes)
	}
}
