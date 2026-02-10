package main
import (
	"net/http"
	"fmt"
)

func main() {
	handler := http.NewServeMux()
	server := http.Server{}
	server.Handler = handler
	server.Addr = ":8080"
	err := server.ListenAndServe()
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
}
