// cph-assemble 只复制已有产物，不触发 core 重新编译。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/ShadowSmallBaby/ClawProxyHub/internal/distribution"
	"os"
)

func main() {
	input := flag.String("manifest", "", "assembly request JSON")
	out := flag.String("out", "", "new output directory")
	flag.Parse()
	if err := run(*input, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(input, out string) error {
	if input == "" || out == "" {
		return fmt.Errorf("--manifest and --out required")
	}
	b, e := os.ReadFile(input)
	if e != nil {
		return e
	}
	var r distribution.Request
	if e = json.Unmarshal(b, &r); e != nil {
		return e
	}
	l, e := distribution.Assemble(r, out)
	if e == nil {
		e = json.NewEncoder(os.Stdout).Encode(l)
	}
	return e
}
