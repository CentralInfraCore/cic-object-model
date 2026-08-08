// Command cic-materialize runs the materialization pipeline of SPEC §8 over
// a schema and an authoring input, or the final validation of SPEC §8.7 over
// an already-canonical object.
//
//	cic-materialize -schema schema.yaml -input input.yaml
//	cic-materialize -validate object.yaml
//
// It is also what makes `deadcode ./...` meaningful: without a main package
// every exported symbol is trivially unreachable.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/CentralInfraCore/cic-object-model/go/module"
	"github.com/CentralInfraCore/cic-object-model/go/objectmodel"
)

func main() {
	schemaPath := flag.String("schema", "", "path to the CIC schema")
	inputPath := flag.String("input", "", "path to the authoring-plane input")
	validatePath := flag.String("validate", "", "path to a canonical object to validate (SPEC §8.7)")
	deliver := flag.Bool("deliver", false, "hand the result to the module boundary (SPEC §9)")
	flag.Parse()

	if err := run(*schemaPath, *inputPath, *validatePath, *deliver); err != nil {
		emitError(err)
		os.Exit(1)
	}
}

// errorEnvelope is the machine-readable shape of a rejection: the same four
// fields conformance/*/expected-error.yaml asserts on, plus prose for a human.
// A second implementation reproduces this, so it is a contract and not a log
// line.
type errorEnvelope struct {
	Error struct {
		Code      string `yaml:"code"`
		Invariant string `yaml:"invariant"`
		Stage     string `yaml:"stage"`
		Path      string `yaml:"path"`
		Detail    string `yaml:"detail"`
	} `yaml:"error"`
}

// emitError writes the envelope as YAML through the marshaller rather than
// with a format string.
//
// The format-string version produced invalid YAML the moment a detail began
// with a backtick — "detail: `priority` is not a member..." — because YAML
// cannot start a plain scalar that way. The envelope claimed to be
// machine-readable and was not, for a whole class of messages. Found by
// cli_golden_test.go, which is the reason to pin a CLI contract at all.
func emitError(err error) {
	var env errorEnvelope
	var me *objectmodel.Error
	if errors.As(err, &me) {
		env.Error.Code = me.Code
		env.Error.Invariant = me.Invariant
		env.Error.Stage = string(me.Stage)
		env.Error.Path = me.Path
		env.Error.Detail = me.Detail
	} else {
		// Not a pipeline rejection — a missing file, a usage mistake. It still
		// arrives in the same envelope so a caller has one shape to parse.
		env.Error.Code = "E_CLI"
		env.Error.Detail = err.Error()
	}
	out, merr := yaml.Marshal(env)
	if merr != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return
	}
	os.Stderr.Write(out)
}

func run(schemaPath, inputPath, validatePath string, deliver bool) error {
	if validatePath != "" {
		object, err := os.ReadFile(validatePath)
		if err != nil {
			return err
		}
		if err := objectmodel.ValidateCanonicalDocument(object); err != nil {
			return err
		}
		fmt.Println("valid")
		return nil
	}

	if schemaPath == "" || inputPath == "" {
		flag.Usage()
		return errors.New("both -schema and -input are required")
	}
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	input, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}

	obj, err := objectmodel.Materialize(schema, input)
	if err != nil {
		return err
	}
	if _, err := os.Stdout.Write(obj.CanonicalYAML()); err != nil {
		return err
	}
	if deliver {
		if err := module.Execute(obj); err != nil {
			return err
		}
		root := obj.Root()
		fmt.Fprintf(os.Stderr, "delivered model %s, root origin %v, path %s\n",
			obj.ModelVersion(), root.Origin(), root.Path())
	}
	return nil
}
