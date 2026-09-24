/*
Copyright (c) 2020 gqlgen authors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package parsequery

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gqlgo/gqlgenc/internal/fileglob"

	"github.com/vektah/gqlparser/v2/ast"
)

// LoadQuerySources loads query sources from file names.
// Supports glob patterns like **/test/*.graphql; see internal/fileglob.
func LoadQuerySources(queryFileNames []string) ([]*ast.Source, error) {
	noGlobQueryFileNames, err := fileglob.Expand(queryFileNames)
	if err != nil {
		return nil, err
	}

	querySources := make([]*ast.Source, 0, len(noGlobQueryFileNames))
	for _, filename := range noGlobQueryFileNames {
		filename = filepath.ToSlash(filename)

		var (
			err       error
			schemaRaw []byte
		)

		schemaRaw, err = os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("unable to open schema: %w", err)
		}

		querySources = append(querySources, &ast.Source{Name: filename, Input: string(schemaRaw)})
	}

	return querySources, nil
}
