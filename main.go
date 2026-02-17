package main
import (
	_ "github.com/lib/pq"
	"strings"
	"time"
	"github.com/google/uuid"
	"context"
	"log"
	"net/http"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"encoding/json"
	"database/sql"
	"github.com/IArtMediums/chirp_project/internal/database"
	"github.com/joho/godotenv"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries *database.Queries
	platform string
}

var port string = "8080"
var filePathRoot string = "/app/"

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	mux := http.NewServeMux()
	config := &apiConfig{
		fileserverHits: atomic.Int32{}, 
		dbQueries: database.New(db), 
		platform: os.Getenv("PLATFORM"), 
	}
	mux.Handle(filePathRoot, config.middlewareMetricsInc(GetFileServerHandler()))
	registerHandlerFunctions(mux, config)
	server := http.Server{}
	server.Handler = mux
	server.Addr = ":" + port
	
	if err := server.ListenAndServe(); err != nil {
		fmt.Printf("%v\n", err)
		return
	}
}

func registerHandlerFunctions(mux *http.ServeMux, cfg *apiConfig) {
	mux.HandleFunc("GET /api/healthz", HandlerHealthz)
	mux.HandleFunc("GET /admin/metrics", cfg.displayMetrics())
	mux.HandleFunc("POST /admin/reset", cfg.reset())
	mux.HandleFunc("POST /api/users", cfg.middlewareCfg(HandlerCreateUser))
	mux.HandleFunc("POST /api/chirps", cfg.middlewareCfg(HandlerCreateChirp))
	mux.HandleFunc("GET /api/chirps", cfg.middlewareCfg(HandlerGetAllChirps))
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.middlewareCfg(HandlerGetChirpByChirpID))
}

func HandlerGetChirpByChirpID(w http.ResponseWriter, r *http.Request, cfg *apiConfig) {
	type chirp struct {
		ID			uuid.UUID	`json:"id"`
		CreatedAt	time.Time	`json:"created_at"`
		UpdatedAt	time.Time	`json:"updated_at"`
		Body		string		`json:"body"`
		UserID		uuid.UUID	`json:"user_id"`
	}
	idString := r.PathValue("chirpID")
	id, err := uuid.Parse(idString)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(404)
		return
	}
	ctx := context.Background()
	c, err := cfg.dbQueries.GetChirp(ctx, id)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(404)
		return
	}
	res := chirp{
		ID: c.ID,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
		Body: c.Body,
		UserID: c.UserID,
	}
	data, err := json.Marshal(&res)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(200)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func HandlerGetAllChirps(w http.ResponseWriter, r *http.Request, cfg *apiConfig) {
	type chirp struct {
		ID			uuid.UUID	`json:"id"`
		CreatedAt	time.Time	`json:"created_at"`
		UpdatedAt	time.Time	`json:"updated_at"`
		Body		string		`json:"body"`
		UserID		uuid.UUID	`json:"user_id"`
	}
	res := []chirp{}
	ctx := context.Background()
	chirps, err := cfg.dbQueries.GetChirps(ctx)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	for _, c := range chirps {
		res = append(res, chirp{
			ID: c.ID,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			Body: c.Body,
			UserID: c.UserID,
		})
	}
	data, err := json.Marshal(&res)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(200)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func HandlerCreateUser(w http.ResponseWriter, r *http.Request, cfg *apiConfig) {
	type request struct {
		Email     string	`json:"email"`
	}
	type response struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email	  string	`json:"email"`
	}
	decoder := json.NewDecoder(r.Body)
	req := request{}
	if err := decoder.Decode(&req); err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	ctx := context.Background()
	user, err := cfg.dbQueries.CreateUser(ctx, req.Email)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	res := response{
		ID: user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email: user.Email,
	}
	w.WriteHeader(201)
	data, err := json.Marshal(&res)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func HandlerCreateChirp(w http.ResponseWriter, r *http.Request, cfg *apiConfig) {
	type request struct {
		Body	string		`json:"body"`
		UserID	uuid.UUID	`json:"user_id"`
	}
	type response struct {
		ID			uuid.UUID	`json:"id"`
		CreatedAt	time.Time	`json:"created_at"`
		UpdatedAt	time.Time	`json:"updated_at"`
		Body		string		`json:"body"`
		UserID		uuid.UUID	`json:"user_id"`
	}
	decoder := json.NewDecoder(r.Body)
	req := request{}
	if err := decoder.Decode(&req); err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	if !isChirpValid(req.Body) {
		w.WriteHeader(400)
		return
	}
	findAndReplaceProfane(&req.Body)
	ctx := context.Background()
	params := database.CreateChirpParams{
		Body: req.Body,
		UserID: req.UserID,
	}
	chirp, err := cfg.dbQueries.CreateChirp(ctx, params)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	res := response{
		ID: chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body: chirp.Body,
		UserID: chirp.UserID,
	}
	data, err := json.Marshal(&res)
	if err != nil{
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(201)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func isChirpValid(chirp string) bool {
	if len(chirp) > 140 {
		return false
	}
	return true
}

func findAndReplaceProfane(text *string) {
	profane := map[string]struct{}{
		"kerfuffle": {},
		"sharbert": {},
		"fornax": {},
	}
	replace := "****"
	split := strings.Split(*text, " ")
	result := []string{}
	for _, word := range split {
		formatedWord := strings.ToLower(word)
		if containsString(profane, formatedWord) {
			result = append(result, replace)
			continue
		}
		result = append(result, word)
	}
	*text = strings.Join(result, " ")
}

func containsString(mapping map[string]struct{}, value string) bool {
	_, ok := mapping[value]
	return ok
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
		if a.platform != "dev" {
			w.WriteHeader(403)
			return
		}
		ctx := context.Background()
		if err := a.dbQueries.ResetUsers(ctx); err != nil {
			log.Printf("%v\n", err)
			w.WriteHeader(500)
			return
		}
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

func (a *apiConfig) middlewareCfg(handler func (http.ResponseWriter, *http.Request, *apiConfig)) func (http.ResponseWriter, *http.Request) {
	return func (w http.ResponseWriter, req *http.Request) {
		handler(w, req, a)
	}
}
