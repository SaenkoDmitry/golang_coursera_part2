package main

import (
	"net/http"
	"database/sql"
	"encoding/json"
	"strings"
	"errors"
	_ "github.com/go-sql-driver/mysql"
	"strconv"
	"io/ioutil"
	"fmt"
)

type Column struct {
	Field   string
	Type    string
	Null    string
	Key     string
	Default string
	Extra   string
}

type ColumnStore map[string][]Column

func (c *ColumnStore) Search(table string, fieldName string) (*Column, error) {
	mp := map[string][]Column(*c)
	for _, item := range mp[table] {
		if item.Field == fieldName {
			return &item, nil
		}
	}
	return nil, errors.New("Not found")
}

func (c *ColumnStore) GetFields(table string) []string {
	var l []string
	mp := map[string][]Column(*c)
	for _, item := range mp[table] {
		l = append(l, item.Field)
	}
	return l
}

func WriteError(s string) {

}

func NewDbCRUD(db *sql.DB) (http.Handler, error) {
	var err error
	rows, _ := db.Query("SHOW TABLES")
	defer rows.Close()
	tables := []string{}
	for rows.Next() {
		var table string
		rows.Scan(&table)
		tables = append(tables, table)
	}

	columns := make(ColumnStore)

	for _, item := range tables {
		rows, err := db.Query("SHOW COLUMNS FROM " + item)
		if err != nil {
			panic(err)
		}
		defer rows.Close()
		for rows.Next() {
			col := new(Column)
			rows.Scan(&col.Field,
				&col.Type,
				&col.Null,
				&col.Key,
				&col.Default,
				&col.Extra)
			columns[item] = append(columns[item], *col)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.Method {
		case "GET":
			switch r.URL.Path {
			case "/":
				res := make(map[string]map[string]interface{})
				res["response"] = make(map[string]interface{})
				res["response"]["tables"] = tables

				js, err := json.Marshal(res)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.WriteHeader(http.StatusOK)
				w.Write(js)
				return

			default:
				escPath := r.URL.EscapedPath()
				vars := strings.Split(escPath[1:], "/")

				var table string
				var id string
				var rows *sql.Rows

				if len(vars) < 1 {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				table = vars[0]
				founded := false
				exists := false
				for _, item := range tables {
					if table == item {
						exists = true
					}
				}
				if !exists {
					res := make(map[string]interface{})
					res["error"] = errors.New("unknown table").Error()
					js, err := json.Marshal(res)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.WriteHeader(http.StatusNotFound)
					w.Write(js)
					return
				}

				if len(vars) > 1 {
					id = vars[1]
					rows, err = db.Query("SELECT * FROM "+table+" WHERE id = ?", id)
				} else {
					founded = true
					var (
						limit, offset int
						err, err1 error
					)
					l := r.FormValue("limit")
					if l == "" {
						limit = 5
					} else {
						limit, err = strconv.Atoi(l)
					}
					o := r.FormValue("offset")
					if o == "" {
						offset = 0
					} else {
						offset, err1 = strconv.Atoi(o)
					}
					if o != "" && err1 != nil || l != "" && err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					rows, err = db.Query("SELECT * FROM "+table+" LIMIT ? OFFSET ?", limit, offset)
				}
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				cols, err := rows.Columns()

				res := make(map[string]map[string]interface{})
				res["response"] = make(map[string]interface{})
				var items []map[string]interface{}
				var item map[string]interface{}

				count := 0
				for rows.Next() {
					count++
					founded = true
					item = make(map[string]interface{})

					vals := make([]interface{}, len(cols))
					for i := range cols {
						vals[i] = new(sql.RawBytes)
					}
					err = rows.Scan(vals...)

					for i := range vals {
						c, _ := columns.Search(table, cols[i])

						switch c.Type {
						case "int(11)":
							b := []byte(*vals[i].(*sql.RawBytes))
							number, _ := strconv.ParseInt(string(b), 10, 32)
							item[cols[i]] = number
							continue
						case "varchar(255)", "text":
							b := []byte(*vals[i].(*sql.RawBytes))
							if len(b) == 0 && c.Null == "YES" {
								item[cols[i]] = interface{}(nil)
							} else {
								item[cols[i]] = string(b)
							}
						default:
							w.WriteHeader(http.StatusInternalServerError)
							return
						}
					}
					items = append(items, item)
				}

				if count == 0 || !founded {
					res := make(map[string]interface{})
					res["error"] = errors.New("record not found").Error()
					js, err := json.Marshal(res)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.WriteHeader(http.StatusNotFound)
					w.Write(js)
					return
				}

				if len(vars) > 1 {
					res["response"]["record"] = item
				} else {
					res["response"]["records"] = items
				}

				js, err := json.Marshal(res)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write(js)
				return
			}
		case "POST":
			// -------------- getting vars from query string
			escPath := r.URL.EscapedPath()
			vars := strings.Split(escPath[1:], "/")

			if len(vars) != 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			table := vars[0]
			id := vars[1]

			// -------------- check existence table with name from vars
			exists := false
			for _, item := range tables {
				if table == item {
					exists = true
				}
			}
			if !exists {
				res := make(map[string]interface{})
				res["error"] = errors.New("unknown table").Error()
				js, err := json.Marshal(res)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNotFound)
				w.Write(js)
				return
			}

			// -------------- read body params
			var params []interface{}
			qStr := ""
			del := ""
			b, _ := ioutil.ReadAll(r.Body)
			p := make(map[string]interface{})
			_ = json.Unmarshal(b, &p)

			// -------------- validating params
			for _, name := range columns.GetFields(table) {

				item, exists := p[name]
				if !exists {
					continue
				}
				var ok bool
				c, _ := columns.Search(table, name)
				switch c.Type {
				case "int(11)":
					if item == nil {
						params = append(params, 0)
						if c.Null == "YES" {
							ok = true
						}
					} else {
						var n float64
						n, ok = item.(float64)
						params = append(params, n)
					}
				case "varchar(255)", "text":
					if item == nil {
						params = append(params, "")
						if c.Null == "YES" {
							ok = true
						}
					} else {
						var s string
						s, ok = item.(string)
						params = append(params, s)
					}
				}
				if !ok || c.Key == "PRI" {
					res := make(map[string]interface{})
					res["error"] = errors.New("field " + name + " have invalid type").Error()
					js, err := json.Marshal(res)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.WriteHeader(http.StatusBadRequest)
					w.Write(js)
					return
				}
				qStr += del + name + " = ?"
				del = ", "
			}
			params = append(params, id)

			// -------------- updating is needed
			if qStr != "" {
				result, err := db.Exec("UPDATE "+table+" SET "+qStr+" WHERE id = ?", params...)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				g, e := result.RowsAffected()
				if e != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				res := make(map[string]map[string]interface{})
				res["response"] = make(map[string]interface{})
				res["response"]["updated"] = g
				js, err := json.Marshal(res)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write(js)
				return

			} else {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case "DELETE":
			escPath := r.URL.EscapedPath()
			vars := strings.Split(escPath[1:], "/")

			var table string
			var id string

			if len(vars) != 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			table = vars[0]
			id = vars[1]

			exists := false
			for _, item := range tables {
				if table == item {
					exists = true
				}
			}
			if !exists {
				res := make(map[string]interface{})
				res["error"] = errors.New("unknown table").Error()
				js, err := json.Marshal(res)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNotFound)
				w.Write(js)
				return
			}

			result, _ := db.Exec("DELETE FROM "+table+" WHERE id = ?", id)

			g, _ := result.RowsAffected()

			res := make(map[string]map[string]interface{})
			res["response"] = make(map[string]interface{})
			res["response"]["deleted"] = g
			js, err := json.Marshal(res)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write(js)

			return
		case "PUT":
			escPath := r.URL.EscapedPath()
			vars := strings.Split(escPath[1:], "/")

			var temp []string
			for i := range vars {
				if vars[i] == "" {
					continue
				}
				temp = append(temp, vars[i])
			}
			vars = temp

			var table string

			if len(vars) != 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			table = vars[0]

			exists := false
			for _, item := range tables {
				if table == item {
					exists = true
				}
			}
			if !exists {
				res := make(map[string]interface{})
				res["error"] = errors.New("unknown table").Error()
				js, err := json.Marshal(res)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNotFound)
				w.Write(js)
				return
			}

			var params []interface{}
			qStr := ""
			qParams := ""
			del := ""

			b, _ := ioutil.ReadAll(r.Body)
			p := make(map[string]interface{})
			_ = json.Unmarshal(b, &p)

			// -------------- validating params
			for _, name := range columns.GetFields(table) {

				item := p[name]
				var ok bool
				c, _ := columns.Search(table, name)
				if c.Key == "PRI" {
					continue
				}
				switch c.Type {
				case "int(11)":
					if item == nil {
						params = append(params, 0)
						if c.Null == "YES" {
							ok = true
						}
					} else {
						var n float64
						n, ok = item.(float64)
						params = append(params, n)
					}
				case "varchar(255)", "text":
					if item == nil {
						params = append(params, "")
						if c.Null == "YES" {
							ok = true
						}
					} else {
						var s string
						s, ok = item.(string)
						params = append(params, s)
					}
				}
				if !ok {
					res := make(map[string]interface{})
					res["error"] = errors.New("field id have invalid type").Error()
					js, err := json.Marshal(res)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.WriteHeader(http.StatusBadRequest)
					w.Write(js)
					return
				}
				qStr += del + "?"
				qParams += del + name
				del = ", "
			}
			qStr = "(" + qStr + ")"
			qParams = "(" + qParams + ")"

			if qStr != "" {
				result, err := db.Exec("INSERT INTO "+table+" " + qParams + " VALUES"+qStr, params...)
				if err != nil {
					fmt.Println("i:", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				//fmt.Println("after inserting")
				id, e := result.LastInsertId()
				if e != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				res := make(map[string]map[string]interface{})
				res["response"] = make(map[string]interface{})
				res["response"]["id"] = id
				js, err := json.Marshal(res)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write(js)
				return

			} else {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

		default:
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
	return mux, err
}
