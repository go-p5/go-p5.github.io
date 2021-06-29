// Copyright ©2021 The go-p5 Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	log.SetPrefix("gen-p5: ")
	log.SetFlags(0)

	vers := flag.String("vers", "main", "version of go-p5/p5 to generate examples for")

	flag.Parse()

	gen(*vers)
}

var excludes = map[string]struct{}{
	"sketch":     {},
	"wasm-p5-ex": {},
}

func gen(vers string) {
	tmp, err := os.MkdirTemp("", "go-p5-gen-")
	if err != nil {
		log.Fatalf("could not create tmp dir: %+v", err)
	}
	defer os.RemoveAll(tmp)

	cmd := exec.Command("git", "clone", "--depth=1", "-b", vers, "https://github.com/go-p5/p5")
	cmd.Dir = tmp
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		log.Fatalf("could not initialize module: %+v", err)
	}

	revision, err := fetchRevision(filepath.Join(tmp, "p5"))
	if err != nil {
		log.Fatalf("could not retrieve git revision:\n%s\nerr: %+v", revision, err)
	}
	log.Printf("revision: %q", revision)

	root := new(strings.Builder)
	root.WriteString(fmt.Sprintf(rootHeader, revision))

	js, err := loadWASM()
	if err != nil {
		log.Fatalf("could not find WASM bootstrap code: %+v", err)
	}
	err = os.MkdirAll("assets", 0755)
	if err != nil {
		log.Fatalf("could not create 'assets': %+v", err)
	}

	err = os.WriteFile(filepath.Join("assets", "wasm_exec.js"), js, 0644)
	if err != nil {
		log.Fatalf("could not write wasm_exec.js: %+v", err)
	}

	pkgs, err := os.ReadDir(filepath.Join(tmp, "p5", "example"))
	if err != nil {
		log.Fatalf("could not read dir: %+v", err)
	}
	for _, p := range pkgs {
		log.Printf(">>> %+v", p.Name())
	}

	for _, dir := range pkgs {
		if _, ok := excludes[dir.Name()]; ok {
			log.Printf("ignoring %s...", dir.Name())
			continue
		}
		name := "example/" + dir.Name()
		log.Printf("generating %s...", name)
		pkg := filepath.Base(name)
		cmd := exec.Command("go", "build", "-o", "../bin/"+pkg+".wasm", "./"+name)
		cmd.Dir = filepath.Join(tmp, "p5")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = []string{
			"GOBIN=" + tmp + "/bin",
			"GOOS=js",
			"GOARCH=wasm",
		}
		cmd.Env = append(cmd.Env, os.Environ()...)

		err = cmd.Run()
		if err != nil {
			log.Fatalf("could not build WASM %q: %+v", name, err)
		}

		err = os.MkdirAll(name, 0755)
		if err != nil {
			log.Fatalf("could not create source dir %q: %+v", name, err)
		}

		title := "Go-P5: " + pkg
		src := fmt.Sprintf("https://go-p5.github.io/example/%s/%s.wasm", pkg, pkg)
		err = os.WriteFile(
			filepath.Join(name, "index.html"),
			[]byte(fmt.Sprintf(indexHTML, title, src)),
			0644,
		)
		if err != nil {
			log.Fatalf("could not write example HTML %q: %+v", name, err)
		}

		fname := filepath.Join(name, pkg+".wasm")
		wasm, err := os.ReadFile(filepath.Join(tmp, "bin", pkg+".wasm"))
		if err != nil {
			log.Fatalf("could not read WASM binary %q: %+v", name, err)
		}
		err = os.WriteFile(fname, wasm, 0644)
		if err != nil {
			log.Fatalf("could not write WASM binary %q: %+v", name, err)
		}

		err = exec.Command("git", "add", fname).Run()
		if err != nil {
			log.Fatalf("could not add WASM binary to repository: %+v", err)
		}

		root.WriteString(fmt.Sprintf(
			"<li><a href=%q>%s</a></li>\n",
			"https://go-p5.github.io/example/"+pkg+"/index.html",
			pkg,
		))
	}

	root.WriteString(rootFooter)
	err = os.WriteFile("index.html", []byte(root.String()), 0644)
	if err != nil {
		log.Fatalf("could not create root index: %+v", err)
	}
}

func loadWASM() (js []byte, err error) {
	root := filepath.Join(runtime.GOROOT(), "misc", "wasm")
	js, err = os.ReadFile(filepath.Join(root, "wasm_exec.js"))
	if err != nil {
		return
	}
	return
}

const indexHTML = `
<!doctype html>
<!--
Copyright 2018 The Go Authors. All rights reserved.
Use of this source code is governed by a BSD-style
license that can be found in the LICENSE file.
-->
<html>

<head>
        <meta charset="utf-8">
        <title>%s</title>
</head>

<body>
        <!--
        Add the following polyfill for Microsoft Edge 17/18 support:
        <script src="https://cdn.jsdelivr.net/npm/text-encoding@0.7.0/lib/encoding.min.js"></script>
        (see https://caniuse.com/#feat=textencoder)
        -->
		<script src="https://go-p5.github.io/assets/wasm_exec.js"></script>
        <script>
                if (!WebAssembly.instantiateStreaming) { // polyfill
                        WebAssembly.instantiateStreaming = async (resp, importObject) => {
                                const source = await (await resp).arrayBuffer();
                                return await WebAssembly.instantiate(source, importObject);
                        };
                }

                const go = new Go();
                let mod, inst;
                WebAssembly.instantiateStreaming(fetch("%s"), go.importObject).then((result) => {
                        mod = result.module;
                        inst = result.instance;
                        document.getElementById("runButton").disabled = false;
                }).catch((err) => {
                        console.error(err);
                });

                async function run() {
                        console.clear();
                        await go.run(inst);
                        inst = await WebAssembly.instantiate(mod, go.importObject); // reset instance
                }
        </script>

        <button onClick="run();" id="runButton" disabled>Run</button>
</body>

</html>
`

const rootHeader = `
<!doctype html>
<html>
<head>
        <meta charset="utf-8">
        <title>Go-P5</title>
</head>

<body>
<h2>Welcome to the Go-P5 examples page (version=%s)</h2>
This page shows a few <code>go-p5</code> examples, compiled to <code>WASM</code>.

<ul>
`

const rootFooter = `
</ul>
</body>

</html>
`

func fetchRevision(dir string) (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--always")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
