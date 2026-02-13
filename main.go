package main
import (
	"net/http"
	"fmt"
	"io"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

var port string = "8080"
var filePathRoot string = "/app/"

func main() {
	mux := http.NewServeMux()
	config := &apiConfig{fileserverHits: atomic.Int32{}, }
	mux.Handle(filePathRoot, config.middlewareMetricsInc(GetFileServerHandler()))
	registerHandlerFunctions(mux, config)
	server := http.Server{}
	server.Handler = mux
	server.Addr = ":" + port
	err := server.ListenAndServe()
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
}

func registerHandlerFunctions(mux *http.ServeMux, cfg *apiConfig) {
	mux.HandleFunc("GET /api/healthz", HandlerHealthz)
	mux.HandleFunc("GET /admin/metrics", cfg.displayMetrics())
	mux.HandleFunc("POST /admin/reset", cfg.reset())
}

func HandlerHealthz(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "OK")
}

func GetFileServerHandler() http.Handler {
	return http.StripPrefix(filePathRoot, http.FileServer(http.Dir(".")))
}

func (a *apiConfig) reset() func (http.ResponseWriter, *http.Request) {
	return func (w http.ResponseWriter, req *http.Request) {
		a.fileserverHits.Store(0)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "OK")
	}
}

func (a *apiConfig) displayMetrics() func (http.ResponseWriter, *http.Request) {
	return func (w http.ResponseWriter, req *http.Request) {
		contents := fmt.Sprintf(`
			<html>
			  <body>
				<h1>Welcome, Chirpy Admin</h1>
				<p>Chirpy has been visited %d times!</p>
			  </body>
			</html>
			`, a.fileserverHits.Load())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, contents)
	}
}

func (a *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func (w http.ResponseWriter, req *http.Request) {
		a.fileserverHits.Add(1)
		next.ServeHTTP(w, req)
	})
}

