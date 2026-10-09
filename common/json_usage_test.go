package common

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// 编解码统一走 common/json.go（sonic），直接调用 encoding/json 的这些函数会绕过加速。
// json.RawMessage、json.Marshaler 等类型以及 json.Valid 不受限制。
var forbiddenJSONFuncs = map[string]bool{
	"Marshal": true, "Unmarshal": true, "MarshalIndent": true, "NewDecoder": true, "NewEncoder": true,
}

// TestNoDirectEncodingJSONCalls 扫描项目中的非测试代码，禁止绕过 common 直接调用 encoding/json 编解码。
func TestNoDirectEncodingJSONCalls(t *testing.T) {
	root := ".."
	fset := token.NewFileSet()
	var violations []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != root && (strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.ToSlash(path) == "../common/json.go" {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		jsonName := ""
		for _, imp := range file.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == "encoding/json" {
				jsonName = "json"
				if imp.Name != nil {
					jsonName = imp.Name.Name
				}
			}
		}
		if jsonName == "" {
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == jsonName && forbiddenJSONFuncs[sel.Sel.Name] {
				violations = append(violations, fset.Position(call.Pos()).String()+": json."+sel.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("use common.Marshal / common.Unmarshal / common.DecodeJson instead of encoding/json:\n  %s",
			strings.Join(violations, "\n  "))
	}
}
