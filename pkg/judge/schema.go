package judge

import (
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// SchemaJudge validates that agent output is valid JSON conforming to a JSON Schema.
type SchemaJudge struct {
	Schema string `json:"schema" yaml:"schema"`
}

// Name returns the judge type identifier.
func (j *SchemaJudge) Name() string { return "schema" }

// Evaluate parses the output as JSON and validates it against the configured schema.
func (j *SchemaJudge) Evaluate(input Input) (Result, error) {
	var schemaDoc interface{}
	if err := json.Unmarshal([]byte(j.Schema), &schemaDoc); err != nil {
		return Result{}, fmt.Errorf("invalid JSON schema: %w", err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", schemaDoc); err != nil {
		return Result{}, fmt.Errorf("invalid JSON schema: %w", err)
	}
	sch, err := c.Compile("schema.json")
	if err != nil {
		return Result{}, fmt.Errorf("compiling JSON schema: %w", err)
	}

	var v interface{}
	if err := json.Unmarshal([]byte(input.Output), &v); err != nil {
		return Result{
			Pass:   false,
			Score:  0.0,
			Reason: fmt.Sprintf("output is not valid JSON: %v", err),
		}, nil
	}

	if err := sch.Validate(v); err != nil {
		return Result{
			Pass:   false,
			Score:  0.0,
			Reason: fmt.Sprintf("output does not match schema: %v", err),
		}, nil
	}

	return Result{
		Pass:   true,
		Score:  1.0,
		Reason: "output matches JSON schema",
	}, nil
}
