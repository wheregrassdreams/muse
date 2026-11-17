// Command mdshot renders a Markdown or text file with the native renderer
// and writes a PNG, without opening a window.
//
// It is how the document renderer is checked visually while developing:
//
//	go run ./cmd/mdshot -o doc.png ../docs/native-migration.md
package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"

	"github.com/egoist/mygo/ui"

	"muse/markdown"
	markdownui "muse/nativeui/components/markdown"
	"muse/platform/filesystem/imagefiles"
)

func main() {
	width := flag.Int("w", 900, "image width in DIPs")
	height := flag.Int("h", 1400, "image height in DIPs")
	out := flag.String("o", "doc.png", "output PNG path")
	dark := flag.Bool("dark", false, "render on the dark theme")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: mdshot [flags] <file.md>")
		os.Exit(2)
	}
	path := flag.Arg(0)
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	doc := markdown.Parse(string(src))
	localFiles := imagefiles.New()
	tt := ui.NewTester(func(c *ui.Context) {
		markdownui.View(c, doc, markdownui.Options{LocalFiles: localFiles})
	}, *width, *height)
	tt.SetDark(*dark)
	tt.Frame()

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%dx%d), %d blocks\n", *out, *width, *height, len(doc.Blocks))
}
