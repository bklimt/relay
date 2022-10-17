package main

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/bklimt/relay"
	"github.com/bklimt/relay/common"
	"github.com/bklimt/relay/nest"
	"github.com/gorilla/mux"

	firebase "firebase.google.com/go"
)

type server struct {
	App *firebase.App
	Cfg *relay.Config
}

type HandlerFunc func(http.ResponseWriter, *http.Request, *server) error

func wrapHandler(f HandlerFunc, s *server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := f(w, r, s); err != nil {
			w.WriteHeader(common.Status(err))
			fmt.Fprintf(w, "%s", err)
		}
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request, srv *server) error {
	log.Printf("Handling %s request to %s.\n", r.Method, r.RequestURI)

	// Generate a state token.
	state, err := relay.GenerateStateToken(r.Context(), srv.App)
	if err != nil {
		return err
	}

	// Redirect to the Nest oauth endpoint.
	authURL := fmt.Sprintf("https://home.nest.com/login/oauth2?client_id=%s&state=%s", srv.Cfg.ClientID, state)
	w.Header().Add("Location", authURL)
	w.WriteHeader(http.StatusFound)
	fmt.Fprintln(w, "Redirecting")
	return nil
}

func handleOAuth(w http.ResponseWriter, r *http.Request, srv *server) error {
	log.Printf("Handling %s request to /oauth.\n", r.Method)

	params := r.URL.Query()
	codes := params["code"]
	states := params["state"]
	if len(codes) == 0 || len(states) == 0 {
		return common.Errorf(http.StatusBadRequest, "missing code or state")
	}
	code := codes[0]
	state := states[0]

	// Check the state.
	if err := relay.CheckState(r.Context(), srv.App, state); err != nil {
		return err
	}

	// Get a Nest access token.
	accessToken, err := nest.GetAccessToken(srv.Cfg.ClientID, srv.Cfg.ClientSecret, code)
	if err != nil {
		return err
	}

	// Get the user ID.
	data, err := nest.GetData(r.Context(), accessToken)
	if err != nil {
		return err
	}

	// Save the metadata to Firestore.
	if err := relay.SaveNestData(r.Context(), srv.App, data); err != nil {
		return err
	}

	fmt.Fprintln(w, "Logged in successfully.")
	return nil
}

func handleLog(w http.ResponseWriter, r *http.Request, srv *server) error {
	log.Printf("Handling %s request to %s.\n", r.Method, r.RequestURI)

	// Read the JSON for the Feather in the request body.
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return common.Errorf(http.StatusBadRequest, "unable to read body: %s", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return common.Errorf(http.StatusBadRequest, "unable to parse json: %s", err)
	}

	// Make a key to store the data under.
	key := relay.KeyForNow()

	// Save the data from the feather.
	if err := relay.LogFeatherData(r.Context(), srv.App, key, data); err != nil {
		return err
	}

	// Get the current Nest data and save it.
	if err := LogNestData(r.Context(), srv.App, key); err != nil {
		return err
	}

	fmt.Fprintln(w, "ack")
	return nil
}

func handleImage(w http.ResponseWriter, r *http.Request, srv *server) error {
	log.Printf("Handling %s request to %s.\n", r.Method, r.RequestURI)

	vars := mux.Vars(r)
	filename, ok := vars["filename"]
	if !ok {
		return common.Errorf(http.StatusInternalServerError, "missing filename")
	}

	// Check that it's an image.
	contentType := r.Header.Get("Content-Type")
	if contentType != "image/jpeg" {
		return common.Errorf(http.StatusBadRequest, "invalid content type %s", contentType)
	}

	// Read the body of the request.
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return common.Errorf(http.StatusBadRequest, "unable to read body: %s", err)
	}

	now := time.Now().UTC()
	path := fmt.Sprintf("%d/%d/%d/%s", now.Year(), now.Month(), now.Day(), filename)

	// Write it to Cloud Storage.
	if err := WriteToStorage(r.Context(), srv.App, path, contentType, body); err != nil {
		return err
	}

	fmt.Fprintf(w, "%s", filename)
	return nil
}

func WriteToStorage(ctx context.Context, app *firebase.App, filename, contentType string, data []byte) error {
	storage, err := app.Storage(ctx)
	if err != nil {
		return fmt.Errorf("unable to access storage: %s", err)
	}

	bucket, err := storage.DefaultBucket()
	if err != nil {
		return fmt.Errorf("unable to get bucket: %s", err)
	}

	object := bucket.Object(filename)
	writer := object.NewWriter(ctx)
	writer.ObjectAttrs.ContentType = contentType
	if _, err = writer.Write(data); err != nil {
		return fmt.Errorf("unable to write file: %s", err)
	}

	if err = writer.Close(); err != nil {
		return fmt.Errorf("unable to close file: %s", err)
	}

	return nil
}

func serve(port uint16, app *firebase.App, cfg *relay.Config) {
	r := mux.NewRouter()

	server := &server{
		App: app,
		Cfg: cfg,
	}

	// Redirects to the Nest login.
	r.HandleFunc("/login", wrapHandler(handleLogin, server))

	// Handles the oauth redirect from Nest.
	r.HandleFunc("/oauth", wrapHandler(handleOAuth, server))

	// Logs a data snapshot to Firestore.
	r.HandleFunc("/log", wrapHandler(handleLog, server)).Methods("POST")

	// Saves an image to Firebase Storage.
	r.HandleFunc("/image/{filename}", wrapHandler(handleImage, server)).Methods("POST")

	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{
		Handler:      r,
		Addr:         addr,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Printf("Listening on %s.\n", addr)
	log.Fatal(srv.ListenAndServe())
}

func LogNestData(ctx context.Context, app *firebase.App, key string) error {
	users, err := relay.GetNestUsers(ctx, app)
	if err != nil {
		return err
	}

	for _, token := range users {
		data, err := nest.GetData(ctx, token)
		if err != nil {
			return err
		}

		err = relay.LogNestData(ctx, app, key, data)
		if err != nil {
			return err
		}
	}

	return nil
}

func Checkup() {
	log.Printf("%s: Time for a checkup...", relay.KeyForNow())
}

func CheckupForever() {
	for {
		Checkup()
		time.Sleep(1 * time.Hour)
	}
}

func main() {
	cfg := relay.LoadConfig()
	expvar.NewString("projectId").Set(cfg.ProjectID)
	expvar.NewString("clientId").Set(cfg.ClientID)
	app := relay.InitFirebase(&firebase.Config{
		ProjectID:     cfg.ProjectID,
		StorageBucket: cfg.StorageBucket,
	})

	go CheckupForever()

	serve(8080, app, cfg)
}
