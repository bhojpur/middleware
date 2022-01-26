package engine

// Copyright (c) 2018 Bhojpur Consulting Private Limited, India. All rights reserved.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"
)

func registerMiddlewareRandomly(registeredMiddlewares []Middleware) *MiddlewareStack {
	stack := &MiddlewareStack{}
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)

	sort.Slice(registeredMiddlewares, func(i, j int) bool {
		return r.Intn(100)%2 == 1
	})

	for _, m := range registeredMiddlewares {
		stack.Use(m)
	}

	return stack
}

func registerMiddleware(registeredMiddlewares []Middleware) *MiddlewareStack {
	stack := &MiddlewareStack{}

	for _, m := range registeredMiddlewares {
		stack.Use(m)
	}

	return stack
}

func checkSortedMiddlewares(stack *MiddlewareStack, expectedNames []string, t *testing.T) {
	var (
		sortedNames          []string
		sortedMiddlewares, _ = stack.sortMiddlewares()
	)

	for _, middleware := range sortedMiddlewares {
		sortedNames = append(sortedNames, middleware.Name)
	}

	if fmt.Sprint(sortedNames) != fmt.Sprint(expectedNames) {
		t.Errorf("Expected sorted middleware is %v, but got %v", strings.Join(expectedNames, ", "), strings.Join(sortedNames, ", "))
	}
}

func TestCompileMiddlewares(t *testing.T) {
	availableMiddlewares := []Middleware{{Name: "cookie"}, {Name: "flash", InsertAfter: []string{"cookie"}}, {Name: "auth", InsertAfter: []string{"flash"}}}

	stack := registerMiddlewareRandomly(availableMiddlewares)
	checkSortedMiddlewares(stack, []string{"cookie", "flash", "auth"}, t)
}

func TestCompileComplicatedMiddlewares(t *testing.T) {
	availableMiddlewares := []Middleware{{Name: "A"}, {Name: "B", InsertBefore: []string{"C", "D"}}, {Name: "C", InsertAfter: []string{"E"}}, {Name: "D", InsertAfter: []string{"E"}, InsertBefore: []string{"C"}}, {Name: "E", InsertBefore: []string{"B"}, InsertAfter: []string{"A"}}}
	stack := registerMiddlewareRandomly(availableMiddlewares)

	checkSortedMiddlewares(stack, []string{"A", "E", "B", "D", "C"}, t)
}

func TestConflictingMiddlewares(t *testing.T) {
	t.Skipf("conflicting middlewares")
}

func TestMiddlewaresWithRequires(t *testing.T) {
	availableMiddlewares := []Middleware{{Name: "flash", Requires: []string{"cookie"}}, {Name: "session"}}
	stack := registerMiddlewareRandomly(availableMiddlewares)

	if _, err := stack.sortMiddlewares(); err == nil {
		t.Errorf("Should return error as required middleware doesn't exist")
	}
}
