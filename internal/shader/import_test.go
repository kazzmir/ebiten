// Copyright 2026 The Ebiten Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package shader

import (
	"reflect"
	"strings"
	"testing"
)

func TestCompileWithImports(t *testing.T) {
	src, imports, err := ResolveImports([]byte(`//kage:unit pixels

package main

import "helpers"

func Vertex(dstPos vec2, srcPos vec2, color vec4) (vec4, vec2, vec4) {
	return vec4(dstPos, 0, 1), srcPos, color
}

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return vec4(helpers.clr(1))
}
`), map[string][]byte{
		"helpers": []byte(`package helpers

func clr(red float) (float, float, float, float) {
	return red, 0, 0, 1
}
`),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := CompileWithImports(src, imports, "Vertex", "Fragment", 0); err != nil {
		t.Fatal(err)
	}
}

func TestResolveImportsRejectsCircular(t *testing.T) {
	_, _, err := ResolveImports([]byte(`package main

import "a"

func Vertex(dstPos vec2, srcPos vec2, color vec4) (vec4, vec2, vec4) {
	return vec4(dstPos, 0, 1), srcPos, color
}

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return a.color()
}
`), map[string][]byte{
		"a": []byte(`package a

import "b"

func color() vec4 {
	return b.color()
}
`),
		"b": []byte(`package b

import "a"

func color() vec4 {
	return a.color()
}
`),
	})
	if err == nil {
		t.Fatal("ResolveImports succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "circular import") {
		t.Fatalf("ResolveImports error = %v, want circular import", err)
	}
}

func TestCompileWithImportsKeepsMainUniformsOnly(t *testing.T) {
	src, imports, err := ResolveImports([]byte(`//kage:unit pixels

package main

import "helpers"

var Red float

func Vertex(dstPos vec2, srcPos vec2, color vec4) (vec4, vec2, vec4) {
	return vec4(dstPos, 0, 1), srcPos, color
}

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	return vec4(helpers.clr(Red))
}
`), map[string][]byte{
		"helpers": []byte(`package helpers

var Imported float

func clr(red float) (float, float, float, float) {
	return red, 0, 0, 1
}
`),
	})
	if err != nil {
		t.Fatal(err)
	}

	ir, err := CompileWithImports(src, imports, "Vertex", "Fragment", 0)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := ir.UniformNames, []string{"Red"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UniformNames = %q, want %q", got, want)
	}
}
