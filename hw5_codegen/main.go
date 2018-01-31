package main

// это программа для которой ваш кодогенератор будет писать код
// запускать через go test -v, как обычно

// этот код закомментирован чтобы он не светился в тестовом покрытии

import (
	"fmt"
	"net/http"
)
func test(w http.ResponseWriter, r *http.Request) {
}

func main() {
	// будет вызван метод ServeHTTP у структуры MyApi
	http.Handle("/user/", &MyApi{})

	fmt.Println("starting server at :8080")
	http.ListenAndServe(":8080", nil)
}