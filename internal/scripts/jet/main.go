//go:build jet

package main

import (
	"log/slog"
	"os"

	"github.com/go-jet/jet/v2/generator/metadata"
	"github.com/go-jet/jet/v2/generator/postgres"
	"github.com/go-jet/jet/v2/generator/template"
	postgres2 "github.com/go-jet/jet/v2/postgres"
	_ "github.com/joho/godotenv/autoload"
	_ "github.com/lib/pq"
)

func main() {
	err := postgres.GenerateDSN(
		os.Getenv("DATABASE_URL"),
		"todo",
		"./internal/pkg/",
		template.Default(postgres2.Dialect).
			UseSchema(func(schemaMetaData metadata.Schema) template.Schema {
				return template.DefaultSchema(schemaMetaData).
					UseModel(template.Model{
						Skip: true,
					})
			}),
	)

	if err != nil {
		slog.Error("Error running jet: %v", err)
		os.Exit(1)
	}
}
