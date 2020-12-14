package godartsass

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	qt "github.com/frankban/quicktest"
)

const (
	// https://github.com/sass/dart-sass-embedded/releases
	// TODO1
	dartSassEmbeddedFilename = "/Users/bep/Downloads/sass_embedded/dart-sass-embedded"

	sassSample = `nav {
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  li { display: inline-block; }

  a {
    display: block;
    padding: 6px 12px;
    text-decoration: none;
  }
}`
	sassSampleTranspiled = "nav ul {\n  margin: 0;\n  padding: 0;\n  list-style: none;\n}\nnav li {\n  display: inline-block;\n}\nnav a {\n  display: block;\n  padding: 6px 12px;\n  text-decoration: none;\n}"
)

type testImportResolver struct {
	name    string
	content string
}

func (t testImportResolver) CanonicalizeURL(url string) string {
	if url != t.name {
		return ""
	}

	return url
}

func (t testImportResolver) Load(url string) string {
	if !strings.Contains(url, t.name) {
		panic("protocol error")
	}
	return t.content
}

func TestTranspilerVariants(t *testing.T) {
	c := qt.New(t)

	colorsResolver := testImportResolver{
		name:    "colors",
		content: `$white:    #ffff`,
	}

	for _, test := range []struct {
		name   string
		opts   Options
		args   Args
		expect interface{}
	}{
		{"Output style compressed", Options{}, Args{Source: "div { color: #ccc; }", OutputStyle: OutputStyleCompressed}, "div{color:#ccc}"},
		{"Invalid syntax", Options{}, Args{Source: "div { color: $white; }"}, false},
		{"Import not found", Options{}, Args{Source: "@import \"foo\""}, false},
		{"Sass syntax", Options{}, Args{
			Source: `$font-stack:    Helvetica, sans-serif
$primary-color: #333

body
  font: 100% $font-stack
  color: $primary-color
`,
			OutputStyle:  OutputStyleCompressed,
			SourceSyntax: SourceSyntaxSASS,
		}, "body{font:100% Helvetica,sans-serif;color:#333}"},
		{"Import resolver", Options{ImportResolver: colorsResolver}, Args{Source: "@import \"colors\";\ndiv { p { color: $white; } }"}, "div p {\n  color: #ffff;\n}"},
		//{"Precision", Options{Precision: 3}, "div { width: percentage(1 / 3); }", "div {\n  width: 33.333%; }\n"},
	} {

		test := test
		c.Run(test.name, func(c *qt.C) {
			b, ok := test.expect.(bool)
			shouldFail := ok && !b
			transpiler, clean := newTestTranspiler(c, test.opts)
			defer clean()
			result, err := transpiler.Execute(test.args)
			if shouldFail {
				c.Assert(err, qt.Not(qt.IsNil))
			} else {
				c.Assert(err, qt.IsNil)
				c.Assert(result.CSS, qt.Equals, test.expect)
			}
		})

	}
}

func TestTranspilerParallel(t *testing.T) {
	c := qt.New(t)
	transpiler, clean := newTestTranspiler(c, Options{})
	defer clean()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				src := fmt.Sprintf(`
$primary-color: #%03d;

div { color: $primary-color; }`, num)

				result, err := transpiler.Execute(Args{Source: src})
				c.Check(err, qt.IsNil)
				c.Check(result.CSS, qt.Equals, fmt.Sprintf("div {\n  color: #%03d;\n}", num))
				if c.Failed() {
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

func newTestTranspiler(c *qt.C, opts Options) (*Transpiler, func()) {
	opts.DartSassEmbeddedFilename = dartSassEmbeddedFilename
	transpiler, err := Start(opts)
	c.Assert(err, qt.IsNil)

	return transpiler, func() {
		c.Assert(transpiler.Close(), qt.IsNil)
	}
}

func BenchmarkTranspiler(b *testing.B) {
	type tester struct {
		src        string
		expect     string
		transpiler *Transpiler
		clean      func()
	}

	newTester := func(b *testing.B, opts Options) tester {
		c := qt.New(b)
		transpiler, clean := newTestTranspiler(c, Options{})

		return tester{
			transpiler: transpiler,
			clean:      clean,
		}
	}

	runBench := func(b *testing.B, t tester) {
		defer t.clean()
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			result, err := t.transpiler.Execute(Args{Source: t.src})
			if err != nil {
				b.Fatal(err)
			}
			if result.CSS != t.expect {
				b.Fatalf("Got: %q\n", result.CSS)
			}
		}
	}

	b.Run("SCSS", func(b *testing.B) {
		t := newTester(b, Options{})
		t.src = sassSample
		t.expect = sassSampleTranspiled
		runBench(b, t)
	})

	b.Run("SCSS Parallel", func(b *testing.B) {
		t := newTester(b, Options{})
		t.src = sassSample
		t.expect = sassSampleTranspiled
		defer t.clean()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				result, err := t.transpiler.Execute(Args{Source: t.src})
				if err != nil {
					b.Fatal(err)
				}
				if result.CSS != t.expect {
					b.Fatalf("Got: %q\n", result.CSS)
				}
			}
		})
	})
}