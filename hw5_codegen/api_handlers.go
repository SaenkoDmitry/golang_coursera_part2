package main

import "net/http"
import "encoding/json"
import "errors"
import "strconv"
import "strings"

func (h *MyApi) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/user/profile":
		h.wrapperProfile(w, r)
	case "/user/create":
		h.wrapperCreate(w, r)

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

func (h *MyApi) wrapperProfile(w http.ResponseWriter, r *http.Request) {
	// заполнение структуры params

	params := new(ProfileParams)

	Login := r.FormValue("login")

	if Login == "" {
		w.WriteHeader(http.StatusBadRequest)
		err := errors.New("login must me not empty").Error()
		mp := make(map[string]interface{})
		mp["error"] = err
		js, _ := json.Marshal(mp)
		w.Write(js)
		return
	}
	params.Login = Login


	ctx := r.Context()
	res, err := h.Profile(ctx, *params)
	if err != nil {
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
	w.Write(js)
}

func (h *MyApi) wrapperCreate(w http.ResponseWriter, r *http.Request) {
	// заполнение структуры params

	if r.Method != "POST" {
		w.WriteHeader(http.StatusNotAcceptable)
		err := errors.New("bad method").Error()
		mp := make(map[string]interface{})
		mp["error"] = err
		js, _ := json.Marshal(mp)
		w.Write(js)
		return
	}

	if r.Header.Get("X-Auth") != "100500" {
		w.WriteHeader(http.StatusForbidden)
		err := errors.New("unauthorized").Error()
		mp := make(map[string]interface{})
		mp["error"] = err
		js, _ := json.Marshal(mp)
		w.Write(js)
		return
	}

	params := new(CreateParams)

	Login := r.FormValue("login")

	if Login == "" {
		w.WriteHeader(http.StatusBadRequest)
		err := errors.New("login must me not empty").Error()
		mp := make(map[string]interface{})
		mp["error"] = err
		js, _ := json.Marshal(mp)
		w.Write(js)
		return
	}

	if len(Login) < 10 {
		w.WriteHeader(http.StatusBadRequest)
		err := errors.New("login len must be >= 10").Error()
		mp := make(map[string]interface{})
		mp["error"] = err
		js, _ := json.Marshal(mp)
		w.Write(js)
		return
	}
	params.Login = Login

	Name := r.FormValue("full_name")
	params.Name = Name

	Status := r.FormValue("status")
	if Status != "" {
		var b bool
		for _, x := range strings.Split("user|moderator|admin", "|") {
			if Status == x {
				b = true
			}
		}
		
	if !b {
		w.WriteHeader(http.StatusBadRequest)
		err := errors.New("status must be one of [user, moderator, admin]").Error()
		mp := make(map[string]interface{})
		mp["error"] = err
		js, _ := json.Marshal(mp)
		w.Write(js)
		return
	}

	}
	if Status == "" {
		Status = "user"
	}
	params.Status = Status

	Age := r.FormValue("age")

	if n, _ := strconv.Atoi(Age); n < 0 {
		w.WriteHeader(http.StatusBadRequest)
		err := errors.New("age must be >= 0").Error()
		mp := make(map[string]interface{})
		mp["error"] = err
		js, _ := json.Marshal(mp)
		w.Write(js)
		return
	}

	if n, _ := strconv.Atoi(Age); n > 128 {
		w.WriteHeader(http.StatusBadRequest)
		err := errors.New("age must be <= 128").Error()
		mp := make(map[string]interface{})
		mp["error"] = err
		js, _ := json.Marshal(mp)
		w.Write(js)
		return
	}
	i, err := strconv.Atoi(Age)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		err := errors.New("age must be int").Error()
		mp := make(map[string]interface{})
		mp["error"] = err
		js, _ := json.Marshal(mp)
		w.Write(js)
		return
	}
	params.Age = i

	ctx := r.Context()
	res, err := h.Create(ctx, *params)
	if err != nil {
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
	w.Write(js)
}

