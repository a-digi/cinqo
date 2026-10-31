// repotool builds a tool repository file from the packaged tools in
// tools/*/versions/ — see plan/ai/tools/repository/tool-repositories.md.
//
//	go run ./cmd/repotool -tools-dir ../tools -out-dir ../dist/repository
//
// With -in-place the file lands in the tools folder itself and links each package
// at <tool>/versions/<zip>, so serving that folder is enough.
//
// Host the output folder with any static file server and add the link to
// repository.json on the Repositories page.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/a-digi/cinqo/src/toolrepository/publish"
)

func main() {
	var opts publish.Options
	flag.StringVar(&opts.ToolsDir, "tools-dir", "tools", "folder holding one folder per tool, each with a versions/ folder of packaged zips")
	flag.StringVar(&opts.OutDir, "out-dir", "dist/repository", "folder to write the repository file and the packages into")
	flag.StringVar(&opts.FileName, "file-name", publish.FileName, "name of the repository file")
	flag.StringVar(&opts.Name, "name", "", "repository name shown in the app")
	flag.StringVar(&opts.Description, "description", "", "repository description")
	flag.StringVar(&opts.BaseURL, "base-url", "", "address the output folder will be served from; leave empty for links relative to the repository file")
	flag.StringVar(&opts.AssetsURL, "assets-url", "", "serve every package flat from this address, e.g. https://github.com/<owner>/<repo>/releases/download/<tag>; links become <assets-url>/<zip>")
	flag.StringVar(&opts.MetaFile, "meta", "", "the repository's own facts (name, description, authors, company, links); default <tools-dir>/repository.meta.json when it exists")
	flag.BoolVar(&opts.RequireAuthors, "require-authors", false, "fail, instead of writing the older format 1, when the repository or a tool version has no authors or licence")
	flag.BoolVar(&opts.InPlace, "in-place", false, "write the repository file into -tools-dir and link packages where they are, copying nothing")
	flag.Parse()
	opts.Now = time.Now

	res, err := publish.Generate(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "repotool:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s: format %d, %d tools, %d packages\n", res.File, res.Catalog.FormatVersion, len(res.Catalog.Tools), res.Packages)
	if len(res.Warnings) > 0 {
		fmt.Printf("written in the older format 1 because who made it is missing (use -require-authors to make this an error):\n")
		for _, w := range res.Warnings {
			fmt.Printf("  ! %s\n", w)
		}
	}
	for _, t := range res.Catalog.Tools {
		fmt.Printf("  %-14s current %s (%d versions)\n", t.ID, t.CurrentVersionID, len(t.Versions))
	}
}
