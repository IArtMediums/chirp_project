package main
import (
	"net/http"
	"fmt"
	"io"
)

var port string = "8080"
var filePathRoot string = "/app/"

func main() {
	mux := http.NewServeMux()
	mux.Handle(filePathRoot, http.StripPrefix(filePathRoot, http.FileServer(http.Dir("."))))
	registerHandlerFunctions(mux)
	server := http.Server{}
	server.Handler = mux
	server.Addr = ":" + port
	err := server.ListenAndServe()
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	fmt.Printf("Server is running on port %v\n", port)
}

func registerHandlerFunctions(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", HandlerHealthz)
}

func HandlerHealthz(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "OK")
}
