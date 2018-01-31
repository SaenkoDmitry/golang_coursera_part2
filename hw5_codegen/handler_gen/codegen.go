package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"log"
	"fmt"
	"strings"
	"encoding/json"
	"text/template"
	"reflect"
	"bytes"
)

type tpl struct {
	StructName string
	Cases      []subTpl
	Functions  []mtdTpl
}

func (s *tpl) String() string {
	s1 := ""
	s2 := ""
	for _, x := range s.Cases {
		s1 += x.String()
	}
	for _, x := range s.Functions {
		s2 += x.String()
	}
	return `func (h *` + s.StructName + `) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
` + s1 + `
	default:
		w.WriteHeader(http.StatusNotFound)
		err := errors.New("unknown method").Error()
		mp := make(map[string]interface{})
		mp["error"] = err
		js, _ := json.Marshal(mp)
		w.Write(js)
		return
	}
}
` + s2
}

type subTpl struct {
	Url        string
	MethodName string
}

func (s *subTpl) String() string {
	return `	case "` + s.Url + `":
		h.wrapper` + s.MethodName + `(w, r)
`
}

type mtdTpl struct {
	StructName       string
	AuthAndMethod    string
	MethodName       string
	ValidationParams string
	Handlers         string
}

func (s *mtdTpl) String() string {
	return `
func (h *` + s.StructName + `) wrapper` + s.MethodName + `(w http.ResponseWriter, r *http.Request) {
	// заполнение структуры params
` + s.AuthAndMethod + `
	params := new(` + s.MethodName + `Params)

` + s.ValidationParams + `
	ctx := r.Context()
	res, err := h.` + s.MethodName + `(ctx, *params)
` + s.Handlers + `
}
`
}

type errTemplate struct {
	Condition string
	HttpError string
	Msg       string
}

var (
	errTpl = template.Must(template.New("errTpl").Parse(`
	if {{.Condition}} {
		w.WriteHeader({{.HttpError}})
		err := errors.New("{{.Msg}}").Error()
		mp := make(map[string]interface{})
		mp["error"] = err
		js, _ := json.Marshal(mp)
		w.Write(js)
		return
	}
`))
)

func getParam(s string, name string) string {
	v := s[strings.Index(s, name+"="):]
	if strings.Index(v, ",") != -1 {
		v = v[:strings.Index(v, ",")]
	}
	v = strings.TrimPrefix(v, name+"=")
	return v
}

func main() {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, os.Args[1], nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	out, _ := os.Create(os.Args[2])

	fmt.Fprintln(out, `package `+node.Name.Name)
	fmt.Fprintln(out) // empty line
	//fmt.Fprintln(out, `import "encoding/binary"`)
	fmt.Fprintln(out, `import "net/http"`)
	fmt.Fprintln(out, `import "encoding/json"`)
	fmt.Fprintln(out, `import "errors"`)
	fmt.Fprintln(out, `import "strconv"`)
	fmt.Fprintln(out, `import "strings"`)
	fmt.Fprintln(out) // empty line
	tplInstance := &tpl{}
	valids := make(map[string]string)
	resp := make(map[string]string)

	for _, f := range node.Decls {
		currFunc, ok := f.(*ast.FuncDecl)
		if ok {
			needCodegen := false
			doc := currFunc.Doc
			if doc == nil {
				continue
			}
			for _, comment := range currFunc.Doc.List {
				needCodegen = needCodegen || strings.HasPrefix(comment.Text, "// apigen:api ")
			}
			if !needCodegen {
				fmt.Printf("SKIP struct %#v doesnt have cgen mark\n", currFunc.Name.Name)
				continue
			}

			mp := make(map[string]interface{})
			json.Unmarshal([]byte(strings.Trim(currFunc.Doc.List[0].Text, "// apigen:api ")), &mp)

			s := subTpl{}
			s.Url = mp["url"].(string)
			s.MethodName = currFunc.Name.Name
			tplInstance.Cases = append(tplInstance.Cases, s)

			m := mtdTpl{}
			m.MethodName = currFunc.Name.Name
			if mp["method"] == "POST" {
				temp := bytes.NewBufferString("")
				errTpl.Execute(temp, errTemplate{`r.Method != "POST"`, "http.StatusNotAcceptable", "bad method"})
				m.AuthAndMethod += temp.String()
			}
			if mp["auth"].(bool) {
				temp := bytes.NewBufferString("")
				errTpl.Execute(temp, errTemplate{`r.Header.Get("X-Auth") != "100500"`, "http.StatusForbidden", "unauthorized"})
				m.AuthAndMethod += temp.String()
			}
			m.StructName = tplInstance.StructName
			tplInstance.Functions = append(tplInstance.Functions, m)
			continue
		}

		g, ok := f.(*ast.GenDecl)
		if !ok {
			fmt.Printf("SKIP %#T is not *ast.GenDecl\n", f)
			continue
		}
	SPECS_LOOP:
		for _, spec := range g.Specs {
			currType, ok := spec.(*ast.TypeSpec)
			if !ok {
				//fmt.Printf("SKIP %#T is not ast.TypeSpec\n", currType)
				continue SPECS_LOOP
			}

			currStruct, ok := currType.Type.(*ast.StructType)
			if !ok {
				fmt.Printf("SKIP %#T is not ast.StructType\n", currStruct)
				continue
			}

			//FIELDS_LOOP:
			if len(currStruct.Fields.List) == 0 {
				tplInstance.StructName = currType.Name.Name
			}
			for _, field := range currStruct.Fields.List {
				apivalidator := ""
				if field.Tag != nil {
					tag := reflect.StructTag(field.Tag.Value[1: len(field.Tag.Value)-1])
					apivalidator = tag.Get("apivalidator")
					if apivalidator == "" {
						continue
					}

					fieldName := field.Names[0].Name

					if strings.Contains(apivalidator, "paramname=") {
						paramName := getParam(apivalidator, "paramname")
						valids[currType.Name.Name] += `	` + fieldName + ` := r.FormValue("` + paramName + `")
`
					} else {
						valids[currType.Name.Name] += `	` + fieldName + ` := r.FormValue("` + strings.ToLower(fieldName) + `")
`
					}

					if strings.Contains(apivalidator, "required") {
						temp := bytes.NewBufferString("")
						errTpl.Execute(temp, errTemplate{fieldName + ` == ""`, "http.StatusBadRequest", "login must me not empty"})
						valids[currType.Name.Name] += temp.String()
					}

					if strings.Contains(apivalidator, "enum=") {
						enum := getParam(apivalidator, "enum")
						temp := bytes.NewBufferString("")
						errTpl.Execute(temp, errTemplate{Condition:"!b", HttpError:"http.StatusBadRequest", Msg:"status must be one of [user, moderator, admin]"})
						valids[currType.Name.Name] += `	if ` + fieldName + ` != "" {
		var b bool
		for _, x := range strings.Split("` + enum + `", "|") {
			if ` + fieldName + ` == x {
				b = true
			}
		}
		` + temp.String() + `
	}
`
					}

					if strings.Contains(apivalidator, "default=") {
						v := getParam(apivalidator, "default")
						valids[currType.Name.Name] += `	if ` + fieldName + ` == "" {
		` + fieldName + ` = "` + v + `"
	}
`
					}

					if strings.Contains(apivalidator, "min=") {
						min := getParam(apivalidator, "min")
						fileType := field.Type.(*ast.Ident).Name
						temp := bytes.NewBufferString("")
						if fileType == "string" {
							s := ""
							if fieldName == "Login" {
								s = " len"
							}
							errTpl.Execute(temp, errTemplate{`len(` + fieldName + `) < ` + min, "http.StatusBadRequest", strings.ToLower(fieldName) + s + ` must be >= ` + min})
							valids[currType.Name.Name] += temp.String()
						} else {
							errTpl.Execute(temp, errTemplate{`n, _ := strconv.Atoi(` + fieldName + `); n < ` + min, "http.StatusBadRequest", strings.ToLower(fieldName) + ` must be >= ` + min})
							valids[currType.Name.Name] += temp.String()
						}
					}

					if strings.Contains(apivalidator, "max=") {
						max := getParam(apivalidator, "max")
						temp := bytes.NewBufferString("")
						errTpl.Execute(temp, errTemplate{`n, _ := strconv.Atoi(` + fieldName + `); n > ` + max, "http.StatusBadRequest", strings.ToLower(fieldName) + ` must be <= ` + max})
						valids[currType.Name.Name] += temp.String()
					}
					fileType := field.Type.(*ast.Ident).Name
					switch fileType {
					case "int":
						temp := bytes.NewBufferString("")
						errTpl.Execute(temp, errTemplate{`err != nil`, "http.StatusBadRequest", strings.ToLower(fieldName) + ` must be int`})
						valids[currType.Name.Name] += `	i, err := strconv.Atoi(` + fieldName + `)`
						valids[currType.Name.Name] += temp.String()
						valids[currType.Name.Name] += "	params." + fieldName + " = i\n"
					case "string":
						valids[currType.Name.Name] += "	params." + fieldName + " = " + fieldName + "\n\n"
					//default:
					//	_ = 1
					}
				}
			}
			resp[currType.Name.Name] += `	if err != nil {
		e, ok := err.(ApiError)
		if ok {
			w.WriteHeader(e.HTTPStatus)
			mk := make(map[string]interface{})
			mk["error"] = err.Error()
			js, _ := json.Marshal(mk)
			w.Write(js)
			return
		} else {
			if err != nil && err.Error() == "bad user" {
				w.WriteHeader(http.StatusInternalServerError)
				mk := make(map[string]interface{})
				mk["error"] = err.Error()
				js, _ := json.Marshal(mk)
				w.Write(js)
				return
			}
		}
	}
	w.WriteHeader(http.StatusOK)
	mk := make(map[string]interface{})
	mk["error"] =	 ""
	mk["response"] = res
	js, _ := json.Marshal(mk)
	w.Write(js)`
		}
	}
	for i := range tplInstance.Functions {
		tplInstance.Functions[i].ValidationParams = valids[tplInstance.Functions[i].MethodName+"Params"]
		tplInstance.Functions[i].Handlers = resp[tplInstance.Functions[i].MethodName+"Params"]
	}

	fmt.Fprintln(out, tplInstance.String())
}
