package main
import (
	_ "github.com/lib/pq"
	"strings"
	"time"
	"sort"
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
	"github.com/IArtMediums/chirp_project/internal/auth"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries *database.Queries
	platform string
	secret string
	polkaKey string
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
		secret: os.Getenv("SECRET"),
		polkaKey: os.Getenv("POLKA_KEY"),
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
	mux.HandleFunc("POST /api/chirps", cfg.middlewareAuthCfg(HandlerCreateChirp))
	mux.HandleFunc("GET /api/chirps", cfg.middlewareCfg(HandlerGetAllChirps))
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.middlewareCfg(HandlerGetChirpByChirpID))
	mux.HandleFunc("POST /api/login", cfg.middlewareCfg(HandlerLogin))
	mux.HandleFunc("POST /api/refresh", cfg.middlewareCfg(HandlerRefreshToken))
	mux.HandleFunc("POST /api/revoke", cfg.middlewareCfg(HandlerRevoke))
	mux.HandleFunc("PUT /api/users", cfg.middlewareAuthCfg(HandlerUpdateLogin))
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", cfg.middlewareAuthCfg(HandlerDeleteChirp))
	mux.HandleFunc("POST /api/polka/webhooks", cfg.middlewareCfg(HandlerUpgradeUser))
}

func HandlerUpgradeUser(w http.ResponseWriter, r *http.Request, cfg *apiConfig) {
	type requestData struct {
		UserID	uuid.UUID	`json:"user_id"`
	}
	type request struct {
		Event	string			`json:"event"`
		Data	requestData		`json:"data"`
	}
	api_key, err := auth.GetAPIKey(r.Header)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(401)
		return
	}
	if api_key != cfg.polkaKey {
		log.Printf("%v\n", err)
		w.WriteHeader(401)
		return
	}
	decoder := json.NewDecoder(r.Body)
	req := request{}
	if err := decoder.Decode(&req); err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	if req.Event != "user.upgraded" {
		w.WriteHeader(204)
		return
	}
	ctx := context.Background()
	if err := cfg.dbQueries.UpgradeUser(ctx, req.Data.UserID); err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(404)
		return
	}
	w.WriteHeader(204)
}

func HandlerDeleteChirp(w http.ResponseWriter, r *http.Request, cfg *apiConfig, id uuid.UUID) {
	idString := r.PathValue("chirpID")
	chirp_id, err := uuid.Parse(idString)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(404)
		return
	}
	ctx := context.Background()
	chirp, err := cfg.dbQueries.GetChirp(ctx, chirp_id)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(404)
		return
	}
	if chirp.UserID != id {
		log.Printf("%v\n", err)
		w.WriteHeader(403)
		return
	}
	if err := cfg.dbQueries.DeleteChirp(ctx, chirp_id); err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(404)
		return
	}
	w.WriteHeader(204)
}

func HandlerUpdateLogin(w http.ResponseWriter, r *http.Request, cfg *apiConfig, id uuid.UUID) {
	type request struct {
		Password	string	`json:"password"`
		Email		string	`json:"email"`
	}
	type response struct {
		ID			uuid.UUID			`json:"id"`
		Email		string				`json:"email"`
		CreatedAt	time.Time			`json:"created_at"`
		UpdatedAt	time.Time			`json:"updated_at"`
		IsChirpyRed	bool				`json:"is_chirpy_red"`
	}
	decoder := json.NewDecoder(r.Body)
	req := request{}
	if err := decoder.Decode(&req); err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(401)
		return
	}
	ctx := context.Background()
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	params := database.UpdateUserLoginParams{
		Email: req.Email,
		HashedPassword: hash,
		ID: id,
	}
	user, err := cfg.dbQueries.UpdateUserLogin(ctx, params)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(401)
		return
	}
	res := response{
		ID: id,
		Email: user.Email,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		IsChirpyRed: user.IsChirpyRed,
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

func HandlerRevoke(w http.ResponseWriter, r *http.Request, cfg *apiConfig) {
	refToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(401)
		return
	}
	ctx := context.Background()
	if err := cfg.dbQueries.RevokeToken(ctx, refToken); err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(401)
		return
	}
	w.WriteHeader(204)
}

func HandlerRefreshToken(w http.ResponseWriter, r *http.Request, cfg *apiConfig) {
	type response struct {
		Token string `json:"token"`
	}
	refToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(401)
		return
	}
	ctx := context.Background()
	id, err := cfg.dbQueries.GetUserFromRefreshToken(ctx, refToken)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(401)
		return
	}
	acToken, err := CreateAccessToken(id, cfg)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	res := response{
		Token: acToken,
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

func HandlerLogin(w http.ResponseWriter, r *http.Request, cfg *apiConfig) {
	type request struct {
		Email				string			`json:"email"`
		Password			string			`json:"password"`
	}
	type response struct {
		ID				uuid.UUID	`json:"id"`
		CreatedAt		time.Time	`json:"created_at"`
		UpdatedAt		time.Time	`json:"updated_at"`
		Email			string		`json:"email"`
		Token			string		`json:"token"`
		RefreshToken	string		`json:"refresh_token"`
		IsChirpyRed		bool		`json:"is_chirpy_red"`
	}
	decoder := json.NewDecoder(r.Body)
	req := request{}
	if err := decoder.Decode(&req); err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	ctx := context.Background()
	user, err := cfg.dbQueries.GetUserByEmail(ctx, req.Email)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(404)
		return
	}
	match, err := auth.CheckPasswordHash(req.Password, user.HashedPassword)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	if !match {
		w.WriteHeader(401)
		return
	}
	acToken, err := CreateAccessToken(user.ID, cfg)
	refToken, err := CreateRefreshToken(user.ID, cfg)
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
		Token: acToken,
		RefreshToken: refToken,
		IsChirpyRed: user.IsChirpyRed,
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

func CreateAccessToken(id uuid.UUID, cfg *apiConfig) (string, error) {
	duration, err := time.ParseDuration("1h")
	if err != nil {
		return "", err
	}
	acToken, err := auth.MakeJWT(id, cfg.secret, duration)
	if err != nil {
		return "", err
	}
	return acToken, nil
}

func CreateRefreshToken(id uuid.UUID, cfg *apiConfig) (string, error) {
	refToken := auth.MakeRefreshToken()
	ctx := context.Background()
	params := database.CreateTokenParams{
		Token: refToken,
		UserID: id,
	}
	_, err := cfg.dbQueries.CreateToken(ctx, params)
	if err != nil {
		return "", err
	}
	return refToken, nil
}

func RefreshToken(refToken string, cfg *apiConfig) (uuid.UUID, error) {
	return uuid.Nil, nil
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
	author_id := r.URL.Query().Get("author_id")
	var get_func func(context.Context) ([]database.Chirp, error)
	if author_id != "" {
		get_func = func(c context.Context) ([]database.Chirp, error) {
			parsed, err := uuid.Parse(author_id)
			if err != nil {
				return cfg.dbQueries.GetChirps(c)
			}
			return cfg.dbQueries.GetChirpsByAuthor(c, parsed)
		}
	} else {
		get_func = cfg.dbQueries.GetChirps
	}
	res := []chirp{}
	ctx := context.Background()
	chirps, err := get_func(ctx)
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
	sortQuery := r.URL.Query().Get("sort")
	if sortQuery == "desc" {
		sort.Slice(res, func(i, j int) bool {return res[i].CreatedAt.Compare(res[j].CreatedAt) == 1})
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
		Password  string	`json:"password"`
	}
	type response struct {
		ID			uuid.UUID	`json:"id"`
		CreatedAt	time.Time	`json:"created_at"`
		UpdatedAt	time.Time	`json:"updated_at"`
		Email		string		`json:"email"`
		IsChirpyRed	bool		`json:"is_chirpy_red"`
	}
	decoder := json.NewDecoder(r.Body)
	req := request{}
	if err := decoder.Decode(&req); err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	ctx := context.Background()
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		log.Printf("%v\n", err)
		w.WriteHeader(500)
		return
	}
	params := database.CreateUserParams{
		Email: req.Email,
		HashedPassword: hash,
	}
	user, err := cfg.dbQueries.CreateUser(ctx, params)
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
		IsChirpyRed: user.IsChirpyRed,
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

func HandlerCreateChirp(w http.ResponseWriter, r *http.Request, cfg *apiConfig, id uuid.UUID) {
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
		UserID: id,
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

func (a *apiConfig) middlewareAuthCfg(handler func (http.ResponseWriter, *http.Request, *apiConfig, uuid.UUID)) func (http.ResponseWriter, *http.Request) {
	return func (w http.ResponseWriter, r *http.Request) {
		token, err := auth.GetBearerToken(r.Header)
		if err != nil {
			log.Printf("%v\n", err)
			w.WriteHeader(401)
			return
		}
		id, err := auth.ValidateJWT(token, a.secret)
		if err != nil {
			log.Printf("%v\n", err)
			w.WriteHeader(401)
			return
		}
		handler(w, r, a, id)
	}
}
