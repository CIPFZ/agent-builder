package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"unicode"
	"unicode/utf8"
)

var logMethods = map[string]bool{
	"Debug": true, "Error": true, "Fatal": true, "Info": true,
	"Print": true, "Printf": true, "Println": true, "Warn": true,
}

var skippedDirectories = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
}

func main() {
	fset := token.NewFileSet()
	violations := 0
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && path != "." && skippedDirectories[entry.Name()] {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !logMethods[selector.Sel.Name] {
				return true
			}
			receiver, ok := selector.X.(*ast.Ident)
			if !ok || receiver.Name != "slog" {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			message, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr != nil || message == "" {
				return true
			}
			first, _ := utf8.DecodeRuneInString(message)
			if unicode.IsLower(first) {
				position := fset.Position(literal.Pos())
				fmt.Printf("%s:%d:%d: slog.%s message must start with a capital letter: %q\n", filepath.ToSlash(path), position.Line, position.Column, selector.Sel.Name, message)
				violations++
			}
			return true
		})
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "check log capitalization: %v\n", err)
		os.Exit(1)
	}
	if violations > 0 {
		fmt.Fprintf(os.Stderr, "Log messages must start with a capital letter; found %d violation(s).\n", violations)
		os.Exit(1)
	}
}
