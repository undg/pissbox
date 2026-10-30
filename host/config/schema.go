package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const globalSchemaURL = "https://pissbox.local/schemas/global-config.json"

//go:embed global-schema.json
var globalSchemaJSON []byte

var globalSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileSchema(globalSchemaURL, globalSchemaJSON)
})

// compileSchema keeps global and future project schemas on one in-memory path.
func compileSchema(url string, data []byte) (*jsonschema.Schema, error) {
	decoded, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode embedded schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(url, decoded); err != nil {
		return nil, fmt.Errorf("add embedded schema: %w", err)
	}
	schema, err := compiler.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("compile embedded schema: %w", err)
	}
	return schema, nil
}

func validateSchema(value any) error {
	schema, err := globalSchema()
	if err != nil {
		return err
	}
	return schema.Validate(value)
}
