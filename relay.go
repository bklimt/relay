package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"golang.org/x/net/context"

	firebase "firebase.google.com/go"
)

type Config struct {
	ClientID string `json:"clientId"`
}

func Serve(port uint16, app *firebase.App, cfg *Config) {
	// Redirects to the Nest login.
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handling %s request to %s.\n", r.Method, r.RequestURI)

		// Generate a state token.
		client, err := app.Firestore(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to initialize firestore: %s", err)
			return
		}
		defer client.Close()

		doc, _, err := client.Collection("auth").Add(r.Context(), map[string]interface{}{
			"used": false,
			"created": firestore.ServerTimestamp,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to write to firestore: %s", err)
			return
		}
		state := doc.ID

		// Redirect to the Nest oauth endpoint.
		authUrl := fmt.Sprintf("https://home.nest.com/login/oauth2?client_id=%s&state=%s", cfg.ClientID, state)
		w.Header().Add("Location", authUrl)
		w.WriteHeader(http.StatusFound)
		fmt.Fprintln(w, "Redirecting")
	})

	// Handles the oauth redirect from Nest.
	http.HandleFunc("/oauth", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handling %s request to /oauth.\n", r.Method)

		params := r.URL.Query()
		codes := params["code"]
		states := params["state"]
		if len(codes) == 0 || len(states) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "missing code or state")
			return
		}
		//code := codes[0]
		state := states[0]

		// Check the state.
		client, err := app.Firestore(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to initialize firestore: %s", err)
			return
		}
		defer client.Close()

		doc, err := client.Collection("auth").Doc(state).Get(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to read from firestore: %s", err)
			return
		}
		data := doc.Data()
		if data["used"] != false {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "invalid oauth state")
			return
		}
		data["used"] = true
		data["updated"] = firestore.ServerTimestamp
		_, err = client.Collection("auth").Doc(state).Set(r.Context(), data)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to write to firestore: %s", err)
			return
		}

		fmt.Fprintln(w, "Logged in successfully.")
	})

	// Logs a data snapshot to Firestore.
	http.HandleFunc("/log", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handling %s request to %s.\n", r.Method, r.RequestURI)

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "unable to read body: %s", err)
			return
		}
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "unable to parse json: %s", err)
			return
		}

		key := time.Now().UTC().Format(time.RFC3339)
		data["timestamp"] = firestore.ServerTimestamp

		client, err := app.Firestore(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to initialize firestore: %s", err)
			return
		}
		defer client.Close()

		_, err = client.Collection("log").Doc(key).Set(r.Context(), data)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "unable to write to firestore: %s", err)
			return
		}

		fmt.Fprintln(w, "ack")
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Listening on %s.\n", addr)

	log.Fatal(http.ListenAndServe(addr, nil))
}

func LoadConfig() *Config {
	path := os.Getenv("KLIMT_RELAY_CONFIG")
	if path == "" {
		log.Fatal("KLIMT_RELAY_CONFIG environment variable must point to config")
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatalf("error opening config at %q: %s", path, err)
	}

	var cfg *Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		log.Fatalf("error parsing config %q: %s", data, err)
	}

	return cfg
}

func InitFirebase() *firebase.App {
	ctx := context.Background()
	cfg := &firebase.Config{ProjectID: "home-d09a0"}
	app, err := firebase.NewApp(ctx, cfg)
	if err != nil {
		log.Fatalf("error initializing firebase: %s", err)
	}
	return app
}

func main() {
	cfg := LoadConfig()
	app := InitFirebase()
	Serve(8080, app, cfg)
}
