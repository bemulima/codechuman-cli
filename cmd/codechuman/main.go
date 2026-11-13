package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/bemulima/codechuman-cli/internal/checkout"
	"github.com/bemulima/codechuman-cli/internal/validation"
)

const version = "0.1.0-dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "codechuman:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("expected command: doctor, checkout, list, validate-js, validate-public-js or version")
	}
	switch args[0] {
	case "version":
		fmt.Println(version)
		return nil
	case "doctor":
		root, err := checkout.WorkspaceRoot("")
		if err != nil {
			return err
		}
		_, gitErr := exec.LookPath("git")
		_, nodeErr := exec.LookPath("node")
		return writeJSON(map[string]any{
			"version": version, "workspace_root": root,
			"git_available": gitErr == nil, "node_available": nodeErr == nil,
		})
	case "checkout":
		flags := flag.NewFlagSet("checkout", flag.ContinueOnError)
		fileName := flags.String("spec", "", "path to a server-authorized checkout JSON")
		root := flags.String("root", "", "workspace root (default configurable)")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *fileName == "" {
			return errors.New("--spec is required")
		}
		f, err := os.Open(*fileName)
		if err != nil {
			return err
		}
		defer f.Close()
		spec, err := checkout.ReadSpec(f)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		result, err := checkout.Checkout(ctx, *root, spec, checkout.GitCloner{})
		if err != nil {
			return err
		}
		return writeJSON(result)
	case "list":
		flags := flag.NewFlagSet("list", flag.ContinueOnError)
		root := flags.String("root", "", "workspace root")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		entries, err := checkout.List(*root)
		if err != nil {
			return err
		}
		return writeJSON(entries)
	case "validate-js":
		flags := flag.NewFlagSet("validate-js", flag.ContinueOnError)
		workspace := flags.String("workspace", "", "local task checkout directory")
		file := flags.String("file", "app.js", "public JavaScript file to syntax-check")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *workspace == "" {
			return errors.New("--workspace is required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return writeJSON(validation.JavaScriptSyntax(ctx, *workspace, *file))
	case "validate-public-js":
		flags := flag.NewFlagSet("validate-public-js", flag.ContinueOnError)
		workspace := flags.String("workspace", "", "local task checkout directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *workspace == "" {
			return errors.New("--workspace is required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return writeJSON(validation.PublicJavaScriptTests(ctx, *workspace))
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
