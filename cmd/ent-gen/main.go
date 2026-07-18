// Ent code generation entry point.
// Run `go run ./cmd/ent-gen` from project root to regenerate Ent code.

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	if err := entc.Generate("./internal/store/ent/schema", &gen.Config{}, entc.TemplateDir("")); err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
